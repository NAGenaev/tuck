package client

import "context"

// PKIClient manages the PKI secrets engine.
type PKIClient struct{ c *Client }

// PKI returns a client for the PKI secrets engine.
func (c *Client) PKI() *PKIClient { return &PKIClient{c: c} }

// PKICARequest configures a new CA to generate.
type PKICARequest struct {
	CommonName   string   `json:"common_name"`
	TTL          string   `json:"ttl,omitempty"`          // e.g. "87600h"
	KeyType      string   `json:"key_type,omitempty"`     // "ec" (default) or "rsa"
	KeyBits      int      `json:"key_bits,omitempty"`     // 256, 384, 521 for EC; 2048, 4096 for RSA
	Organization []string `json:"organization,omitempty"`
	Country      []string `json:"country,omitempty"`
}

// PKIRole defines the certificate issuance policy for a named role.
type PKIRole struct {
	Name            string   `json:"name"`
	AllowedDomains  []string `json:"allowed_domains,omitempty"`
	AllowSubdomains bool     `json:"allow_subdomains,omitempty"`
	AllowAnyDomain  bool     `json:"allow_any_domain,omitempty"`
	MaxTTL          string   `json:"max_ttl,omitempty"`
	KeyType         string   `json:"key_type,omitempty"`
	KeyBits         int      `json:"key_bits,omitempty"`
	Organization    []string `json:"organization,omitempty"`
	Country         []string `json:"country,omitempty"`
}

// PKIIssueRequest specifies parameters for issuing a certificate.
type PKIIssueRequest struct {
	CommonName string   `json:"common_name"`
	AltNames   []string `json:"alt_names,omitempty"`
	IPSANs     []string `json:"ip_sans,omitempty"`
	TTL        string   `json:"ttl,omitempty"`
}

// PKICert is a certificate returned by issue or get operations.
type PKICert struct {
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key,omitempty"`
	SerialNumber string `json:"serial_number"`
	CAChain     string `json:"ca_chain,omitempty"`
	Expiration  int64  `json:"expiration,omitempty"`
}

// GenerateCA initialises the PKI engine with a new self-signed CA.
// Returns the CA certificate PEM.
func (p *PKIClient) GenerateCA(ctx context.Context, req PKICARequest) (string, error) {
	var resp struct {
		Certificate string `json:"certificate"`
	}
	if err := p.c.post(ctx, "/v1/pki/generate/root", req, &resp); err != nil {
		return "", err
	}
	return resp.Certificate, nil
}

// GetCACert returns the current CA certificate PEM.
func (p *PKIClient) GetCACert(ctx context.Context) (string, error) {
	var resp struct {
		Certificate string `json:"certificate"`
	}
	if err := p.c.get(ctx, "/v1/pki/ca/pem", &resp); err != nil {
		return "", err
	}
	return resp.Certificate, nil
}

// PutRole creates or updates a PKI role.
func (p *PKIClient) PutRole(ctx context.Context, role PKIRole) error {
	return p.c.putJSON(ctx, "/v1/pki/roles/"+role.Name, role)
}

// GetRole reads a PKI role.
func (p *PKIClient) GetRole(ctx context.Context, name string) (*PKIRole, error) {
	var out PKIRole
	if err := p.c.get(ctx, "/v1/pki/roles/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteRole removes a PKI role.
func (p *PKIClient) DeleteRole(ctx context.Context, name string) error {
	return p.c.doDelete(ctx, "/v1/pki/roles/"+name)
}

// IssueCert issues a certificate for the given role.
func (p *PKIClient) IssueCert(ctx context.Context, role string, req PKIIssueRequest) (*PKICert, error) {
	var out PKICert
	if err := p.c.post(ctx, "/v1/pki/issue/"+role, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeCert revokes a certificate by serial number.
func (p *PKIClient) RevokeCert(ctx context.Context, serial string) error {
	return p.c.post(ctx, "/v1/pki/revoke/"+serial, nil, nil)
}

// GetCert reads a certificate by serial number.
func (p *PKIClient) GetCert(ctx context.Context, serial string) (*PKICert, error) {
	var out PKICert
	if err := p.c.get(ctx, "/v1/pki/certs/"+serial, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
