// Package github implements GitHub Actions OIDC authentication for Tuck.
//
// GitHub Actions emits short-lived OIDC tokens signed by GitHub's key pair.
// The issuer and JWKS URL are fixed; callers only need to define roles that
// bind GitHub-specific claims (repository, ref, environment, etc.) to Tuck
// policies.
//
// Verification chain:
//  1. Fetch GitHub's JWKS from the canonical endpoint.
//  2. Validate the JWT signature, iss, aud and expiry.
//  3. Match the validated claims against the named role.
//  4. Return policies + TTL to the caller.
package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	jwtpkg "github.com/NAGenaev/tuck/internal/auth/jwt"
)

const (
	// GitHubIssuer is the fixed issuer for all GitHub Actions OIDC tokens.
	GitHubIssuer = "https://token.actions.githubusercontent.com"

	// GitHubJWKSURI is the canonical JWKS endpoint for GitHub Actions.
	GitHubJWKSURI = "https://token.actions.githubusercontent.com/.well-known/jwks"
)

var (
	// ErrRoleNotFound is returned when the requested role does not exist.
	ErrRoleNotFound = errors.New("github auth: role not found")
	// ErrNoMatch is returned when the token claims do not satisfy the role.
	ErrNoMatch = errors.New("github auth: token claims do not match role constraints")
	// ErrInvalidToken is returned when the JWT fails cryptographic validation.
	ErrInvalidToken = errors.New("github auth: invalid or expired token")
)

// Role defines which GitHub Actions claims are allowed to log in, and which
// Tuck policies the resulting token should carry.
type Role struct {
	Name string `json:"name"`

	// GitHub claim matchers — all non-empty fields must match exactly.
	// An empty field is a wildcard (any value accepted).
	Repository      string `json:"repository,omitempty"`       // e.g. "myorg/myrepo"
	RepositoryOwner string `json:"repository_owner,omitempty"` // e.g. "myorg"
	Ref             string `json:"ref,omitempty"`              // e.g. "refs/heads/main"
	Environment     string `json:"environment,omitempty"`      // e.g. "production"
	WorkflowRef     string `json:"workflow_ref,omitempty"`     // full path@ref
	Actor           string `json:"actor,omitempty"`            // GitHub username
	// Audience that must appear in the "aud" claim.
	// Defaults to GitHubIssuer if empty.
	Audience string `json:"audience,omitempty"`

	// Tuck policies granted to the resulting token.
	Policies []string `json:"policies"`
	// TTL of the resulting Tuck token. Defaults to 1 hour.
	TTL time.Duration `json:"ttl,omitempty"`
}

// LoginResult is returned by Provider.Login on success.
type LoginResult struct {
	Subject  string
	Policies []string
	TTL      time.Duration
}

// Provider validates GitHub Actions OIDC tokens against a named Role.
type Provider struct {
	jwtProvider *jwtpkg.Provider
}

// NewProvider creates a Provider.
// The JWT provider is pre-configured with the GitHub issuer and JWKS URI.
func NewProvider() *Provider {
	cfg := jwtpkg.Config{
		JWKSURI: GitHubJWKSURI,
		Issuer:  GitHubIssuer,
	}
	return &Provider{jwtProvider: jwtpkg.NewProvider(cfg)}
}

// Login validates tokenStr, matches it against role, and returns the login result.
func (p *Provider) Login(ctx context.Context, tokenStr string, role *Role) (*LoginResult, error) {
	if role == nil {
		return nil, ErrRoleNotFound
	}

	aud := role.Audience
	if aud == "" {
		aud = GitHubIssuer
	}

	// Build a JWT role with GitHub claim bindings.
	bound := boundClaims(role)
	jwtRole := &jwtpkg.Role{
		Name:           role.Name,
		BoundClaims:    bound,
		BoundAudiences: []string{aud},
		Policies:       role.Policies,
		TTL:            role.TTL,
	}

	res, err := p.jwtProvider.Login(ctx, tokenStr, []*jwtpkg.Role{jwtRole})
	if err != nil {
		if errors.Is(err, jwtpkg.ErrInvalidToken) {
			return nil, ErrInvalidToken
		}
		if errors.Is(err, jwtpkg.ErrNoRole) {
			return nil, ErrNoMatch
		}
		return nil, fmt.Errorf("github auth: %w", err)
	}

	ttl := res.TTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &LoginResult{
		Subject:  res.Subject,
		Policies: res.Policies,
		TTL:      ttl,
	}, nil
}

// boundClaims converts the GitHub-specific role fields into a generic
// BoundClaims map that the JWT provider can validate.
func boundClaims(r *Role) map[string]string {
	m := make(map[string]string)
	set := func(k, v string) {
		if v != "" {
			m[k] = v
		}
	}
	set("repository", r.Repository)
	set("repository_owner", r.RepositoryOwner)
	set("ref", r.Ref)
	set("environment", r.Environment)
	set("workflow_ref", r.WorkflowRef)
	set("actor", r.Actor)
	return m
}

// SubjectFor builds the GitHub sub claim for documentation / debugging.
// GitHub's sub has the form "repo:<owner>/<repo>:environment:<env>" etc.
func SubjectFor(repository, qualifier, value string) string {
	if qualifier == "" || value == "" {
		return "repo:" + repository
	}
	return strings.Join([]string{"repo:" + repository, qualifier + ":" + value}, ":")
}
