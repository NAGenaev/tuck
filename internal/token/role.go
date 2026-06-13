package token

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

// ErrRoleNotFound is returned when a token role does not exist.
var ErrRoleNotFound = errors.New("token role not found")

const rolePrefix = "auth/token-role/" // #nosec G101 — storage path prefix

// Role is a named token template. Tokens created from a role inherit its settings.
type Role struct {
	Name      string        `json:"name"`
	Policies  []string      `json:"policies"`
	TTL       time.Duration `json:"ttl"`        // default token lifetime; 0 = never expires
	MaxTTL    time.Duration `json:"max_ttl"`    // caps renewals; 0 = no cap
	MaxUses   int           `json:"max_uses"`   // 0 = unlimited
	Renewable bool          `json:"renewable"`
	Period    time.Duration `json:"period"`     // if > 0, renewable tokens are extended to Period on each renewal
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// RoleStore persists token roles in the barrier.
type RoleStore struct {
	barrier barrierer
}

// barrierer is the barrier subset used by RoleStore and Store.
type barrierer interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// NewRoleStore creates a RoleStore backed by b.
func NewRoleStore(b barrierer) *RoleStore { return &RoleStore{barrier: b} }

func (s *RoleStore) Put(ctx context.Context, r *Role) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal role: %w", err)
	}
	return s.barrier.Put(ctx, &physical.Entry{Key: rolePrefix + r.Name, Value: data})
}

func (s *RoleStore) Get(ctx context.Context, name string) (*Role, error) {
	e, err := s.barrier.Get(ctx, rolePrefix+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrRoleNotFound
	}
	var r Role
	if err := json.Unmarshal(e.Value, &r); err != nil {
		return nil, fmt.Errorf("unmarshal role: %w", err)
	}
	return &r, nil
}

func (s *RoleStore) Delete(ctx context.Context, name string) error {
	return s.barrier.Delete(ctx, rolePrefix+name)
}

func (s *RoleStore) List(ctx context.Context) ([]string, error) {
	keys, err := s.barrier.List(ctx, rolePrefix)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = strings.TrimPrefix(k, rolePrefix)
	}
	return names, nil
}
