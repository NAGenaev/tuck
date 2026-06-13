// Package kvv2 implements versioned key-value storage (KV engine v2) on top of
// the barrier. Each write creates a new immutable version; versions can be
// soft-deleted (data hidden, recoverable) or destroyed (data wiped forever).
package kvv2

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

const (
	metaPrefix = "kv2/meta/"
	dataPrefix = "kv2/data/"
	defaultMax = 10
)

// barrierer is the subset of barrier.Barrier used by the store.
type barrierer interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// KVMeta holds version metadata for a single secret path.
type KVMeta struct {
	CurrentVersion int              `json:"current_version"`
	MaxVersions    int              `json:"max_versions"`
	Versions       map[int]*VerMeta `json:"versions"`
}

// VerMeta describes one version of a secret.
type VerMeta struct {
	Version   int        `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	Destroyed bool       `json:"destroyed"`
}

// Store implements KV v2 operations backed by the barrier.
type Store struct {
	b barrierer
}

// New creates a Store backed by the given barrier.
func New(b barrierer) *Store { return &Store{b: b} }

// Write stores value as a new version under p. If cas is non-nil the write
// fails unless the current version equals *cas (check-and-set). Returns the
// new version number.
func (s *Store) Write(ctx context.Context, p string, value []byte, cas *int) (int, error) {
	meta, err := s.getMeta(ctx, p)
	if err != nil {
		return 0, err
	}
	if meta == nil {
		meta = &KVMeta{MaxVersions: defaultMax, Versions: map[int]*VerMeta{}}
	}
	if cas != nil && meta.CurrentVersion != *cas {
		return 0, fmt.Errorf("CAS mismatch: expected version %d, current is %d", *cas, meta.CurrentVersion)
	}

	newVer := meta.CurrentVersion + 1
	meta.CurrentVersion = newVer
	meta.Versions[newVer] = &VerMeta{Version: newVer, CreatedAt: time.Now().UTC()}

	// Enforce max_versions: permanently destroy the oldest version.
	if meta.MaxVersions > 0 && len(meta.Versions) > meta.MaxVersions {
		oldest := newVer - meta.MaxVersions
		if vm, ok := meta.Versions[oldest]; ok && !vm.Destroyed {
			_ = s.b.Delete(ctx, dataKey(p, oldest))
			vm.Destroyed = true
		}
	}

	if err := s.b.Put(ctx, &physical.Entry{Key: dataKey(p, newVer), Value: value}); err != nil {
		return 0, fmt.Errorf("store version data: %w", err)
	}
	if err := s.putMeta(ctx, p, meta); err != nil {
		return 0, fmt.Errorf("store metadata: %w", err)
	}
	return newVer, nil
}

// Read returns the value and metadata for a specific version. version=0 means
// the current (latest) version. Returns (nil, nil, nil) if the path has never
// been written. Returns (nil, vm, nil) for a soft-deleted version.
func (s *Store) Read(ctx context.Context, p string, version int) ([]byte, *VerMeta, error) {
	meta, err := s.getMeta(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	if meta == nil || meta.CurrentVersion == 0 {
		return nil, nil, nil
	}
	if version == 0 {
		version = meta.CurrentVersion
	}
	vm, ok := meta.Versions[version]
	if !ok {
		return nil, nil, fmt.Errorf("version %d not found", version)
	}
	if vm.Destroyed {
		return nil, vm, fmt.Errorf("version %d has been permanently destroyed", version)
	}
	if vm.DeletedAt != nil {
		return nil, vm, nil // soft-deleted: metadata visible, value hidden
	}
	e, err := s.b.Get(ctx, dataKey(p, version))
	if err != nil {
		return nil, nil, err
	}
	if e == nil {
		return nil, nil, fmt.Errorf("version %d data missing from storage", version)
	}
	return e.Value, vm, nil
}

// SoftDelete marks the listed versions as deleted. Data is preserved and can
// be recovered with Undelete.
func (s *Store) SoftDelete(ctx context.Context, p string, versions []int) error {
	meta, err := s.getMeta(ctx, p)
	if err != nil || meta == nil {
		return err
	}
	now := time.Now().UTC()
	for _, v := range versions {
		if vm, ok := meta.Versions[v]; ok && !vm.Destroyed && vm.DeletedAt == nil {
			t := now
			vm.DeletedAt = &t
		}
	}
	return s.putMeta(ctx, p, meta)
}

// Undelete clears the soft-delete flag from the listed versions.
func (s *Store) Undelete(ctx context.Context, p string, versions []int) error {
	meta, err := s.getMeta(ctx, p)
	if err != nil || meta == nil {
		return err
	}
	for _, v := range versions {
		if vm, ok := meta.Versions[v]; ok && !vm.Destroyed {
			vm.DeletedAt = nil
		}
	}
	return s.putMeta(ctx, p, meta)
}

// Destroy permanently removes the data for the listed versions. This cannot
// be undone.
func (s *Store) Destroy(ctx context.Context, p string, versions []int) error {
	meta, err := s.getMeta(ctx, p)
	if err != nil || meta == nil {
		return err
	}
	for _, v := range versions {
		if vm, ok := meta.Versions[v]; ok && !vm.Destroyed {
			_ = s.b.Delete(ctx, dataKey(p, v))
			vm.Destroyed = true
		}
	}
	return s.putMeta(ctx, p, meta)
}

// GetMeta returns version metadata for p, or nil if p has no versions.
func (s *Store) GetMeta(ctx context.Context, p string) (*KVMeta, error) {
	return s.getMeta(ctx, p)
}

// UpdateMeta changes the max_versions limit for a path.
func (s *Store) UpdateMeta(ctx context.Context, p string, maxVersions int) error {
	meta, err := s.getMeta(ctx, p)
	if err != nil {
		return err
	}
	if meta == nil {
		meta = &KVMeta{MaxVersions: maxVersions, Versions: map[int]*VerMeta{}}
	} else {
		meta.MaxVersions = maxVersions
	}
	return s.putMeta(ctx, p, meta)
}

// DeleteAll removes every version and the metadata entry for p.
func (s *Store) DeleteAll(ctx context.Context, p string) error {
	meta, err := s.getMeta(ctx, p)
	if err != nil {
		return err
	}
	if meta == nil {
		return nil
	}
	for v := range meta.Versions {
		_ = s.b.Delete(ctx, dataKey(p, v))
	}
	return s.b.Delete(ctx, metaKey(p))
}

// List returns the secret paths under prefix (paths with at least one version,
// strips the internal kv2/meta/ prefix).
func (s *Store) List(ctx context.Context, prefix string) ([]string, error) {
	storagePrefix := metaPrefix
	queryPrefix := ""
	if prefix != "" {
		clean := path.Clean("/" + prefix)[1:]
		storagePrefix = metaPrefix + clean
		if !strings.HasSuffix(storagePrefix, "/") {
			storagePrefix += "/"
		}
		queryPrefix = clean
		if !strings.HasSuffix(queryPrefix, "/") {
			queryPrefix += "/"
		}
	}
	keys, err := s.b.List(ctx, storagePrefix)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var result []string
	for _, k := range keys {
		rel := strings.TrimPrefix(k, metaPrefix)
		rel = strings.TrimPrefix(rel, queryPrefix)
		if rel == "" {
			continue
		}
		if idx := strings.Index(rel, "/"); idx >= 0 {
			folder := rel[:idx+1]
			if _, dup := seen[folder]; !dup {
				seen[folder] = struct{}{}
				result = append(result, folder)
			}
		} else {
			if _, dup := seen[rel]; !dup {
				seen[rel] = struct{}{}
				result = append(result, rel)
			}
		}
	}
	sort.Strings(result)
	return result, nil
}

func (s *Store) getMeta(ctx context.Context, p string) (*KVMeta, error) {
	e, err := s.b.Get(ctx, metaKey(p))
	if err != nil || e == nil {
		return nil, err
	}
	var m KVMeta
	if err := json.Unmarshal(e.Value, &m); err != nil {
		return nil, fmt.Errorf("decode kv2 metadata for %q: %w", p, err)
	}
	if m.Versions == nil {
		m.Versions = map[int]*VerMeta{}
	}
	return &m, nil
}

func (s *Store) putMeta(ctx context.Context, p string, m *KVMeta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode kv2 metadata: %w", err)
	}
	return s.b.Put(ctx, &physical.Entry{Key: metaKey(p), Value: b})
}

func metaKey(p string) string {
	return metaPrefix + path.Clean("/"+p)[1:]
}

func dataKey(p string, version int) string {
	return fmt.Sprintf("%s%s/%d", dataPrefix, path.Clean("/"+p)[1:], version)
}
