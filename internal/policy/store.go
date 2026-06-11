package policy

import (
	"context"
	"errors"
	"fmt"

	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/physical"
)

// ErrNotFound is returned when a policy does not exist in the store.
var ErrNotFound = errors.New("policy not found")

// Store is a thin CRUD wrapper over a barrier for policy persistence.
type Store struct {
	barrier *barrier.Barrier
}

func NewStore(b *barrier.Barrier) *Store { return &Store{barrier: b} }

func (s *Store) Put(ctx context.Context, p *Policy) error {
	data, err := p.marshal()
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}
	return s.barrier.Put(ctx, &physical.Entry{Key: policyKey(p.Name), Value: data})
}

func (s *Store) Get(ctx context.Context, name string) (*Policy, error) {
	e, err := s.barrier.Get(ctx, policyKey(name))
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNotFound
	}
	return unmarshal(e.Value)
}

func (s *Store) Delete(ctx context.Context, name string) error {
	return s.barrier.Delete(ctx, policyKey(name))
}

func policyKey(name string) string { return "auth/policy/" + name }
