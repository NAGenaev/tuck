package token

import (
	"context"
	"errors"
	"fmt"

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

func tokenKey(id string) string { return "auth/token/" + id }
