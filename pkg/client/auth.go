package client

import (
	"context"
	"time"
)

// AuthClient provides methods for authenticating against Tuck auth backends.
type AuthClient struct{ c *Client }

// Auth returns a client for authentication operations.
func (c *Client) Auth() *AuthClient { return &AuthClient{c: c} }

// LoginResult holds the token returned by any auth backend login.
type LoginResult struct {
	Token       string        `json:"token"`
	DisplayName string        `json:"display_name"`
	Policies    []string      `json:"policies"`
	TTL         time.Duration `json:"ttl"`
	ExpiresAt   string        `json:"expires_at"`
}

// LoginAppRole exchanges a role_id / secret_id pair for a token.
func (a *AuthClient) LoginAppRole(ctx context.Context, roleID, secretID string) (*LoginResult, error) {
	var res LoginResult
	return &res, a.c.post(ctx, "/v1/auth/approle/login", map[string]string{
		"role_id":   roleID,
		"secret_id": secretID,
	}, &res)
}

// LoginLDAP authenticates with LDAP credentials and returns a token.
func (a *AuthClient) LoginLDAP(ctx context.Context, username, password string) (*LoginResult, error) {
	var res LoginResult
	return &res, a.c.post(ctx, "/v1/auth/ldap/login", map[string]string{
		"username": username,
		"password": password,
	}, &res)
}

// LoginJWT authenticates with a pre-signed JWT and a role name.
func (a *AuthClient) LoginJWT(ctx context.Context, jwt, role string) (*LoginResult, error) {
	var res LoginResult
	return &res, a.c.post(ctx, "/v1/auth/jwt/login", map[string]string{
		"token": jwt,
		"role":  role,
	}, &res)
}

// LoginGitHub authenticates with a GitHub Actions OIDC token and a role name.
func (a *AuthClient) LoginGitHub(ctx context.Context, oidcToken, role string) (*LoginResult, error) {
	var res LoginResult
	return &res, a.c.post(ctx, "/v1/auth/github/login", map[string]string{
		"token": oidcToken,
		"role":  role,
	}, &res)
}

// LookupSelf returns information about the token currently set on the client.
func (c *Client) LookupSelf(ctx context.Context) (*Token, error) {
	var tok Token
	return &tok, c.get(ctx, "/v1/auth/token/self", &tok)
}

// RenewSelf extends the expiry of the token currently set on the client.
// ttl=0 uses the server default.
func (c *Client) RenewSelf(ctx context.Context, ttl time.Duration) (*Token, error) {
	body := map[string]string{}
	if ttl > 0 {
		body["ttl"] = ttl.String()
	}
	var tok Token
	return &tok, c.post(ctx, "/v1/auth/token/self/renew", body, &tok)
}
