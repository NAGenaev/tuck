package jwt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// DefaultTTL is the TTL applied to issued Tuck tokens when the role does not
	// specify one. Defaults to 1 hour.
	DefaultTTL time.Duration `json:"default_ttl,omitempty"`
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
	// Policies are the Tuck policy names granted on login.
	Policies []string `json:"policies"`
	// TTL is the lifetime of issued Tuck tokens. Zero uses Config.DefaultTTL.
	TTL time.Duration `json:"ttl,omitempty"`
}

// LoginResult is returned by Provider.Login on success.
type LoginResult struct {
	Subject  string
	Policies []string
	TTL      time.Duration
	Groups   []string // extracted from the "groups" JWT claim, if present
}

// Provider validates JWTs and resolves the matching Role.
type Provider struct {
	cfg  Config
	jwks *JWKS
}

// NewProvider creates a Provider from cfg.
func NewProvider(cfg Config) *Provider {
	var jwks *JWKS
	if cfg.JWKSURI != "" {
		jwks = NewJWKS(cfg.JWKSURI, 10*time.Minute, nil)
	}
	return &Provider{cfg: cfg, jwks: jwks}
}

// Login validates token, matches it against roles, and returns the login result.
func (p *Provider) Login(ctx context.Context, tokenStr string, roles []*Role) (*LoginResult, error) {
	claims, err := p.validate(ctx, tokenStr)
	if err != nil {
		return nil, err
	}

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
		Subject:  sub,
		Policies: role.Policies,
		TTL:      ttl,
		Groups:   extractGroups(claims),
	}, nil
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
