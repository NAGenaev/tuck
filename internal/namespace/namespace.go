// Package namespace implements Tuck's multi-tenancy layer.
// Each namespace has isolated secret and policy storage.
// The root namespace is implicit and requires no header.
package namespace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

// ErrNotFound is returned when a namespace does not exist.
var ErrNotFound = errors.New("namespace not found")

// ErrInvalidName is returned when a namespace name contains illegal characters.
var ErrInvalidName = errors.New("namespace name must match [a-z0-9][a-z0-9-]* and not be 'root'")

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const (
	storagePrefix = "sys/namespaces/"
	RootNamespace = "root"
)

// Namespace is a named isolation domain for secrets and policies.
type Namespace struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ValidateName returns ErrInvalidName if name is not a valid namespace identifier.
func ValidateName(name string) error {
	if name == RootNamespace || !nameRe.MatchString(name) {
		return ErrInvalidName
	}
	return nil
}

// StoragePrefix returns the barrier key prefix for the given namespace.
// Root namespace has no prefix (empty string).
func StoragePrefix(ns string) string {
	if ns == "" || ns == RootNamespace {
		return ""
	}
	return "ns/" + ns + "/"
}

// barrierer is the storage interface required by the namespace store.
type barrierer interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Store is a thin CRUD wrapper for namespace metadata.
type Store struct {
	barrier barrierer
}

// NewStore creates a namespace store backed by the given barrier.
func NewStore(b barrierer) *Store { return &Store{barrier: b} }

// Put creates or updates a namespace.
func (s *Store) Put(ctx context.Context, ns *Namespace) error {
	data, err := json.Marshal(ns)
	if err != nil {
		return fmt.Errorf("marshal namespace: %w", err)
	}
	return s.barrier.Put(ctx, &physical.Entry{Key: storagePrefix + ns.Name, Value: data})
}

// Get retrieves a namespace by name.
func (s *Store) Get(ctx context.Context, name string) (*Namespace, error) {
	e, err := s.barrier.Get(ctx, storagePrefix+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNotFound
	}
	var ns Namespace
	if err := json.Unmarshal(e.Value, &ns); err != nil {
		return nil, fmt.Errorf("unmarshal namespace: %w", err)
	}
	return &ns, nil
}

// Delete removes a namespace by name.
func (s *Store) Delete(ctx context.Context, name string) error {
	return s.barrier.Delete(ctx, storagePrefix+name)
}

// List returns all namespace names.
func (s *Store) List(ctx context.Context) ([]string, error) {
	keys, err := s.barrier.List(ctx, storagePrefix)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = strings.TrimPrefix(k, storagePrefix)
	}
	return names, nil
}
