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

// ErrAlreadyExists is returned when creating a namespace whose name is taken.
var ErrAlreadyExists = errors.New("namespace already exists")

// ErrInvalidName is returned when a namespace name contains illegal characters.
//
// A name is either a single segment or several segments joined by a single
// "/" (e.g. "team-a/project-x"), each matching [a-z0-9][a-z0-9-]*. The "/" is
// purely an organizational naming convention for grouping in UIs and tooling:
// the full string (segments and slashes together) is always one atomic,
// fully-isolated namespace with its own independent secrets, policies and
// tokens. There is no parent/child relationship, inheritance or cascading
// delete between namespaces that happen to share a prefix segment.
var ErrInvalidName = errors.New("namespace name must be [a-z0-9][a-z0-9-]* segments joined by single '/', no segment 'root', at most " + fmt.Sprint(MaxNameSegments) + " segments and " + fmt.Sprint(MaxNameLength) + " bytes total")

var segmentRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const (
	storagePrefix = "sys/namespaces/"
	RootNamespace = "root"

	// MaxNameSegments bounds the number of "/"-separated segments in a
	// namespace name (organizational nesting depth, not storage nesting).
	MaxNameSegments = 4
	// MaxNameLength bounds the total byte length of a namespace name.
	MaxNameLength = 128
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

// ValidateName returns ErrInvalidName if name is not a valid namespace
// identifier. "/" may be used to group names by organizational convention
// (e.g. "team-a/project-x"); the full name including any slashes remains a
// single, flat, fully-isolated namespace — see ErrInvalidName for details.
func ValidateName(name string) error {
	if len(name) > MaxNameLength {
		return ErrInvalidName
	}
	segments := strings.Split(name, "/")
	if len(segments) > MaxNameSegments {
		return ErrInvalidName
	}
	for _, seg := range segments {
		if seg == RootNamespace || !segmentRe.MatchString(seg) {
			return ErrInvalidName
		}
	}
	return nil
}

// StoragePrefix returns the barrier key prefix for the given namespace.
// Root namespace has no prefix (empty string).
//
// A namespace name may itself contain "/" (organizational grouping, see
// ValidateName); naively concatenating it as "ns/"+ns+"/" would make the
// boundary between the namespace segment and the following engine-key
// segment ambiguous — e.g. namespace "team-a" storing a KV v2 secret at
// path "secret/meta/x" and namespace "team-a/secret" storing its own
// metadata for secret "x" would both resolve to the barrier key
// "ns/team-a/secret/meta/x". Since a valid namespace segment can only
// contain "[a-z0-9-]" (never "~"), replacing "/" with "~" here is
// injective and keeps the encoded name free of "/": the first "/" after
// the "ns/" literal is then always the unambiguous end of the namespace
// segment, no matter what follows. Existing slash-free namespace names
// encode to themselves, so this is backward compatible with data already
// on disk.
func StoragePrefix(ns string) string {
	if ns == "" || ns == RootNamespace {
		return ""
	}
	return "ns/" + strings.ReplaceAll(ns, "/", "~") + "/"
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
