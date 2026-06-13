package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/NAGenaev/tuck/internal/physical"
)

const rolesPrefix = "auth/github/roles/"

type barrierer interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Store persists GitHub auth roles inside the encrypted barrier.
type Store struct {
	b barrierer
}

// NewStore creates a Store backed by b.
func NewStore(b barrierer) *Store { return &Store{b: b} }

func (s *Store) PutRole(ctx context.Context, r *Role) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("github store: marshal role: %w", err)
	}
	return s.b.Put(ctx, &physical.Entry{Key: rolesPrefix + r.Name, Value: data})
}

func (s *Store) GetRole(ctx context.Context, name string) (*Role, error) {
	e, err := s.b.Get(ctx, rolesPrefix+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrRoleNotFound
	}
	var r Role
	if err := json.Unmarshal(e.Value, &r); err != nil {
		return nil, fmt.Errorf("github store: unmarshal role: %w", err)
	}
	return &r, nil
}

func (s *Store) DeleteRole(ctx context.Context, name string) error {
	return s.b.Delete(ctx, rolesPrefix+name)
}

func (s *Store) ListRoles(ctx context.Context) ([]string, error) {
	keys, err := s.b.List(ctx, rolesPrefix)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = strings.TrimPrefix(k, rolesPrefix)
	}
	return names, nil
}
