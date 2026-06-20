package client

import "context"

// SSHClient manages the SSH secrets engine.
type SSHClient struct{ c *Client }

// SSH returns a client for the SSH secrets engine.
func (c *Client) SSH() *SSHClient { return &SSHClient{c: c} }

// SSHRole defines the signing policy for SSH certificates.
type SSHRole struct {
	Name            string   `json:"name"`
	KeyType         string   `json:"key_type"`              // "ca"
	AllowedUsers    string   `json:"allowed_users,omitempty"`
	AllowedDomains  string   `json:"allowed_domains,omitempty"`
	DefaultUser     string   `json:"default_user,omitempty"`
	TTL             string   `json:"ttl,omitempty"`
	MaxTTL          string   `json:"max_ttl,omitempty"`
	AllowUserCerts  bool     `json:"allow_user_certificates,omitempty"`
	AllowHostCerts  bool     `json:"allow_host_certificates,omitempty"`
	Extensions      map[string]string `json:"extension,omitempty"`
	DefaultExtensions map[string]string `json:"default_extensions,omitempty"`
}

// SSHSignRequest specifies parameters for signing an SSH public key.
type SSHSignRequest struct {
	PublicKey   string   `json:"public_key"`
	ValidPrincipals string `json:"valid_principals,omitempty"`
	TTL         string   `json:"ttl,omitempty"`
	CertType    string   `json:"cert_type,omitempty"` // "user" or "host"
	Extensions  map[string]string `json:"extensions,omitempty"`
}

// SSHSignResult is returned after signing an SSH key.
type SSHSignResult struct {
	SignedKey    string `json:"signed_key"`
	SerialNumber string `json:"serial_number"`
}

// GenerateCA generates a new SSH CA key pair within the engine.
func (s *SSHClient) GenerateCA(ctx context.Context) error {
	return s.c.post(ctx, "/v1/ssh/generate/ca", nil, nil)
}

// GetCAPublicKey returns the SSH CA public key in OpenSSH format.
func (s *SSHClient) GetCAPublicKey(ctx context.Context) (string, error) {
	var resp struct {
		PublicKey string `json:"public_key"`
	}
	if err := s.c.get(ctx, "/v1/ssh/ca/public-key", &resp); err != nil {
		return "", err
	}
	return resp.PublicKey, nil
}

// PutRole creates or updates an SSH signing role.
func (s *SSHClient) PutRole(ctx context.Context, role SSHRole) error {
	return s.c.putJSON(ctx, "/v1/ssh/roles/"+role.Name, role)
}

// GetRole reads an SSH role.
func (s *SSHClient) GetRole(ctx context.Context, name string) (*SSHRole, error) {
	var out SSHRole
	if err := s.c.get(ctx, "/v1/ssh/roles/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRole removes an SSH role.
func (s *SSHClient) DeleteRole(ctx context.Context, name string) error {
	return s.c.doDelete(ctx, "/v1/ssh/roles/"+name)
}

// Sign signs an SSH public key using the named role and returns the certificate.
func (s *SSHClient) Sign(ctx context.Context, role string, req SSHSignRequest) (*SSHSignResult, error) {
	var out SSHSignResult
	if err := s.c.post(ctx, "/v1/ssh/sign/"+role, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
