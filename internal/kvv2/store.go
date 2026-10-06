// Package kvv2 implements versioned key-value storage (KV engine v2) on top of
// the barrier. Each write creates a new immutable version; versions can be
// soft-deleted (data hidden, recoverable) or destroyed (data wiped forever).
package kvv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/NAGenaev/tuck/internal/physical"
)

const (
	metaPrefix = "kv2/meta/"
	dataPrefix = "kv2/data/"
	defaultMax = 10
)

// Limits for the per-secret metadata (same order of magnitude as Vault's).
const (
	MaxCustomMetadataPairs = 64
	MaxCustomMetadataKey   = 128
	MaxCustomMetadataValue = 512
	// MaxVersionsLimit is the largest accepted max_versions value.
	MaxVersionsLimit = 1000
)

// Sentinel errors; callers map them to HTTP statuses with errors.Is.
var (
	// ErrCASMismatch is returned when the supplied cas does not equal the
	// current version of the secret.
	ErrCASMismatch = errors.New("CAS mismatch")
	// ErrCASRequired is returned for a write without cas when cas_required is
	// enabled for the secret or the mount.
	ErrCASRequired = errors.New("check-and-set parameter required for this secret")
	// ErrInvalidMetadata wraps validation failures of metadata updates.
	ErrInvalidMetadata = errors.New("invalid metadata")
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
	CurrentVersion int `json:"current_version"`
	// OldestVersion is the lowest version still present in Versions. When it is
	// greater than 1, older versions were evicted by the max_versions cap.
	OldestVersion int `json:"oldest_version"`
	// MaxVersions caps the number of retained versions; 0 means unlimited.
	MaxVersions int `json:"max_versions"`
	// CASRequired makes every write to this secret carry a cas parameter.
	CASRequired bool `json:"cas_required"`
	// CustomMetadata is user-defined, non-secret key/value data on the path.
	CustomMetadata map[string]string `json:"custom_metadata,omitempty"`
	// CreatedAt is when the path was first written; UpdatedAt the latest write.
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Versions  map[int]*VerMeta `json:"versions"`
}

// VerMeta describes one version of a secret.
type VerMeta struct {
	Version   int        `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy *Author    `json:"created_by,omitempty"` // nil for versions written before authorship was recorded
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	Destroyed bool       `json:"destroyed"`
}

// Author identifies the token that wrote a version. It never contains the
// token ID itself: only the display name, the public accessor and the same
// short fingerprint the audit log uses.
type Author struct {
	DisplayName string `json:"display_name,omitempty"`
	Accessor    string `json:"accessor,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	EntityID    string `json:"entity_id,omitempty"`
}

// Label returns a short human-readable identity: display name, else accessor,
// else fingerprint. Empty for a nil or empty author.
func (a *Author) Label() string {
	if a == nil {
		return ""
	}
	switch {
	case a.DisplayName != "":
		return a.DisplayName
	case a.Accessor != "":
		return a.Accessor
	default:
		return a.Fingerprint
	}
}

type authorKey struct{}

// WithAuthor returns a context that makes Write record a as the author of the
// version it creates. Keeps Write's signature unchanged for existing callers.
func WithAuthor(ctx context.Context, a Author) context.Context {
	return context.WithValue(ctx, authorKey{}, a)
}

func authorFrom(ctx context.Context) *Author {
	a, ok := ctx.Value(authorKey{}).(Author)
	if !ok || a == (Author{}) {
		return nil
	}
	return &a
}

// Defaults are the mount-level KV v2 options.
type Defaults struct {
	// MaxVersions seeds max_versions for newly created secrets (0 = built-in 10).
	MaxVersions int
	// CASRequired forces cas on every write below the mount.
	CASRequired bool
}

// DefaultsFunc loads the current mount-level options.
type DefaultsFunc func(ctx context.Context) Defaults

// Store implements KV v2 operations backed by the barrier.
type Store struct {
	b        barrierer
	defaults DefaultsFunc
}

// New creates a Store backed by the given barrier.
func New(b barrierer) *Store { return &Store{b: b} }

// WithDefaults attaches a provider of mount-level options and returns s.
func (s *Store) WithDefaults(f DefaultsFunc) *Store {
	s.defaults = f
	return s
}

func (s *Store) mountDefaults(ctx context.Context) Defaults {
	if s.defaults == nil {
		return Defaults{}
	}
	return s.defaults(ctx)
}

// pathLocks serialise read-modify-write cycles on one secret so versions are
// numbered without gaps/duplicates and CAS is atomic. Striped by path hash;
// shared across Store instances (namespace views create their own Store).
var pathLocks [64]sync.Mutex

func lockPath(p string) func() {
	h := fnv.New32a()
	_, _ = h.Write([]byte(metaKey(p)))
	m := &pathLocks[h.Sum32()%uint32(len(pathLocks))]
	m.Lock()
	return m.Unlock
}

// Write stores value as a new version under p. If cas is non-nil the write
// fails unless the current version equals *cas (check-and-set); when cas_required
// is enabled for the secret or the mount, cas must be supplied (ErrCASRequired).
// The author is taken from ctx (see WithAuthor). Returns the new version number.
func (s *Store) Write(ctx context.Context, p string, value []byte, cas *int) (int, error) {
	defer lockPath(p)()
	meta, err := s.getMeta(ctx, p)
	if err != nil {
		return 0, err
	}
	def := s.mountDefaults(ctx)
	now := time.Now().UTC()
	if meta == nil {
		max := defaultMax
		if def.MaxVersions > 0 {
			max = def.MaxVersions
		}
		meta = &KVMeta{MaxVersions: max, CreatedAt: now, Versions: map[int]*VerMeta{}}
	}
	if cas == nil && (meta.CASRequired || def.CASRequired) {
		return 0, ErrCASRequired
	}
	if cas != nil && meta.CurrentVersion != *cas {
		return 0, fmt.Errorf("%w: expected version %d, current is %d", ErrCASMismatch, *cas, meta.CurrentVersion)
	}

	newVer := meta.CurrentVersion + 1
	meta.CurrentVersion = newVer
	meta.UpdatedAt = now
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.Versions[newVer] = &VerMeta{Version: newVer, CreatedAt: now, CreatedBy: authorFrom(ctx)}
	evicted := meta.evict()

	if err := s.b.Put(ctx, &physical.Entry{Key: dataKey(p, newVer), Value: value}); err != nil {
		return 0, fmt.Errorf("store version data: %w", err)
	}
	if err := s.putMeta(ctx, p, meta); err != nil {
		return 0, fmt.Errorf("store metadata: %w", err)
	}
	// Only after the metadata no longer references them is the data wiped.
	for _, v := range evicted {
		_ = s.b.Delete(ctx, dataKey(p, v))
	}
	return newVer, nil
}

// evict enforces max_versions: versions numbered at or below
// CurrentVersion-MaxVersions are dropped from the metadata. Returns the
// evicted version numbers (their data must still be deleted by the caller).
func (m *KVMeta) evict() []int {
	if m.MaxVersions <= 0 {
		m.normalize()
		return nil
	}
	cutoff := m.CurrentVersion - m.MaxVersions
	var gone []int
	for v := range m.Versions {
		if v <= cutoff {
			gone = append(gone, v)
		}
	}
	sort.Ints(gone)
	for _, v := range gone {
		delete(m.Versions, v)
	}
	m.normalize()
	return gone
}

// normalize derives fields that older stored entries do not carry.
func (m *KVMeta) normalize() {
	if m.Versions == nil {
		m.Versions = map[int]*VerMeta{}
	}
	oldest := 0
	var first, last time.Time
	for v, vm := range m.Versions {
		if vm == nil {
			delete(m.Versions, v)
			continue
		}
		if oldest == 0 || v < oldest {
			oldest = v
			first = vm.CreatedAt
		}
		if v == m.CurrentVersion {
			last = vm.CreatedAt
		}
	}
	m.OldestVersion = oldest
	if m.CreatedAt.IsZero() {
		m.CreatedAt = first
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = last
	}
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
	defer lockPath(p)()
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
	defer lockPath(p)()
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
	defer lockPath(p)()
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

// GetMeta returns version metadata for p, or nil if p has never been written.
func (s *Store) GetMeta(ctx context.Context, p string) (*KVMeta, error) {
	return s.getMeta(ctx, p)
}

// MountDefaults returns the mount-level options currently in effect.
func (s *Store) MountDefaults(ctx context.Context) Defaults { return s.mountDefaults(ctx) }

// UpdateMeta changes the max_versions limit for a path.
func (s *Store) UpdateMeta(ctx context.Context, p string, maxVersions int) error {
	_, err := s.UpdateMetadata(ctx, p, MetaUpdate{MaxVersions: &maxVersions})
	return err
}

// MetaUpdate describes a change to the configurable part of a secret's
// metadata. Nil/absent fields are left untouched.
type MetaUpdate struct {
	MaxVersions *int
	CASRequired *bool
	// CustomMetadata: in replace mode the map becomes the whole custom
	// metadata (nil values ignored); in merge mode keys are set, and a nil
	// value deletes the key. A nil map leaves custom metadata untouched.
	CustomMetadata map[string]*string
	Replace        bool
}

// UpdateMetadata applies u to the metadata of p, creating an empty metadata
// record (current_version 0) if the path has not been written yet. Returns
// the resulting metadata. Validation failures wrap ErrInvalidMetadata.
func (s *Store) UpdateMetadata(ctx context.Context, p string, u MetaUpdate) (*KVMeta, error) {
	if u.MaxVersions != nil && (*u.MaxVersions < 0 || *u.MaxVersions > MaxVersionsLimit) {
		return nil, fmt.Errorf("%w: max_versions must be between 0 and %d", ErrInvalidMetadata, MaxVersionsLimit)
	}
	defer lockPath(p)()
	meta, err := s.getMeta(ctx, p)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		max := defaultMax
		if d := s.mountDefaults(ctx).MaxVersions; d > 0 {
			max = d
		}
		meta = &KVMeta{MaxVersions: max, CreatedAt: time.Now().UTC(), Versions: map[int]*VerMeta{}}
	}
	if u.CustomMetadata != nil {
		next := map[string]string{}
		if !u.Replace {
			for k, v := range meta.CustomMetadata {
				next[k] = v
			}
		}
		for k, v := range u.CustomMetadata {
			if v == nil {
				delete(next, k)
			} else {
				next[k] = *v
			}
		}
		if err := validateCustom(next); err != nil {
			return nil, err
		}
		if len(next) == 0 {
			next = nil
		}
		meta.CustomMetadata = next
	}
	if u.MaxVersions != nil {
		meta.MaxVersions = *u.MaxVersions
	}
	if u.CASRequired != nil {
		meta.CASRequired = *u.CASRequired
	}
	if err := s.putMeta(ctx, p, meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func validateCustom(m map[string]string) error {
	if len(m) > MaxCustomMetadataPairs {
		return fmt.Errorf("%w: at most %d custom_metadata entries", ErrInvalidMetadata, MaxCustomMetadataPairs)
	}
	for k, v := range m {
		if k == "" || len(k) > MaxCustomMetadataKey {
			return fmt.Errorf("%w: custom_metadata key must be 1-%d bytes", ErrInvalidMetadata, MaxCustomMetadataKey)
		}
		if len(v) > MaxCustomMetadataValue {
			return fmt.Errorf("%w: custom_metadata value for %q exceeds %d bytes", ErrInvalidMetadata, k, MaxCustomMetadataValue)
		}
		if !validText(k) || !validText(v) {
			return fmt.Errorf("%w: custom_metadata must be valid UTF-8 without control characters", ErrInvalidMetadata)
		}
	}
	return nil
}

func validText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// DeleteAll removes every version and the metadata entry for p.
func (s *Store) DeleteAll(ctx context.Context, p string) error {
	defer lockPath(p)()
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

// listScope returns the storage prefix and the logical prefix (with trailing
// slash, or empty) for a listing under prefix.
func listScope(prefix string) (storagePrefix, queryPrefix string) {
	storagePrefix = metaPrefix
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
	return storagePrefix, queryPrefix
}

// List returns the secret paths under prefix (paths with at least one version,
// strips the internal kv2/meta/ prefix).
func (s *Store) List(ctx context.Context, prefix string) ([]string, error) {
	storagePrefix, queryPrefix := listScope(prefix)
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

// ListEntry is one row of a detailed listing. It is built from metadata only:
// secret values are never read.
type ListEntry struct {
	Name   string `json:"name"` // "key" or "folder/"
	Folder bool   `json:"folder"`
	// Count is the number of secrets below a folder (recursive).
	Count int `json:"count,omitempty"`

	// Restricted marks a secret the caller may list but not read metadata of.
	Restricted     bool      `json:"restricted,omitempty"`
	CurrentVersion int       `json:"current_version,omitempty"`
	Versions       int       `json:"versions,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitzero"`
	UpdatedBy      string    `json:"updated_by,omitempty"`
	MaxVersions    int       `json:"max_versions,omitempty"`
	CASRequired    bool      `json:"cas_required,omitempty"`
	// Deleted: the current version is soft-deleted. Destroyed: it was destroyed.
	Deleted   bool `json:"deleted,omitempty"`
	Destroyed bool `json:"destroyed,omitempty"`
}

// Totals aggregates every secret below a prefix.
type Totals struct {
	Secrets   int `json:"secrets"`
	Folders   int `json:"folders"`
	Versions  int `json:"versions"`
	Deleted   int `json:"deleted"`   // soft-deleted versions
	Destroyed int `json:"destroyed"` // destroyed versions
	// Hidden counts secrets skipped because the caller may not read them.
	Hidden int `json:"hidden,omitempty"`
}

// ListOptions tunes ListDetailed.
type ListOptions struct {
	// Totals also walks the whole subtree to fill Totals.
	Totals bool
	// Allow, when set, reports whether the caller may read metadata of the
	// secret at the given path (relative to the mount). Disallowed secrets
	// come back as Restricted rows and are left out of the totals.
	Allow func(secretPath string) bool
}

// ListDetailed lists the direct children of prefix like List, enriched with
// per-secret metadata (current version, time and author of the last write,
// flags), and optionally totals for the whole subtree. Only metadata entries
// are read.
func (s *Store) ListDetailed(ctx context.Context, prefix string, opt ListOptions) ([]ListEntry, *Totals, error) {
	storagePrefix, queryPrefix := listScope(prefix)
	keys, err := s.b.List(ctx, storagePrefix)
	if err != nil {
		return nil, nil, err
	}
	allow := func(string) bool { return true }
	if opt.Allow != nil {
		allow = opt.Allow
	}

	byName := map[string]*ListEntry{}
	var totals *Totals
	var dirs map[string]struct{}
	if opt.Totals {
		totals = &Totals{}
		dirs = map[string]struct{}{}
	}
	for _, k := range keys {
		full := strings.TrimPrefix(k, metaPrefix)
		rel := strings.TrimPrefix(full, queryPrefix)
		if rel == "" {
			continue
		}
		name := rel
		if idx := strings.Index(rel, "/"); idx >= 0 {
			name = rel[:idx+1]
		}
		e := byName[name]
		if e == nil {
			e = &ListEntry{Name: name, Folder: strings.HasSuffix(name, "/")}
			byName[name] = e
		}
		if e.Folder {
			e.Count++
		}
		if opt.Totals {
			for i := 0; i < len(rel); i++ {
				if rel[i] == '/' {
					dirs[rel[:i]] = struct{}{}
				}
			}
		}
		direct := !e.Folder
		if !direct && !opt.Totals {
			continue
		}
		if !allow(full) {
			if direct {
				e.Restricted = true
			}
			if opt.Totals {
				totals.Hidden++
			}
			continue
		}
		meta, err := s.getMeta(ctx, full)
		if err != nil {
			return nil, nil, err
		}
		if meta == nil {
			continue
		}
		if direct {
			fillEntry(e, meta)
		}
		if opt.Totals {
			totals.Secrets++
			totals.Versions += len(meta.Versions)
			for _, vm := range meta.Versions {
				if vm.Destroyed {
					totals.Destroyed++
				} else if vm.DeletedAt != nil {
					totals.Deleted++
				}
			}
		}
	}
	if totals != nil {
		totals.Folders = len(dirs)
	}
	out := make([]ListEntry, 0, len(byName))
	for _, e := range byName {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, totals, nil
}

func fillEntry(e *ListEntry, m *KVMeta) {
	e.CurrentVersion = m.CurrentVersion
	e.Versions = len(m.Versions)
	e.UpdatedAt = m.UpdatedAt
	e.MaxVersions = m.MaxVersions
	e.CASRequired = m.CASRequired
	if vm := m.Versions[m.CurrentVersion]; vm != nil {
		e.UpdatedBy = vm.CreatedBy.Label()
		e.Destroyed = vm.Destroyed
		e.Deleted = !vm.Destroyed && vm.DeletedAt != nil
	}
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
	// Entries stored before authorship/oldest_version/timestamps existed load
	// fine; the derived fields are filled in here.
	m.normalize()
	return &m, nil
}

func (s *Store) putMeta(ctx context.Context, p string, m *KVMeta) error {
	m.normalize()
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
