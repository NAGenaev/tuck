package client

import "context"

// TOTPClient manages the TOTP secrets engine.
type TOTPClient struct{ c *Client }

// TOTP returns a client for the TOTP secrets engine.
func (c *Client) TOTP() *TOTPClient { return &TOTPClient{c: c} }

// TOTPKeyRequest configures a TOTP key to create or import.
type TOTPKeyRequest struct {
	Issuer    string `json:"issuer,omitempty"`
	AccountName string `json:"account_name,omitempty"`
	Algorithm string `json:"algorithm,omitempty"` // "SHA1" (default), "SHA256", "SHA512"
	Digits    int    `json:"digits,omitempty"`    // 6 (default) or 8
	Period    int    `json:"period,omitempty"`    // seconds (default 30)
	// Key is the base32-encoded secret for importing; omit to auto-generate.
	Key       string `json:"key,omitempty"`
}

// TOTPKey describes a stored TOTP key.
type TOTPKey struct {
	Name      string `json:"name"`
	Issuer    string `json:"issuer"`
	AccountName string `json:"account_name"`
	Algorithm string `json:"algorithm"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	// URL is the otpauth:// URL (only returned on creation).
	URL       string `json:"url,omitempty"`
}

// CreateKey creates (or imports) a TOTP key.
func (t *TOTPClient) CreateKey(ctx context.Context, name string, req TOTPKeyRequest) (*TOTPKey, error) {
	var out TOTPKey
	if err := t.c.post(ctx, "/v1/totp/keys/"+name, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetKey reads a TOTP key's metadata (the secret is never returned).
func (t *TOTPClient) GetKey(ctx context.Context, name string) (*TOTPKey, error) {
	var out TOTPKey
	if err := t.c.get(ctx, "/v1/totp/keys/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteKey removes a TOTP key.
func (t *TOTPClient) DeleteKey(ctx context.Context, name string) error {
	return t.c.doDelete(ctx, "/v1/totp/keys/"+name)
}

// GenerateCode generates the current TOTP code for the named key.
func (t *TOTPClient) GenerateCode(ctx context.Context, name string) (string, error) {
	var resp struct {
		Code string `json:"code"`
	}
	if err := t.c.get(ctx, "/v1/totp/code/"+name, &resp); err != nil {
		return "", err
	}
	return resp.Code, nil
}

// ValidateCode validates a TOTP code for the named key.
func (t *TOTPClient) ValidateCode(ctx context.Context, name, code string) (bool, error) {
	var resp struct {
		Valid bool `json:"valid"`
	}
	if err := t.c.post(ctx, "/v1/totp/code/"+name, map[string]string{"code": code}, &resp); err != nil {
		return false, err
	}
	return resp.Valid, nil
}
