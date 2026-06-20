package client

import (
	"context"
	"time"
)

// AppRoleClient manages AppRole roles and secret IDs (not login — see Auth().LoginAppRole).
type AppRoleClient struct{ c *Client }

// AppRole returns a client for AppRole role management.
func (c *Client) AppRole() *AppRoleClient { return &AppRoleClient{c: c} }

// AppRoleRole defines an AppRole role.
type AppRoleRole struct {
	Name            string        `json:"name"`
	RoleID          string        `json:"role_id,omitempty"`
	Policies        []string      `json:"policies"`
	TokenTTL        time.Duration `json:"token_ttl,omitempty"`
	SecretIDTTL     time.Duration `json:"secret_id_ttl,omitempty"`
	SecretIDNumUses int           `json:"secret_id_num_uses,omitempty"`
}

// AppRoleSecretID is a generated SecretID for an AppRole role.
type AppRoleSecretID struct {
	ID        string    `json:"id"`
	RoleName  string    `json:"role_name"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	NumUses   int       `json:"num_uses,omitempty"`
	UsesLeft  int       `json:"uses_left,omitempty"`
}

// PutRole creates or updates an AppRole role.
func (a *AppRoleClient) PutRole(ctx context.Context, role AppRoleRole) error {
	return a.c.putJSON(ctx, "/v1/auth/approle/role/"+role.Name, role)
}

// GetRole reads an AppRole role.
func (a *AppRoleClient) GetRole(ctx context.Context, name string) (*AppRoleRole, error) {
	var out AppRoleRole
	if err := a.c.get(ctx, "/v1/auth/approle/role/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRole removes an AppRole role.
func (a *AppRoleClient) DeleteRole(ctx context.Context, name string) error {
	return a.c.doDelete(ctx, "/v1/auth/approle/role/"+name)
}

// ListRoles returns all AppRole role names.
func (a *AppRoleClient) ListRoles(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := a.c.doList(ctx, "/v1/auth/approle/role/", &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// GenerateSecretID generates a new SecretID for the named role.
func (a *AppRoleClient) GenerateSecretID(ctx context.Context, role string) (*AppRoleSecretID, error) {
	var out AppRoleSecretID
	if err := a.c.post(ctx, "/v1/auth/approle/role/"+role+"/secret-id", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// LookupSecretID reads metadata for a SecretID.
func (a *AppRoleClient) LookupSecretID(ctx context.Context, role, id string) (*AppRoleSecretID, error) {
	var out AppRoleSecretID
	if err := a.c.get(ctx, "/v1/auth/approle/role/"+role+"/secret-id/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DestroySecretID permanently revokes a SecretID.
func (a *AppRoleClient) DestroySecretID(ctx context.Context, role, id string) error {
	return a.c.doDelete(ctx, "/v1/auth/approle/role/"+role+"/secret-id/"+id)
}
