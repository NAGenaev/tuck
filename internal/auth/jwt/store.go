package jwt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/NAGenaev/tuck/internal/physical"
)

const (
	configKey = "auth/jwt/config"
	rolesKey  = "auth/jwt/roles/"
)

// ErrConfigNotFound is returned when JWT auth has not been configured yet.
var ErrConfigNotFound = errors.New("jwt: auth not configured — PUT /v1/auth/jwt/config first")

// barrier is the minimal interface the store needs from the barrier layer.
type barrier interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Store persists the JWT auth Config and Roles inside the encrypted barrier.
type Store struct {
	b barrier
}

// NewStore creates a Store backed by b.
func NewStore(b barrier) *Store { return &Store{b: b} }

// GetConfig reads the provider config from the barrier.
func (s *Store) GetConfig(ctx context.Context) (*Config, error) {
	e, err := s.b.Get(ctx, configKey)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrConfigNotFound
	}
	var cfg Config
	if err := json.Unmarshal(e.Value, &cfg); err != nil {
		return nil, fmt.Errorf("jwt: decode config: %w", err)
	}
	return &cfg, nil
}

// PutConfig writes the provider config into the barrier.
func (s *Store) PutConfig(ctx context.Context, cfg *Config) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("jwt: encode config: %w", err)
	}
	return s.b.Put(ctx, &physical.Entry{Key: configKey, Value: data})
}

// GetRole reads a role by name.
func (s *Store) GetRole(ctx context.Context, name string) (*Role, error) {
	e, err := s.b.Get(ctx, rolesKey+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("jwt: role %q not found", name)
	}
	var r Role
	if err := json.Unmarshal(e.Value, &r); err != nil {
		return nil, fmt.Errorf("jwt: decode role: %w", err)
	}
	return &r, nil
}

// PutRole writes a role.
func (s *Store) PutRole(ctx context.Context, r *Role) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("jwt: encode role: %w", err)
	}
	return s.b.Put(ctx, &physical.Entry{Key: rolesKey + r.Name, Value: data})
}

// DeleteRole removes a role.
func (s *Store) DeleteRole(ctx context.Context, name string) error {
	return s.b.Delete(ctx, rolesKey+name)
}

// ListRoles returns all role names.
func (s *Store) ListRoles(ctx context.Context) ([]string, error) {
	return s.b.List(ctx, rolesKey)
}

// AllRoles loads and returns all roles.
func (s *Store) AllRoles(ctx context.Context) ([]*Role, error) {
	keys, err := s.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	roles := make([]*Role, 0, len(keys))
	for _, k := range keys {
		name := k[len(rolesKey):]
		r, err := s.GetRole(ctx, name)
		if err != nil {
			continue
		}
		roles = append(roles, r)
	}
	return roles, nil
}
