// Package token implements Tuck's bearer-token identity layer.
package token

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"time"
)

const (
	RootPolicyName       = "root"
	RootTokenDisplayName = "Root Token"
)

// Token is a bearer credential that carries a set of policy names.
type Token struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Policies    []string  `json:"policies"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"` // zero means never
}

// Generate creates a new token with a cryptographically random ID.
func Generate(displayName string, policies []string, ttl time.Duration) (*Token, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	t := &Token{
		ID:          "tuck_" + base64.RawURLEncoding.EncodeToString(raw),
		DisplayName: displayName,
		Policies:    policies,
		CreatedAt:   now,
	}
	if ttl > 0 {
		t.ExpiresAt = now.Add(ttl)
	}
	return t, nil
}

// IsExpired reports whether the token has passed its expiry time.
// Tokens with a zero ExpiresAt never expire.
func (t *Token) IsExpired() bool {
	return !t.ExpiresAt.IsZero() && time.Now().After(t.ExpiresAt)
}

func (t *Token) marshal() ([]byte, error)   { return json.Marshal(t) }
func unmarshal(data []byte) (*Token, error) { var t Token; return &t, json.Unmarshal(data, &t) }
