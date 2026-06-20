package client

import (
	"context"
	"encoding/base64"
)

// TransitClient manages the Transit secrets engine (encryption-as-a-service).
type TransitClient struct{ c *Client }

// Transit returns a client for the Transit secrets engine.
func (c *Client) Transit() *TransitClient { return &TransitClient{c: c} }

// TransitKey describes a Transit encryption key.
type TransitKey struct {
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	LatestVersion   int            `json:"latest_version"`
	MinDecryptVersion int          `json:"min_decryption_version"`
	DeletionAllowed bool           `json:"deletion_allowed"`
	Exportable      bool           `json:"exportable"`
	Keys            map[string]any `json:"keys,omitempty"`
}

// CreateKey creates a Transit encryption key. keyType defaults to "aes256-gcm96" when empty.
func (t *TransitClient) CreateKey(ctx context.Context, name, keyType string) error {
	body := map[string]string{}
	if keyType != "" {
		body["type"] = keyType
	}
	return t.c.post(ctx, "/v1/transit/keys/"+name, body, nil)
}

// GetKey reads metadata for a Transit key.
func (t *TransitClient) GetKey(ctx context.Context, name string) (*TransitKey, error) {
	var out TransitKey
	if err := t.c.get(ctx, "/v1/transit/keys/"+name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteKey deletes a Transit key (the key must have deletion_allowed=true).
func (t *TransitClient) DeleteKey(ctx context.Context, name string) error {
	return t.c.doDelete(ctx, "/v1/transit/keys/"+name)
}

// RotateKey rotates a Transit key, incrementing its version.
func (t *TransitClient) RotateKey(ctx context.Context, name string) error {
	return t.c.post(ctx, "/v1/transit/keys/"+name+"/rotate", nil, nil)
}

// Encrypt encrypts plaintext with the named key. Returns a ciphertext string.
func (t *TransitClient) Encrypt(ctx context.Context, name string, plaintext []byte) (string, error) {
	var resp struct {
		Ciphertext string `json:"ciphertext"`
	}
	err := t.c.post(ctx, "/v1/transit/encrypt/"+name, map[string]string{
		"plaintext": base64.StdEncoding.EncodeToString(plaintext),
	}, &resp)
	return resp.Ciphertext, err
}

// Decrypt decrypts a ciphertext string with the named key. Returns plaintext bytes.
func (t *TransitClient) Decrypt(ctx context.Context, name, ciphertext string) ([]byte, error) {
	var resp struct {
		Plaintext string `json:"plaintext"`
	}
	if err := t.c.post(ctx, "/v1/transit/decrypt/"+name, map[string]string{
		"ciphertext": ciphertext,
	}, &resp); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(resp.Plaintext)
}

// Sign signs data with the named key. algorithm defaults to the key's algorithm when empty.
func (t *TransitClient) Sign(ctx context.Context, name, algorithm string, data []byte) (string, error) {
	body := map[string]string{
		"input": base64.StdEncoding.EncodeToString(data),
	}
	if algorithm != "" {
		body["signature_algorithm"] = algorithm
	}
	var resp struct {
		Signature string `json:"signature"`
	}
	err := t.c.post(ctx, "/v1/transit/sign/"+name, body, &resp)
	return resp.Signature, err
}

// Verify verifies a signature over data with the named key.
func (t *TransitClient) Verify(ctx context.Context, name, algorithm, signature string, data []byte) (bool, error) {
	body := map[string]string{
		"input":     base64.StdEncoding.EncodeToString(data),
		"signature": signature,
	}
	if algorithm != "" {
		body["signature_algorithm"] = algorithm
	}
	var resp struct {
		Valid bool `json:"valid"`
	}
	err := t.c.post(ctx, "/v1/transit/verify/"+name, body, &resp)
	return resp.Valid, err
}

// HMAC computes an HMAC over data using the named key.
func (t *TransitClient) HMAC(ctx context.Context, name, algorithm string, data []byte) (string, error) {
	body := map[string]string{
		"input": base64.StdEncoding.EncodeToString(data),
	}
	if algorithm != "" {
		body["algorithm"] = algorithm
	}
	var resp struct {
		HMAC string `json:"hmac"`
	}
	err := t.c.post(ctx, "/v1/transit/hmac/"+name, body, &resp)
	return resp.HMAC, err
}
