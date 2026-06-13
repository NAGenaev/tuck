// Package kvsecret provides the on-disk envelope for KV v1 secrets.
// Values written before this package existed are plain bytes; new writes
// use the JSON envelope so that TTL and metadata can be carried alongside
// the payload without a separate storage key.
package kvsecret

import (
	"encoding/json"
	"time"
)

// Entry is the envelope stored at each KV v1 path.
type Entry struct {
	Value     []byte            `json:"value"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt time.Time         `json:"expires_at,omitempty"` // zero = never
}

// IsExpired reports whether the entry has passed its TTL.
func (e *Entry) IsExpired() bool {
	return !e.ExpiresAt.IsZero() && time.Now().After(e.ExpiresAt)
}

// Marshal encodes an Entry for barrier storage.
func (e *Entry) Marshal() ([]byte, error) { return json.Marshal(e) }

// Unmarshal decodes barrier bytes into an Entry.
// If the bytes are not a valid JSON Entry (legacy raw-bytes secrets), the raw
// bytes are returned as Entry.Value so callers remain backward-compatible.
func Unmarshal(raw []byte) *Entry {
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil || e.CreatedAt.IsZero() {
		// Legacy: treat the whole blob as the value.
		return &Entry{Value: raw, CreatedAt: time.Time{}}
	}
	return &e
}

// New wraps value and optional metadata into an Entry with CreatedAt set to now.
// ttl=0 means no expiry.
func New(value []byte, metadata map[string]string, ttl time.Duration) *Entry {
	e := &Entry{
		Value:     value,
		Metadata:  metadata,
		CreatedAt: time.Now().UTC(),
	}
	if ttl > 0 {
		e.ExpiresAt = e.CreatedAt.Add(ttl)
	}
	return e
}
