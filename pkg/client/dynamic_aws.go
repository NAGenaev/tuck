package client

import (
	"context"
	"time"
)

// AWSClient manages the AWS dynamic secrets engine.
type AWSClient struct{ c *Client }

// AWS returns a client for the AWS dynamic secrets engine.
func (c *Client) AWS() *AWSClient { return &AWSClient{c: c} }

// AWSConfig holds AWS credentials for the secrets engine.
type AWSConfig struct {
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	Region          string `json:"region"`
	IAMEndpoint     string `json:"iam_endpoint,omitempty"`
	STSEndpoint     string `json:"sts_endpoint,omitempty"`
}

// AWSRole defines how AWS credentials are generated.
type AWSRole struct {
	Name           string        `json:"name"`
	CredentialType string        `json:"credential_type"` // "iam_user" or "assumed_role"
	PolicyARNs     []string      `json:"policy_arns,omitempty"`
	PolicyDocument string        `json:"policy_document,omitempty"`
	RoleARNs       []string      `json:"role_arns,omitempty"`
	DefaultTTL     time.Duration `json:"default_ttl,omitempty"`
	MaxTTL         time.Duration `json:"max_ttl,omitempty"`
}

// AWSCredentials is returned when generating dynamic AWS credentials.
type AWSCredentials struct {
	LeaseID         string    `json:"lease_id"`
	AccessKeyID     string    `json:"access_key_id"`
	SecretAccessKey string    `json:"secret_access_key"`
	SessionToken    string    `json:"session_token,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// PutConfig sets the AWS engine root credentials.
func (a *AWSClient) PutConfig(ctx context.Context, cfg AWSConfig) error {
	return a.c.putJSON(ctx, "/v1/aws/config", cfg)
}

// GetConfig reads the current AWS engine config (credentials are redacted).
func (a *AWSClient) GetConfig(ctx context.Context) (*AWSConfig, error) {
	var out AWSConfig
	if err := a.c.get(ctx, "/v1/aws/config", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteConfig removes the AWS engine config.
func (a *AWSClient) DeleteConfig(ctx context.Context) error {
	return a.c.doDelete(ctx, "/v1/aws/config")
}

// PutRole creates or updates an AWS role.
func (a *AWSClient) PutRole(ctx context.Context, role AWSRole) error {
	return a.c.putJSON(ctx, "/v1/aws/roles/"+role.Name, role)
}

// GetRole reads an AWS role.
func (a *AWSClient) GetRole(ctx context.Context, name string) (*AWSRole, error) {
	var out AWSRole
	if err := a.c.get(ctx, "/v1/aws/roles/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRole removes an AWS role.
func (a *AWSClient) DeleteRole(ctx context.Context, name string) error {
	return a.c.doDelete(ctx, "/v1/aws/roles/"+name)
}

// ListRoles returns all AWS role names.
func (a *AWSClient) ListRoles(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := a.c.doList(ctx, "/v1/aws/roles/", &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// GenerateCreds generates dynamic AWS credentials for the named role.
func (a *AWSClient) GenerateCreds(ctx context.Context, role string) (*AWSCredentials, error) {
	var out AWSCredentials
	if err := a.c.post(ctx, "/v1/aws/creds/"+role, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeLease revokes an AWS credential lease before its TTL expires.
func (a *AWSClient) RevokeLease(ctx context.Context, leaseID string) error {
	return a.c.doDelete(ctx, "/v1/aws/lease/"+leaseID)
}
