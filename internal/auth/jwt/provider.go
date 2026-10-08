package jwt

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is returned when a JWT fails validation.
var ErrInvalidToken = errors.New("jwt: invalid or expired token")

// ErrNoRole is returned when no role matches the JWT claims.
var ErrNoRole = errors.New("jwt: no matching role for token claims")

// Config holds the provider-level settings for JWT authentication.
type Config struct {
	// JWKSURI is the HTTPS URL of the JWKS endpoint
	// (e.g. "https://keycloak.example.com/realms/myrealm/protocol/openid-connect/certs").
	// Exactly one of JWKSURI or StaticKeys must be set.
	JWKSURI string `json:"jwks_uri"`
	// Issuer is the expected "iss" claim. Empty means any issuer is accepted.
	Issuer string `json:"issuer,omitempty"`
	// Audience is the expected "aud" claim. Empty means any audience is accepted.
	Audience string `json:"audience,omitempty"`
	// CACert is a PEM bundle of the certificate authorities that sign the
	// identity provider's certificates (typically the bank's internal CA). It
	// is used for the JWKS fetch and for the OIDC token endpoint; when set,
	// only these CAs are trusted. Empty = the system trust store.
	CACert string `json:"ca_cert,omitempty"`
	// DefaultTTL is the TTL applied to issued Tuck tokens when the role does not
	// specify one. Defaults to 1 hour.
	DefaultTTL time.Duration `json:"default_ttl,omitempty"`

	// Browser (authorization-code + PKCE) login. All optional; when
	// OIDCAuthorizationURL is empty the OIDC flow is disabled and only
	// direct JWT login works. See Config.ValidateOIDC.
	OIDCAuthorizationURL string `json:"oidc_authorization_url,omitempty"`
	OIDCTokenURL         string `json:"oidc_token_url,omitempty"`
	OIDCClientID         string `json:"oidc_client_id,omitempty"`
	// OIDCClientSecret is empty for a public client (PKCE only). Never
	// returned by the API.
	OIDCClientSecret string `json:"oidc_client_secret,omitempty"`
	// OIDCRedirectURI is the exact redirect URI registered at the IdP; it
	// points at the web UI callback route.
	OIDCRedirectURI string `json:"oidc_redirect_uri,omitempty"`
	// OIDCScopes is a space-separated list. "openid" is always requested.
	OIDCScopes string `json:"oidc_scopes,omitempty"`
}

// Role binds a set of JWT claims to Tuck policies.
type Role struct {
	Name string `json:"name"`
	// BoundSubject restricts the role to JWTs with a specific "sub" claim.
	// Empty accepts any subject.
	BoundSubject string `json:"bound_subject,omitempty"`
	// BoundClaims are additional JWT claims that must all be present and equal.
	// Example: {"group": "admin", "tenant": "acme"}.
	BoundClaims map[string]string `json:"bound_claims,omitempty"`
	// BoundAudiences is a list of accepted "aud" values. Empty accepts any.
	BoundAudiences []string `json:"bound_audiences,omitempty"`
	// BoundGroups accepts a JWT whose "groups" claim contains ANY of these
	// values (exact match). Empty means no group constraint.
	BoundGroups []string `json:"bound_groups,omitempty"`
	// Policies are the Tuck policy names granted on login.
	Policies []string `json:"policies"`
	// TTL is the lifetime of issued Tuck tokens. Zero uses Config.DefaultTTL.
	TTL time.Duration `json:"ttl,omitempty"`
	// TuckNamespace binds issued tokens to this Tuck namespace (must exist).
	// Empty = root namespace. Never taken from the client.
	TuckNamespace string `json:"tuck_namespace,omitempty"`
}

// LoginResult is returned by Provider.Login on success.
type LoginResult struct {
	Subject       string
	// Name is a human-readable label for the login (preferred_username, else
	// email, else the subject). It names the Tuck token and appears in audit.
	Name          string
	Policies      []string
	TTL           time.Duration
	TuckNamespace string
	Groups        []string // extracted from the "groups" JWT claim, if present
}

// Provider validates JWTs and resolves the matching Role.
type Provider struct {
	cfg        Config
	jwks       *JWKS
	oidcClient *http.Client
}

// ValidateCA checks that ca_cert, if set, holds at least one PEM certificate.
func (c *Config) ValidateCA() error {
	if c.CACert == "" {
		return nil
	}
	if !x509.NewCertPool().AppendCertsFromPEM([]byte(c.CACert)) {
		return errors.New("ca_cert contains no valid PEM certificate")
	}
	return nil
}

// transport returns the HTTP transport for talking to the identity provider,
// trusting only ca_cert when it is set.
func (c *Config) transport() http.RoundTripper {
	if c.CACert == "" {
		return http.DefaultTransport
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(c.CACert)) {
		return failingTransport{} // refused at save time; never fall back to system roots silently
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return tr
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("jwt: ca_cert is not a valid PEM bundle")
}

// NewProvider creates a Provider from cfg.
func NewProvider(cfg Config) *Provider {
	var jwks *JWKS
	rt := cfg.transport()
	if cfg.JWKSURI != "" {
		jwks = NewJWKS(cfg.JWKSURI, 10*time.Minute, &http.Client{Timeout: 10 * time.Second, Transport: rt})
	}
	return &Provider{cfg: cfg, jwks: jwks, oidcClient: &http.Client{
		Timeout:   10 * time.Second,
		Transport: rt,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// Login validates token, matches it against roles, and returns the login result.
func (p *Provider) Login(ctx context.Context, tokenStr string, roles []*Role) (*LoginResult, error) {
	claims, err := p.validate(ctx, tokenStr)
	if err != nil {
		return nil, err
	}
	return p.resolve(claims, roles)
}

// LoginIDToken is Login for an OIDC id_token obtained through the
// authorization-code flow: the token must also carry the nonce bound to the
// login attempt, which defeats replay of an id_token captured elsewhere.
func (p *Provider) LoginIDToken(ctx context.Context, idToken, nonce string, roles []*Role) (*LoginResult, error) {
	claims, err := p.validate(ctx, idToken)
	if err != nil {
		return nil, err
	}
	got, _ := claims["nonce"].(string)
	if nonce == "" || subtle.ConstantTimeCompare([]byte(got), []byte(nonce)) != 1 {
		return nil, ErrInvalidToken
	}
	return p.resolve(claims, roles)
}

func (p *Provider) resolve(claims jwt.MapClaims, roles []*Role) (*LoginResult, error) {
	role := matchRole(claims, roles)
	if role == nil {
		return nil, ErrNoRole
	}

	sub, _ := claims.GetSubject()
	ttl := role.TTL
	if ttl <= 0 {
		ttl = p.cfg.DefaultTTL
		if ttl <= 0 {
			ttl = time.Hour
		}
	}
	return &LoginResult{
		Subject:       sub,
		Name:          displayName(claims, sub),
		Policies:      role.Policies,
		TTL:           ttl,
		TuckNamespace: role.TuckNamespace,
		Groups:        extractGroups(claims),
	}, nil
}

// displayName picks a readable label for the person or workload behind a JWT.
// IdP subjects are often opaque UUIDs, which are useless in an audit trail.
func displayName(claims jwt.MapClaims, sub string) string {
	for _, k := range []string{"preferred_username", "email"} {
		if v, ok := claims[k].(string); ok {
			v = strings.TrimSpace(v)
			if v != "" && len(v) <= 128 && !strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
				return v
			}
		}
	}
	return sub
}

func (p *Provider) validate(ctx context.Context, tokenStr string) (jwt.MapClaims, error) {
	var parserOpts []jwt.ParserOption
	if p.cfg.Issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(p.cfg.Issuer))
	}
	if p.cfg.Audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(p.cfg.Audience))
	}
	parserOpts = append(parserOpts, jwt.WithExpirationRequired())

	tok, err := jwt.ParseWithClaims(tokenStr, jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
		if p.jwks == nil {
			return nil, fmt.Errorf("jwt: no JWKS configured")
		}
		// Reject tokens with no kid — prevents algorithm confusion.
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, fmt.Errorf("jwt: missing kid header")
		}
		return p.jwks.GetKey(ctx, kid)
	}, parserOpts...)

	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// matchRole returns the first role whose bound claims are satisfied by claims.
func matchRole(claims jwt.MapClaims, roles []*Role) *Role {
	for _, role := range roles {
		if roleMatches(claims, role) {
			return role
		}
	}
	return nil
}

func roleMatches(claims jwt.MapClaims, role *Role) bool {
	if role.BoundSubject != "" {
		sub, _ := claims.GetSubject()
		if sub != role.BoundSubject {
			return false
		}
	}
	for k, want := range role.BoundClaims {
		got, err := claimString(claims, k)
		if err != nil || got != want {
			return false
		}
	}
	if len(role.BoundAudiences) > 0 {
		aud, _ := claims.GetAudience()
		if !anyMatch(aud, role.BoundAudiences) {
			return false
		}
	}
	if len(role.BoundGroups) > 0 && !anyMatch(extractGroups(claims), role.BoundGroups) {
		return false
	}
	return true
}

func claimString(claims jwt.MapClaims, key string) (string, error) {
	v, ok := claims[key]
	if !ok {
		return "", fmt.Errorf("claim %q not found", key)
	}
	s, ok := v.(string)
	if !ok {
		// Try JSON encoding for non-string values.
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("claim %q not a string", key)
		}
		return string(b), nil
	}
	return s, nil
}

// extractGroups reads the "groups" claim from a JWT as []string.
// Returns nil when the claim is absent or not a string array.
func extractGroups(claims jwt.MapClaims) []string {
	raw, ok := claims["groups"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	case string:
		return []string{v}
	}
	return nil
}

func anyMatch(haystack, needles []string) bool {
	for _, n := range needles {
		for _, h := range haystack {
			if h == n {
				return true
			}
		}
	}
	return false
}
