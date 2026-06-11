// Package jwt implements JWT/OIDC authentication for Tuck.
// Any OIDC-compatible identity provider (Keycloak, Auth0, GitHub Actions,
// Google, Dex, …) can exchange a signed JWT for a short-lived Tuck token.
package jwt

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// JWKS is a cached JSON Web Key Set fetched from a remote JWKS URI.
// Keys are refreshed automatically after the TTL expires.
type JWKS struct {
	uri     string
	ttl     time.Duration
	mu      sync.RWMutex
	keys    map[string]interface{} // kid → crypto.PublicKey
	fetchAt time.Time
	client  *http.Client
}

// NewJWKS creates a JWKS that caches keys from uri for the given TTL.
// A zero TTL defaults to 10 minutes.
func NewJWKS(uri string, ttl time.Duration, client *http.Client) *JWKS {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &JWKS{uri: uri, ttl: ttl, client: client, keys: make(map[string]interface{})}
}

// GetKey returns the public key for the given kid. Refreshes the JWKS if
// the cache is stale or the kid is unknown.
func (j *JWKS) GetKey(ctx context.Context, kid string) (interface{}, error) {
	j.mu.RLock()
	k, ok := j.keys[kid]
	stale := time.Since(j.fetchAt) > j.ttl
	j.mu.RUnlock()

	if ok && !stale {
		return k, nil
	}

	if err := j.refresh(ctx); err != nil {
		return nil, err
	}

	j.mu.RLock()
	k, ok = j.keys[kid]
	j.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("jwt: unknown key id %q", kid)
	}
	return k, nil
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"` // RSA modulus (base64url)
	E   string `json:"e"` // RSA exponent (base64url)
}

func (j *JWKS) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.uri, nil)
	if err != nil {
		return fmt.Errorf("jwt: build JWKS request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("jwt: fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	var raw jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return fmt.Errorf("jwt: decode JWKS: %w", err)
	}

	keys := make(map[string]interface{}, len(raw.Keys))
	for _, k := range raw.Keys {
		pub, err := parseJWK(k)
		if err != nil {
			continue // skip unsupported keys
		}
		keys[k.Kid] = pub
	}

	j.mu.Lock()
	j.keys = keys
	j.fetchAt = time.Now()
	j.mu.Unlock()
	return nil
}

func parseJWK(k jwk) (interface{}, error) {
	switch k.Kty {
	case "RSA":
		return parseRSAKey(k)
	default:
		return nil, fmt.Errorf("unsupported key type %q", k.Kty)
	}
}

func parseRSAKey(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode RSA n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode RSA e: %w", err)
	}
	e := new(big.Int).SetBytes(eBytes)
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(e.Int64()),
	}, nil
}
