package token

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/physical"
)

// ErrNotFound is returned when a token does not exist in the store.
var ErrNotFound = errors.New("token not found")

const (
	tokenPrefix    = "auth/token/"
	accessorPrefix = "auth/accessor/"
)

// accessorRecord is stored at auth/accessor/<accessor>.
type accessorRecord struct {
	TokenID string `json:"token_id"`
}

// Store is a thin CRUD wrapper over a barrier for token persistence.
type Store struct {
	barrier *barrier.Barrier
}

func NewStore(b *barrier.Barrier) *Store { return &Store{barrier: b} }

func (s *Store) Put(ctx context.Context, t *Token) error {
	data, err := t.marshal()
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}
	if err := s.barrier.Put(ctx, &physical.Entry{Key: tokenKey(t.ID), Value: data}); err != nil {
		return err
	}
	// Maintain accessor index when present.
	if t.Accessor != "" {
		rec, _ := json.Marshal(accessorRecord{TokenID: t.ID})
		_ = s.barrier.Put(ctx, &physical.Entry{Key: accessorKey(t.Accessor), Value: rec})
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id string) (*Token, error) {
	e, err := s.barrier.Get(ctx, tokenKey(id))
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNotFound
	}
	return unmarshal(e.Value)
}

// GetByAccessor looks up a token by its accessor string.
func (s *Store) GetByAccessor(ctx context.Context, accessor string) (*Token, error) {
	e, err := s.barrier.Get(ctx, accessorKey(accessor))
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNotFound
	}
	var rec accessorRecord
	if err := json.Unmarshal(e.Value, &rec); err != nil {
		return nil, ErrNotFound
	}
	return s.Get(ctx, rec.TokenID)
}

func (s *Store) Delete(ctx context.Context, id string) error {
	// Remove accessor index first (best-effort).
	tok, err := s.Get(ctx, id)
	if err == nil && tok.Accessor != "" {
		_ = s.barrier.Delete(ctx, accessorKey(tok.Accessor))
	}
	return s.barrier.Delete(ctx, tokenKey(id))
}

// DeleteByAccessor revokes the token identified by accessor.
func (s *Store) DeleteByAccessor(ctx context.Context, accessor string) error {
	e, err := s.barrier.Get(ctx, accessorKey(accessor))
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotFound
	}
	var rec accessorRecord
	if err := json.Unmarshal(e.Value, &rec); err != nil {
		return ErrNotFound
	}
	_ = s.barrier.Delete(ctx, accessorKey(accessor))
	return s.barrier.Delete(ctx, tokenKey(rec.TokenID))
}

// List returns all token IDs currently persisted in the store.
// Because token IDs are stored under their SHA-256 hash, we read each entry
// to recover the original ID from the token value.
func (s *Store) List(ctx context.Context) ([]string, error) {
	keys, err := s.barrier.List(ctx, tokenPrefix)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(keys))
	for _, k := range keys {
		tok, err := s.getByStorageKey(ctx, k)
		if err != nil {
			continue
		}
		ids = append(ids, tok.ID)
	}
	return ids, nil
}

// ListExpired returns the IDs of all tokens whose TTL has elapsed.
func (s *Store) ListExpired(ctx context.Context) ([]string, error) {
	keys, err := s.barrier.List(ctx, tokenPrefix)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var expired []string
	for _, key := range keys {
		tok, err := s.getByStorageKey(ctx, key)
		if err != nil {
			continue
		}
		if !tok.ExpiresAt.IsZero() && tok.ExpiresAt.Before(now) {
			expired = append(expired, tok.ID)
		}
	}
	return expired, nil
}

// getByStorageKey reads a token directly by its full barrier storage key
// (i.e. the hashed path), without requiring the original token ID.
func (s *Store) getByStorageKey(ctx context.Context, storageKey string) (*Token, error) {
	e, err := s.barrier.Get(ctx, storageKey)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNotFound
	}
	return unmarshal(e.Value)
}

// tokenKey returns the barrier storage key for a given token ID.
// The ID is hashed with SHA-256 so that raw bearer credentials are never
// visible as bbolt keys in a database dump.
func tokenKey(id string) string {
	h := sha256.Sum256([]byte(id))
	return tokenPrefix + hex.EncodeToString(h[:])
}

func accessorKey(acc string) string { return accessorPrefix + acc }
