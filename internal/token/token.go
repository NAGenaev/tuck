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
	ID          string        `json:"id"`
	Accessor    string        `json:"accessor"`            // HMAC-safe alias; returned on create/lookup
	DisplayName string        `json:"display_name"`
	Policies    []string      `json:"policies"`
	EntityID    string        `json:"entity_id,omitempty"` // set when issued via an auth method login
	CreatedAt   time.Time     `json:"created_at"`
	ExpiresAt   time.Time     `json:"expires_at"` // zero means never
	Renewable   bool          `json:"renewable"`  // false by default
	MaxTTL      time.Duration `json:"max_ttl"`    // zero means no cap
	MaxUses     int           `json:"max_uses"`   // 0 = unlimited; N = revoke after N authenticated API calls
	UseCount    int           `json:"use_count"`  // incremented on each Authenticate call
}

// Generate creates a new token with a cryptographically random ID and accessor.
func Generate(displayName string, policies []string, ttl time.Duration) (*Token, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	acc := make([]byte, 16)
	if _, err := rand.Read(acc); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	t := &Token{
		ID:          "tuck_" + base64.RawURLEncoding.EncodeToString(raw),
		Accessor:    "tuck_acc_" + base64.RawURLEncoding.EncodeToString(acc),
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
