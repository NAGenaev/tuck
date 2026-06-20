package client

import (
	"context"
	"time"
)

// GCPClient manages the GCP dynamic secrets engine.
type GCPClient struct{ c *Client }

// GCP returns a client for the GCP dynamic secrets engine.
func (c *Client) GCP() *GCPClient { return &GCPClient{c: c} }

// GCPConfig holds credentials for the GCP secrets engine.
type GCPConfig struct {
	CredentialsJSON string `json:"credentials_json,omitempty"`
}

// GCPRole defines how GCP credentials are generated.
type GCPRole struct {
	Name                string        `json:"name"`
	CredentialType      string        `json:"credential_type"` // "service_account_key" or "access_token"
	ServiceAccountEmail string        `json:"service_account_email"`
	KeyAlgorithm        string        `json:"key_algorithm,omitempty"`
	Scopes              []string      `json:"scopes,omitempty"`
	DefaultTTL          time.Duration `json:"default_ttl,omitempty"`
	MaxTTL              time.Duration `json:"max_ttl,omitempty"`
}

// GCPCredentials is returned when generating dynamic GCP credentials.
type GCPCredentials struct {
	LeaseID        string    `json:"lease_id"`
	CredentialType string    `json:"credential_type"`
	PrivateKeyData string    `json:"private_key_data,omitempty"` // base64 JSON key (service_account_key)
	Token          string    `json:"token,omitempty"`             // access_token
	ExpiresAt      time.Time `json:"expires_at"`
}

// PutConfig sets the GCP engine credentials.
func (g *GCPClient) PutConfig(ctx context.Context, cfg GCPConfig) error {
	return g.c.putJSON(ctx, "/v1/gcp/config", cfg)
}

// GetConfig reads the current GCP engine config.
func (g *GCPClient) GetConfig(ctx context.Context) (*GCPConfig, error) {
	var out GCPConfig
	if err := g.c.get(ctx, "/v1/gcp/config", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteConfig removes the GCP engine config.
func (g *GCPClient) DeleteConfig(ctx context.Context) error {
	return g.c.doDelete(ctx, "/v1/gcp/config")
}

// PutRole creates or updates a GCP role.
func (g *GCPClient) PutRole(ctx context.Context, role GCPRole) error {
	return g.c.putJSON(ctx, "/v1/gcp/roles/"+role.Name, role)
}

// GetRole reads a GCP role.
func (g *GCPClient) GetRole(ctx context.Context, name string) (*GCPRole, error) {
	var out GCPRole
	if err := g.c.get(ctx, "/v1/gcp/roles/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRole removes a GCP role.
func (g *GCPClient) DeleteRole(ctx context.Context, name string) error {
	return g.c.doDelete(ctx, "/v1/gcp/roles/"+name)
}

// ListRoles returns all GCP role names.
func (g *GCPClient) ListRoles(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := g.c.doList(ctx, "/v1/gcp/roles/", &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// GenerateCreds generates dynamic GCP credentials for the named role.
func (g *GCPClient) GenerateCreds(ctx context.Context, role string) (*GCPCredentials, error) {
	var out GCPCredentials
	if err := g.c.post(ctx, "/v1/gcp/creds/"+role, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeLease revokes a GCP credential lease before its TTL expires.
func (g *GCPClient) RevokeLease(ctx context.Context, leaseID string) error {
	return g.c.doDelete(ctx, "/v1/gcp/lease/"+leaseID)
}
