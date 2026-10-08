package jwt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

const (
	oidcStatePrefix = "auth/jwt/oidc-state/"
	oidcStateTTL    = 5 * time.Minute
	// maxOIDCStates caps unauthenticated state records so that hammering the
	// auth-url endpoint cannot grow the barrier without bound.
	maxOIDCStates     = 1000
	oidcSweepAbove    = 100
	maxTokenRespBytes = 1 << 20
)

var (
	// ErrOIDCNotConfigured is returned when the OIDC browser flow is not set up.
	ErrOIDCNotConfigured = errors.New("jwt: OIDC login not configured")
	// ErrInvalidState is returned for an unknown, expired or already used state.
	ErrInvalidState = errors.New("jwt: invalid or expired OIDC state")
	// ErrCodeExchange is returned when the IdP rejects the authorization code.
	ErrCodeExchange = errors.New("jwt: authorization code rejected by identity provider")
	// ErrIdPUnavailable is returned when the IdP token endpoint cannot be reached.
	ErrIdPUnavailable = errors.New("jwt: identity provider unavailable")
	// ErrTooManyPending is returned when too many login attempts are in flight.
	ErrTooManyPending = errors.New("jwt: too many pending OIDC logins")
)

// OIDCEnabled reports whether the browser login flow is configured.
func (c *Config) OIDCEnabled() bool { return c != nil && c.OIDCAuthorizationURL != "" }

func (c *Config) oidcFieldsSet() bool {
	return c.OIDCAuthorizationURL != "" || c.OIDCTokenURL != "" || c.OIDCClientID != "" ||
		c.OIDCClientSecret != "" || c.OIDCRedirectURI != "" || c.OIDCScopes != ""
}

// ValidateOIDC checks the oidc_* fields. A config with none of them set is valid.
func (c *Config) ValidateOIDC() error {
	if !c.oidcFieldsSet() {
		return nil
	}
	for _, f := range []struct{ name, val string }{
		{"oidc_authorization_url", c.OIDCAuthorizationURL},
		{"oidc_token_url", c.OIDCTokenURL},
		{"oidc_client_id", c.OIDCClientID},
		{"oidc_redirect_uri", c.OIDCRedirectURI},
		{"jwks_uri", c.JWKSURI},
		{"issuer", c.Issuer},
	} {
		if f.val == "" {
			return fmt.Errorf("%s is required for OIDC login", f.name)
		}
	}
	for _, f := range []struct{ name, val string }{
		{"oidc_authorization_url", c.OIDCAuthorizationURL},
		{"oidc_token_url", c.OIDCTokenURL},
		{"oidc_redirect_uri", c.OIDCRedirectURI},
	} {
		if err := checkOIDCURL(f.val); err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
	}
	return nil
}

// checkOIDCURL accepts https URLs, and plain http only for loopback hosts.
func checkOIDCURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("must be an absolute URL")
	}
	if u.User != nil || u.Fragment != "" {
		return errors.New("must not contain credentials or a fragment")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return nil
		}
	}
	return errors.New("must use https (http is allowed only for localhost)")
}

func (c *Config) scopes() string {
	fields := strings.Fields(c.OIDCScopes)
	if len(fields) == 0 {
		return "openid profile"
	}
	hasOpenID := false
	for _, f := range fields {
		if f == "openid" {
			hasOpenID = true
		}
	}
	if !hasOpenID {
		fields = append([]string{"openid"}, fields...)
	}
	return strings.Join(fields, " ")
}

// OIDCState is the server-side record of one in-flight browser login.
type OIDCState struct {
	Nonce     string    `json:"nonce"`
	Verifier  string    `json:"verifier"`
	ExpiresAt time.Time `json:"expires_at"`
}

func randomURLSafe(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewOIDCAttempt creates the state, nonce and PKCE pair and the IdP redirect
// URL for one login. The caller persists st under the returned state id.
func (c *Config) NewOIDCAttempt() (authURL, state string, st *OIDCState, err error) {
	if !c.OIDCEnabled() {
		return "", "", nil, ErrOIDCNotConfigured
	}
	if state, err = randomURLSafe(32); err != nil {
		return "", "", nil, err
	}
	nonce, err := randomURLSafe(16)
	if err != nil {
		return "", "", nil, err
	}
	verifier, err := randomURLSafe(32)
	if err != nil {
		return "", "", nil, err
	}
	sum := sha256.Sum256([]byte(verifier))
	u, err := url.Parse(c.OIDCAuthorizationURL)
	if err != nil {
		return "", "", nil, fmt.Errorf("jwt: authorization url: %w", err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", c.OIDCClientID)
	q.Set("redirect_uri", c.OIDCRedirectURI)
	q.Set("scope", c.scopes())
	q.Set("state", state)
	q.Set("nonce", nonce)
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), state, &OIDCState{
		Nonce:     nonce,
		Verifier:  verifier,
		ExpiresAt: time.Now().Add(oidcStateTTL),
	}, nil
}

// NewOIDCProvider is NewProvider for id_tokens: when no audience is configured
// the id_token must be issued for this client.
func NewOIDCProvider(cfg Config) *Provider {
	if cfg.Audience == "" {
		cfg.Audience = cfg.OIDCClientID
	}
	return NewProvider(cfg)
}

// ExchangeCode redeems an authorization code at the token endpoint and returns
// the id_token. Neither the code, the secret nor the response is logged or
// included in returned errors.
func (p *Provider) ExchangeCode(ctx context.Context, code, verifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.cfg.OIDCRedirectURI},
		"client_id":     {p.cfg.OIDCClientID},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.OIDCTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", ErrIdPUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if p.cfg.OIDCClientSecret != "" {
		req.SetBasicAuth(url.QueryEscape(p.cfg.OIDCClientID), url.QueryEscape(p.cfg.OIDCClientSecret))
	}
	resp, err := oidcHTTPClient.Do(req)
	if err != nil {
		return "", ErrIdPUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenRespBytes))
	if err != nil {
		return "", ErrIdPUnavailable
	}
	switch {
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return "", ErrCodeExchange
	case resp.StatusCode != http.StatusOK:
		return "", ErrIdPUnavailable
	}
	var out struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.IDToken == "" {
		return "", ErrInvalidToken
	}
	return out.IDToken, nil
}

// oidcHTTPClient talks to the token endpoint. Redirects are never followed:
// a token endpoint has no legitimate reason to redirect, and following one
// would resend the code and client secret to whatever host it names.
var oidcHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// --- state persistence ---

// oidcMu serialises take-and-delete so a replayed state cannot be redeemed
// twice on this node. Across HA nodes the IdP single-use authorization code
// is the backstop.
var oidcMu sync.Mutex

func oidcStateKey(id string) string {
	sum := sha256.Sum256([]byte(id))
	return oidcStatePrefix + hex.EncodeToString(sum[:])
}

// PutOIDCState persists st under the state id. It sweeps expired records and
// refuses once too many are pending.
func (s *Store) PutOIDCState(ctx context.Context, id string, st *OIDCState) error {
	keys, err := s.b.List(ctx, oidcStatePrefix)
	if err != nil {
		return err
	}
	if len(keys) >= oidcSweepAbove {
		live := 0
		for _, k := range keys {
			e, err := s.b.Get(ctx, k)
			if err != nil || e == nil {
				continue
			}
			var old OIDCState
			if json.Unmarshal(e.Value, &old) != nil || time.Now().After(old.ExpiresAt) {
				_ = s.b.Delete(ctx, k)
				continue
			}
			live++
		}
		if live >= maxOIDCStates {
			return ErrTooManyPending
		}
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.b.Put(ctx, &physical.Entry{Key: oidcStateKey(id), Value: data})
}

// TakeOIDCState returns and deletes the record for id; a second call with the
// same id fails with ErrInvalidState.
func (s *Store) TakeOIDCState(ctx context.Context, id string) (*OIDCState, error) {
	if id == "" {
		return nil, ErrInvalidState
	}
	oidcMu.Lock()
	defer oidcMu.Unlock()
	key := oidcStateKey(id)
	e, err := s.b.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrInvalidState
	}
	if err := s.b.Delete(ctx, key); err != nil {
		return nil, err
	}
	var st OIDCState
	if err := json.Unmarshal(e.Value, &st); err != nil || time.Now().After(st.ExpiresAt) {
		return nil, ErrInvalidState
	}
	return &st, nil
}
