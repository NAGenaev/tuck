package seal

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// TransitSeal implements auto-unseal via a Vault-compatible Transit API.
//
// On Init it generates a root key, encrypts (wraps) it by POST-ing to
// <addr>/v1/transit/encrypt/<keyName>, and stores the ciphertext locally.
// On Unseal it reads the ciphertext and decrypts (unwraps) it via
// <addr>/v1/transit/decrypt/<keyName>. The stored file is safe to expose
// because it is meaningless without the Transit service.
//
// Compatible with:
//   - HashiCorp Vault Transit secrets engine
//   - Any HTTP service implementing the same encrypt/decrypt JSON API
type TransitSeal struct {
	// addr is the base URL, e.g. "http://vault:8200"
	addr string
	// keyName is the Transit key, e.g. "tuck-seal"
	keyName string
	// token is the Vault/Transit token with encrypt+decrypt capability
	token string
	// wrappedKeyPath is the local file where the ciphertext is stored
	wrappedKeyPath string

	client *http.Client
}

// NewTransit creates a TransitSeal.
//
//   - addr: Transit server base URL (no trailing slash)
//   - keyName: name of the Transit encryption key
//   - token: bearer token for the Transit API (stored in memory only)
//   - wrappedKeyPath: path where the encrypted root key ciphertext is stored
func NewTransit(addr, keyName, token, wrappedKeyPath string) *TransitSeal {
	return &TransitSeal{
		addr:           addr,
		keyName:        keyName,
		token:          token,
		wrappedKeyPath: wrappedKeyPath,
		client:         &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *TransitSeal) Type() string { return "transit" }

// Init generates a fresh root key, wraps it via the Transit API, stores the
// ciphertext locally, and returns the root key in InitResult. Shares is nil
// (Transit is auto-unseal).
func (t *TransitSeal) Init() (*InitResult, error) {
	key := make([]byte, rootKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("transit seal: generate root key: %w", err)
	}

	ciphertext, err := t.encrypt(key)
	if err != nil {
		return nil, fmt.Errorf("transit seal: encrypt root key: %w", err)
	}

	if err := os.WriteFile(t.wrappedKeyPath, []byte(ciphertext), 0600); err != nil {
		return nil, fmt.Errorf("transit seal: write wrapped key: %w", err)
	}

	return &InitResult{RootKey: key}, nil
}

// Unseal reads the stored ciphertext and decrypts it via the Transit API,
// returning the root key.
func (t *TransitSeal) Unseal() ([]byte, error) {
	ciphertextBytes, err := os.ReadFile(t.wrappedKeyPath)
	if err != nil {
		return nil, fmt.Errorf("transit seal: read wrapped key: %w", err)
	}

	key, err := t.decrypt(string(ciphertextBytes))
	if err != nil {
		return nil, fmt.Errorf("transit seal: decrypt root key: %w", err)
	}
	return key, nil
}

// --- Vault Transit API helpers ---

// transitEncryptRequest mirrors the Vault Transit encrypt request body.
type transitEncryptRequest struct {
	Plaintext string `json:"plaintext"` // base64-encoded plaintext
}

// transitEncryptResponse mirrors the relevant fields of the Vault Transit
// encrypt response.
type transitEncryptResponse struct {
	Data struct {
		Ciphertext string `json:"ciphertext"`
	} `json:"data"`
	Errors []string `json:"errors"`
}

// transitDecryptRequest mirrors the Vault Transit decrypt request body.
type transitDecryptRequest struct {
	Ciphertext string `json:"ciphertext"`
}

// transitDecryptResponse mirrors the relevant fields of the Vault Transit
// decrypt response.
type transitDecryptResponse struct {
	Data struct {
		Plaintext string `json:"plaintext"` // base64-encoded plaintext
	} `json:"data"`
	Errors []string `json:"errors"`
}

// encrypt calls POST <addr>/v1/transit/encrypt/<keyName> and returns the
// vault:v1:... ciphertext string.
func (t *TransitSeal) encrypt(plaintext []byte) (string, error) {
	reqBody := transitEncryptRequest{
		Plaintext: base64.StdEncoding.EncodeToString(plaintext),
	}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/v1/transit/encrypt/%s", t.addr, t.keyName)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vault-Token", t.token)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP POST encrypt: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read encrypt response: %w", err)
	}

	var result transitEncryptResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse encrypt response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("transit encrypt: status %d: %v", resp.StatusCode, result.Errors)
	}
	if result.Data.Ciphertext == "" {
		return "", fmt.Errorf("transit encrypt: empty ciphertext in response")
	}
	return result.Data.Ciphertext, nil
}

// decrypt calls POST <addr>/v1/transit/decrypt/<keyName> and returns the
// plaintext bytes.
func (t *TransitSeal) decrypt(ciphertext string) ([]byte, error) {
	reqBody := transitDecryptRequest{Ciphertext: ciphertext}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1/transit/decrypt/%s", t.addr, t.keyName)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vault-Token", t.token)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP POST decrypt: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read decrypt response: %w", err)
	}

	var result transitDecryptResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse decrypt response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transit decrypt: status %d: %v", resp.StatusCode, result.Errors)
	}
	if result.Data.Plaintext == "" {
		return nil, fmt.Errorf("transit decrypt: empty plaintext in response")
	}

	plaintext, err := base64.StdEncoding.DecodeString(result.Data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("transit decrypt: base64 decode plaintext: %w", err)
	}
	return plaintext, nil
}
