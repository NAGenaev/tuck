package token

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/physical"
)

// ErrNotFound is returned when a token does not exist in the store.
var ErrNotFound = errors.New("token not found")

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
	return s.barrier.Put(ctx, &physical.Entry{Key: tokenKey(t.ID), Value: data})
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

func (s *Store) Delete(ctx context.Context, id string) error {
	return s.barrier.Delete(ctx, tokenKey(id))
}

// List returns all token IDs currently persisted in the store.
func (s *Store) List(ctx context.Context) ([]string, error) {
	keys, err := s.barrier.List(ctx, "auth/token/")
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = strings.TrimPrefix(k, "auth/token/")
	}
	return ids, nil
}

// ListExpired returns the IDs of all tokens whose TTL has elapsed.
// Tokens with no expiry (ExpiresAt.IsZero()) are never returned.
func (s *Store) ListExpired(ctx context.Context) ([]string, error) {
	keys, err := s.barrier.List(ctx, "auth/token/")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var expired []string
	for _, key := range keys {
		tok, err := s.Get(ctx, strings.TrimPrefix(key, "auth/token/"))
		if err != nil {
			continue // skip unreadable tokens
		}
		if !tok.ExpiresAt.IsZero() && tok.ExpiresAt.Before(now) {
			expired = append(expired, tok.ID)
		}
	}
	return expired, nil
}

func tokenKey(id string) string { return "auth/token/" + id }
