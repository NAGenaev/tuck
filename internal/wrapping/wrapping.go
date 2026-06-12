// Package wrapping implements Tuck's response-wrapping subsystem.
//
// Any JSON payload can be sealed inside a single-use wrapping token with a
// short TTL. The caller passes the token to a consumer; the consumer unwraps it
// to retrieve the payload. Because the token is deleted on first use, a failed
// unwrap proves that someone else has already read the secret.
//
// Storage layout:
//
//	sys/wrapping/<id>  →  JSON record {payload, created_at, expires_at}
package wrapping

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	ErrNotFound = errors.New("wrapping: token not found or already used")
	ErrExpired  = errors.New("wrapping: token has expired")
)

const (
	storagePrefix = "sys/wrapping/"
	TokenPrefix   = "tuck_wrap_"

	DefaultTTL = 5 * time.Minute
	MaxTTL     = 24 * time.Hour
)

type barrierIface interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// record is what we persist in the barrier for each wrapping token.
type record struct {
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
	ExpiresAt time.Time       `json:"expires_at"`
}

// TokenInfo is returned by Lookup.
type TokenInfo struct {
	CreatedAt   time.Time `json:"creation_time"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreationTTL int       `json:"creation_ttl"` // seconds
}

// Store manages wrapping tokens in the barrier.
type Store struct {
	b  barrierIface
	mu sync.Mutex // serialises Unwrap to prevent TOCTOU on single-use tokens
}

// NewStore returns a Store backed by b.
func NewStore(b barrierIface) *Store {
	return &Store{b: b}
}

// Wrap seals payload under a new single-use token valid for ttl.
// Returns the wrapping token string (prefix tuck_wrap_…).
func (s *Store) Wrap(ctx context.Context, payload json.RawMessage, ttl time.Duration) (string, time.Time, error) {
	if ttl == 0 {
		ttl = DefaultTTL
	}
	if ttl > MaxTTL {
		ttl = MaxTTL
	}

	id, err := randomID()
	if err != nil {
		return "", time.Time{}, err
	}
	token := TokenPrefix + id

	now := time.Now()
	expiresAt := now.Add(ttl)
	rec := record{
		Payload:   payload,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return "", time.Time{}, err
	}
	if err := s.b.Put(ctx, &physical.Entry{Key: storagePrefix + id, Value: data}); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

// Unwrap consumes the wrapping token and returns its payload.
// The token is deleted atomically — a second call always returns ErrNotFound.
func (s *Store) Unwrap(ctx context.Context, token string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := tokenID(token)
	if id == "" {
		return nil, ErrNotFound
	}
	key := storagePrefix + id

	entry, err := s.b.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, ErrNotFound
	}

	var rec record
	if err := json.Unmarshal(entry.Value, &rec); err != nil {
		return nil, err
	}

	// Delete before returning — single use.
	if err := s.b.Delete(ctx, key); err != nil {
		return nil, err
	}

	if time.Now().After(rec.ExpiresAt) {
		return nil, ErrExpired
	}
	return rec.Payload, nil
}

// Lookup returns metadata about a wrapping token without consuming it.
func (s *Store) Lookup(ctx context.Context, token string) (*TokenInfo, error) {
	id := tokenID(token)
	if id == "" {
		return nil, ErrNotFound
	}
	entry, err := s.b.Get(ctx, storagePrefix+id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, ErrNotFound
	}
	var rec record
	if err := json.Unmarshal(entry.Value, &rec); err != nil {
		return nil, err
	}
	if time.Now().After(rec.ExpiresAt) {
		_ = s.b.Delete(ctx, storagePrefix+id)
		return nil, ErrExpired
	}
	return &TokenInfo{
		CreatedAt:   rec.CreatedAt,
		ExpiresAt:   rec.ExpiresAt,
		CreationTTL: int(rec.ExpiresAt.Sub(rec.CreatedAt).Seconds()),
	}, nil
}

// Revoke explicitly deletes a wrapping token before it expires.
func (s *Store) Revoke(ctx context.Context, token string) error {
	id := tokenID(token)
	if id == "" {
		return ErrNotFound
	}
	entry, err := s.b.Get(ctx, storagePrefix+id)
	if err != nil {
		return err
	}
	if entry == nil {
		return ErrNotFound
	}
	return s.b.Delete(ctx, storagePrefix+id)
}

// RevokeExpired deletes all expired wrapping tokens. Called by background GC.
func (s *Store) RevokeExpired(ctx context.Context) error {
	keys, err := s.b.List(ctx, storagePrefix)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, fullKey := range keys {
		entry, err := s.b.Get(ctx, fullKey)
		if err != nil || entry == nil {
			continue
		}
		var rec record
		if err := json.Unmarshal(entry.Value, &rec); err != nil {
			continue
		}
		if rec.ExpiresAt.Before(now) {
			_ = s.b.Delete(ctx, fullKey)
		}
	}
	return nil
}

// tokenID extracts the bare random ID from a wrapping token string.
// Returns "" if the token does not have the expected prefix.
func tokenID(token string) string {
	if !strings.HasPrefix(token, TokenPrefix) {
		return ""
	}
	return strings.TrimPrefix(token, TokenPrefix)
}

func randomID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
