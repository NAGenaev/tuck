package client

import (
	"context"
	"time"
)

// AzureClient manages the Azure dynamic secrets engine.
type AzureClient struct{ c *Client }

// Azure returns a client for the Azure dynamic secrets engine.
func (c *Client) Azure() *AzureClient { return &AzureClient{c: c} }

// AzureConfig holds Azure AD credentials for the secrets engine.
type AzureConfig struct {
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// AzureRole defines how Azure AD credentials are generated.
type AzureRole struct {
	Name                string        `json:"name"`
	ApplicationObjectID string        `json:"application_object_id"`
	ApplicationID       string        `json:"application_id"`
	DefaultTTL          time.Duration `json:"default_ttl,omitempty"`
	MaxTTL              time.Duration `json:"max_ttl,omitempty"`
}

// AzureCredentials is returned when generating dynamic Azure credentials.
type AzureCredentials struct {
	LeaseID      string    `json:"lease_id"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// PutConfig sets the Azure engine root credentials.
func (a *AzureClient) PutConfig(ctx context.Context, cfg AzureConfig) error {
	return a.c.putJSON(ctx, "/v1/azure/config", cfg)
}

// GetConfig reads the current Azure engine config (secrets are redacted).
func (a *AzureClient) GetConfig(ctx context.Context) (*AzureConfig, error) {
	var out AzureConfig
	if err := a.c.get(ctx, "/v1/azure/config", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteConfig removes the Azure engine config.
func (a *AzureClient) DeleteConfig(ctx context.Context) error {
	return a.c.doDelete(ctx, "/v1/azure/config")
}

// PutRole creates or updates an Azure role.
func (a *AzureClient) PutRole(ctx context.Context, role AzureRole) error {
	return a.c.putJSON(ctx, "/v1/azure/roles/"+role.Name, role)
}

// GetRole reads an Azure role.
func (a *AzureClient) GetRole(ctx context.Context, name string) (*AzureRole, error) {
	var out AzureRole
	if err := a.c.get(ctx, "/v1/azure/roles/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRole removes an Azure role.
func (a *AzureClient) DeleteRole(ctx context.Context, name string) error {
	return a.c.doDelete(ctx, "/v1/azure/roles/"+name)
}

// ListRoles returns all Azure role names.
func (a *AzureClient) ListRoles(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := a.c.doList(ctx, "/v1/azure/roles/", &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// GenerateCreds generates dynamic Azure credentials for the named role.
func (a *AzureClient) GenerateCreds(ctx context.Context, role string) (*AzureCredentials, error) {
	var out AzureCredentials
	if err := a.c.post(ctx, "/v1/azure/creds/"+role, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeLease revokes an Azure credential lease before its TTL expires.
func (a *AzureClient) RevokeLease(ctx context.Context, leaseID string) error {
	return a.c.doDelete(ctx, "/v1/azure/lease/"+leaseID)
}
