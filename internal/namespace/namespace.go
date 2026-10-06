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
	// Descriptive metadata. Optional and omitted when empty, so namespaces
	// stored before these fields existed load unchanged.
	Description string            `json:"description,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitzero"`
}

// Metadata limits.
const (
	MaxDescription = 512
	MaxOwner       = 128
	MaxLabels      = 32
	MaxLabelKey    = 64
	MaxLabelValue  = 256
)

// ErrInvalidMetadata is returned when description, owner or labels are invalid.
var ErrInvalidMetadata = errors.New("invalid namespace metadata")

// Metadata is the editable, non-secret part of a namespace.
type Metadata struct {
	Description string            `json:"description"`
	Owner       string            `json:"owner"`
	Labels      map[string]string `json:"labels"`
}

// Validate checks the size limits and that the text has no control characters.
func (m Metadata) Validate() error {
	bad := func(msg string) error { return fmt.Errorf("%w: %s", ErrInvalidMetadata, msg) }
	if len(m.Description) > MaxDescription {
		return bad(fmt.Sprintf("description exceeds %d bytes", MaxDescription))
	}
	if len(m.Owner) > MaxOwner {
		return bad(fmt.Sprintf("owner exceeds %d bytes", MaxOwner))
	}
	if len(m.Labels) > MaxLabels {
		return bad(fmt.Sprintf("at most %d labels", MaxLabels))
	}
	if !printable(m.Description, true) || !printable(m.Owner, false) {
		return bad("control characters are not allowed")
	}
	for k, v := range m.Labels {
		if k == "" || len(k) > MaxLabelKey || len(v) > MaxLabelValue {
			return bad("label key must be 1-" + fmt.Sprint(MaxLabelKey) + " bytes and value up to " + fmt.Sprint(MaxLabelValue))
		}
		if !printable(k, false) || !printable(v, false) {
			return bad("control characters are not allowed")
		}
	}
	return nil
}

// printable reports whether s has no control characters (newlines and tabs
// are accepted when multiline is true).
func printable(s string, multiline bool) bool {
	for _, r := range s {
		if r == '\n' || r == '\t' {
			if multiline {
				continue
			}
			return false
		}
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Apply copies m onto ns (a nil or empty Labels map clears the labels).
func (ns *Namespace) Apply(m Metadata) {
	ns.Description = m.Description
	ns.Owner = m.Owner
	if len(m.Labels) == 0 {
		ns.Labels = nil
	} else {
		ns.Labels = make(map[string]string, len(m.Labels))
		for k, v := range m.Labels {
			ns.Labels[k] = v
		}
	}
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
