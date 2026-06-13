package policy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/NAGenaev/tuck/internal/physical"
)

// ErrNotFound is returned when a policy does not exist in the store.
var ErrNotFound = errors.New("policy not found")

// barrierer is the storage interface required by the store.
type barrierer interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Store is a thin CRUD wrapper over a barrier for policy persistence.
type Store struct {
	barrier barrierer
}

func NewStore(b barrierer) *Store { return &Store{barrier: b} }

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

// List returns all policy names currently persisted in the store.
func (s *Store) List(ctx context.Context) ([]string, error) {
	keys, err := s.barrier.List(ctx, "auth/policy/")
	if err != nil {
		return nil, err
	}
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = strings.TrimPrefix(k, "auth/policy/")
	}
	return names, nil
}

func policyKey(name string) string { return "auth/policy/" + name }
