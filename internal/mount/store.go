// Package mount implements the secret engine mount table. Each entry
// associates a logical path prefix with a named engine type and a
// stable accessor UUID. The builtin engines (kv, kv-v2, pki, database,
// aws, gcp, azure, transit, ssh, totp, cubbyhole) are registered on
// first startup; operators can add additional mounts of supported types.
package mount

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

const (
	storePrefix = "sys/mounts/"
	// ErrAlreadyExists is returned when a mount path is already registered.
	ErrAlreadyExists = mountError("mount already exists at this path")
	// ErrNotFound is returned when no mount is registered at the path.
	ErrNotFound = mountError("mount not found")
	// ErrBuiltin is returned when an attempt is made to delete a builtin mount.
	ErrBuiltin = mountError("cannot unmount a builtin engine")
)

type mountError string

func (e mountError) Error() string { return string(e) }

// Is supports errors.Is comparisons.
func (e mountError) Is(target error) bool {
	t, ok := target.(mountError)
	return ok && t == e
}

// Entry describes a single mount-table entry.
type Entry struct {
	Path        string    `json:"path"`         // logical path prefix, e.g. "secret/"
	Type        string    `json:"type"`         // engine type, e.g. "kv", "pki"
	Accessor    string    `json:"accessor"`     // stable UUID for audit / identity correlation
	Description string    `json:"description"`  // human-readable label
	Builtin     bool      `json:"builtin"`      // true for default engines
	CreatedAt   time.Time `json:"created_at"`
}

// barrierer is the subset of barrier.Barrier used by the store.
type barrierer interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Store manages the persisted mount table.
type Store struct{ b barrierer }

// New returns a Store backed by the given barrier.
func New(b barrierer) *Store { return &Store{b: b} }

// Register adds a new (non-builtin) mount. Returns ErrAlreadyExists if path taken.
func (s *Store) Register(ctx context.Context, path, engineType, description string) (*Entry, error) {
	path = normalizePath(path)
	if _, err := s.Get(ctx, path); err == nil {
		return nil, ErrAlreadyExists
	}
	e := &Entry{
		Path:        path,
		Type:        engineType,
		Accessor:    newAccessor(),
		Description: description,
		Builtin:     false,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.put(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// RegisterBuiltin adds a builtin mount (idempotent — silently skips if already present).
func (s *Store) RegisterBuiltin(ctx context.Context, path, engineType, description string) error {
	path = normalizePath(path)
	if _, err := s.Get(ctx, path); err == nil {
		return nil // already present
	}
	return s.put(ctx, &Entry{
		Path:        path,
		Type:        engineType,
		Accessor:    newAccessor(),
		Description: description,
		Builtin:     true,
		CreatedAt:   time.Now().UTC(),
	})
}

// Get returns the entry at the given path.
func (s *Store) Get(ctx context.Context, path string) (*Entry, error) {
	path = normalizePath(path)
	e, err := s.b.Get(ctx, storePrefix+pathKey(path))
	if err != nil || e == nil {
		return nil, ErrNotFound
	}
	var entry Entry
	if err := json.Unmarshal(e.Value, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// Delete removes a non-builtin mount. Returns ErrBuiltin for builtin engines.
func (s *Store) Delete(ctx context.Context, path string) error {
	path = normalizePath(path)
	entry, err := s.Get(ctx, path)
	if err != nil {
		return ErrNotFound
	}
	if entry.Builtin {
		return ErrBuiltin
	}
	return s.b.Delete(ctx, storePrefix+pathKey(path))
}

// List returns all mount entries.
func (s *Store) List(ctx context.Context) ([]*Entry, error) {
	keys, err := s.b.List(ctx, storePrefix)
	if err != nil {
		return nil, err
	}
	out := make([]*Entry, 0, len(keys))
	for _, k := range keys {
		e, err := s.b.Get(ctx, k)
		if err != nil || e == nil {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(e.Value, &entry); err != nil {
			continue
		}
		out = append(out, &entry)
	}
	return out, nil
}

func (s *Store) put(ctx context.Context, e *Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return s.b.Put(ctx, &physical.Entry{Key: storePrefix + pathKey(e.Path), Value: b})
}

// normalizePath trims leading/trailing slashes and appends one trailing slash
// so that "secret", "secret/", and "/secret/" all map to the same key.
func normalizePath(p string) string {
	p = strings.Trim(p, "/")
	if p == "" {
		return "/"
	}
	return p + "/"
}

// pathKey converts a normalized path to a storage-safe key (replaces / with _).
func pathKey(p string) string {
	return strings.ReplaceAll(strings.Trim(p, "/"), "/", "_")
}

func newAccessor() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Builtins returns the default engine mounts that every Tuck server registers.
func Builtins() []struct{ Path, Type, Description string } {
	return []struct{ Path, Type, Description string }{
		{"secret/", "kv", "Default KV v1 secrets engine"},
		{"cubbyhole/", "cubbyhole", "Per-token private storage"},
		{"sys/", "system", "System configuration and management"},
		{"auth/", "auth", "Authentication methods"},
		{"database/", "database", "Database dynamic credentials"},
		{"aws/", "aws", "AWS dynamic credentials"},
		{"gcp/", "gcp", "GCP dynamic credentials"},
		{"azure/", "azure", "Azure dynamic credentials"},
		{"pki/", "pki", "PKI / certificate management"},
		{"transit/", "transit", "Encryption-as-a-service"},
		{"ssh/", "ssh", "SSH certificate signing"},
		{"totp/", "totp", "TOTP two-factor management"},
		{"identity/", "identity", "Entity and alias management"},
	}
}

// Sentinel for errors.Is — expose package-level sentinels.
var (
	_ = errors.Is(ErrAlreadyExists, ErrAlreadyExists) // compile-time check
)
