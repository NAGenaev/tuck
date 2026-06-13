package client

import (
	"context"
	"time"
)

// TokenRole is a named token template.
type TokenRole struct {
	Name      string        `json:"name"`
	Policies  []string      `json:"policies"`
	TTL       time.Duration `json:"ttl,omitempty"`
	MaxTTL    time.Duration `json:"max_ttl,omitempty"`
	MaxUses   int           `json:"max_uses,omitempty"`
	Renewable bool          `json:"renewable"`
	Period    time.Duration `json:"period,omitempty"`
}

// TokenRoles returns a client for token role management.
func (c *Client) TokenRoles() *TokenRoleClient { return &TokenRoleClient{c: c} }

// TokenRoleClient manages Tuck token roles.
type TokenRoleClient struct{ c *Client }

// Put creates or updates a token role.
func (r *TokenRoleClient) Put(ctx context.Context, role TokenRole) error {
	return r.c.putJSON(ctx, "/v1/auth/token/roles/"+role.Name, role)
}

// Get returns a token role by name.
func (r *TokenRoleClient) Get(ctx context.Context, name string) (*TokenRole, error) {
	var role TokenRole
	return &role, r.c.get(ctx, "/v1/auth/token/roles/"+name, &role)
}

// Delete removes a token role.
func (r *TokenRoleClient) Delete(ctx context.Context, name string) error {
	return r.c.doDelete(ctx, "/v1/auth/token/roles/"+name)
}

// List returns all token role names.
func (r *TokenRoleClient) List(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	return resp.Keys, r.c.doList(ctx, "/v1/auth/token/roles/", &resp)
}

// CreateToken issues a token using the named role's template.
func (r *TokenRoleClient) CreateToken(ctx context.Context, roleName string) (*Token, error) {
	var tok Token
	return &tok, r.c.post(ctx, "/v1/auth/token/roles/"+roleName+"/create", nil, &tok)
}
