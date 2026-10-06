// Package wrapping implements Tuck's response-wrapping subsystem.
//
// Any JSON payload can be sealed inside a single-use wrapping token with a
// short TTL. The caller passes the token to a consumer; the consumer unwraps it
// to retrieve the payload. Because the token is deleted on first use, a failed
// unwrap proves that someone else has already read the secret.
//
// Storage layout:
//
//	sys/wrapping/<id>  →  JSON record {payload, created_at, expires_at, creator…}
//	sys/wrapping-stats →  JSON lifetime counters {wrapped, unwrapped, expired, revoked}
//
// The storage key <id> is the secret half of the wrapping token, so it is never
// exposed. Listing and revoking without the token go through an accessor: a
// one-way SHA-256 derivation of <id> that cannot be used to unwrap.
package wrapping

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	ErrNotFound = errors.New("wrapping: token not found or already used")
	ErrExpired  = errors.New("wrapping: token has expired")
)

const (
	storagePrefix = "sys/wrapping/"
	statsKey      = "sys/wrapping-stats"
	TokenPrefix   = "tuck_wrap_"
	// AccessorPrefix marks a wrapping accessor: a listing/revoke handle that is
	// NOT a credential and cannot be used to unwrap.
	AccessorPrefix = "tuck_wacc_"

	DefaultTTL = 5 * time.Minute
	MaxTTL     = 24 * time.Hour

	// MaxCreationPathLen bounds the caller-supplied creation path label.
	MaxCreationPathLen = 256
)

type barrierIface interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// record is what we persist in the barrier for each wrapping token.
//
// The creator fields are optional: records written before they existed still
// load (they just have no creator, so only administrators can see them).
type record struct {
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
	ExpiresAt time.Time       `json:"expires_at"`

	CreatorAccessor    string `json:"creator_accessor,omitempty"`
	CreatorFingerprint string `json:"creator_fingerprint,omitempty"`
	CreatorName        string `json:"creator_name,omitempty"`
	Namespace          string `json:"namespace,omitempty"` // "" = root
	CreationPath       string `json:"creation_path,omitempty"`
}

// Meta describes who created a wrapped response. All fields are optional.
type Meta struct {
	CreatorAccessor    string // accessor of the creating token (never the token itself)
	CreatorFingerprint string // truncated SHA-256 of the creating token, as in the audit log
	CreatorName        string // display name of the creating token
	Namespace          string // namespace the wrap was created in; "" or "root" = root
	CreationPath       string // caller-supplied label of where the data came from
}

// Capsule is the metadata of a live wrapped response. It never carries the
// wrapping token or the payload.
type Capsule struct {
	Accessor           string    `json:"accessor"`
	CreatedAt          time.Time `json:"creation_time"`
	ExpiresAt          time.Time `json:"expires_at"`
	CreationTTL        int       `json:"creation_ttl"` // seconds
	Size               int       `json:"size"`         // payload bytes
	Namespace          string    `json:"namespace"`    // "root" for the root namespace
	CreatorAccessor    string    `json:"creator_accessor,omitempty"`
	CreatorFingerprint string    `json:"creator_fingerprint,omitempty"`
	CreatorName        string    `json:"creator_name,omitempty"`
	CreationPath       string    `json:"creation_path,omitempty"`
}

// Stats are lifetime counters, persisted in the barrier.
type Stats struct {
	Wrapped   uint64    `json:"wrapped"`
	Unwrapped uint64    `json:"unwrapped"`
	Expired   uint64    `json:"expired"`
	Revoked   uint64    `json:"revoked"`
	Since     time.Time `json:"since"`
}

// TokenInfo is returned by Lookup.
type TokenInfo struct {
	CreatedAt   time.Time `json:"creation_time"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreationTTL int       `json:"creation_ttl"` // seconds
}

// Store manages wrapping tokens in the barrier.
type Store struct {
	b  barrierIface
	mu sync.Mutex // serialises every mutation: single-use tokens and exact counters

	stats       Stats
	statsLoaded bool
}

// NewStore returns a Store backed by b.
func NewStore(b barrierIface) *Store {
	return &Store{b: b}
}

// Wrap seals payload under a new single-use token valid for ttl.
// Returns the wrapping token string (prefix tuck_wrap_…).
func (s *Store) Wrap(ctx context.Context, payload json.RawMessage, ttl time.Duration) (string, time.Time, error) {
	return s.WrapWithMeta(ctx, payload, ttl, Meta{})
}

// WrapWithMeta is Wrap that also records who created the capsule.
func (s *Store) WrapWithMeta(ctx context.Context, payload json.RawMessage, ttl time.Duration, meta Meta) (string, time.Time, error) {
	if ttl == 0 {
		ttl = DefaultTTL
	}
	if ttl > MaxTTL {
		ttl = MaxTTL
	}

	id, err := randomID()
	if err != nil {
		return "", time.Time{}, err
	}
	token := TokenPrefix + id

	ns := meta.Namespace
	if ns == RootNamespace {
		ns = ""
	}
	path := meta.CreationPath
	if len(path) > MaxCreationPathLen {
		path = path[:MaxCreationPathLen]
	}

	now := time.Now()
	expiresAt := now.Add(ttl)
	rec := record{
		Payload:            payload,
		CreatedAt:          now,
		ExpiresAt:          expiresAt,
		CreatorAccessor:    meta.CreatorAccessor,
		CreatorFingerprint: meta.CreatorFingerprint,
		CreatorName:        meta.CreatorName,
		Namespace:          ns,
		CreationPath:       path,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return "", time.Time{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.b.Put(ctx, &physical.Entry{Key: storagePrefix + id, Value: data}); err != nil {
		return "", time.Time{}, err
	}
	s.bump(ctx, func(st *Stats) { st.Wrapped++ })
	return token, expiresAt, nil
}

// RootNamespace is how Capsule.Namespace names the root namespace.
const RootNamespace = "root"

// Unwrap consumes the wrapping token and returns its payload.
// The token is deleted atomically — a second call always returns ErrNotFound.
func (s *Store) Unwrap(ctx context.Context, token string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := tokenID(token)
	if id == "" {
		return nil, ErrNotFound
	}
	key := storagePrefix + id

	entry, err := s.b.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, ErrNotFound
	}

	var rec record
	if err := json.Unmarshal(entry.Value, &rec); err != nil {
		return nil, err
	}

	// Delete before returning — single use.
	if err := s.b.Delete(ctx, key); err != nil {
		return nil, err
	}

	if time.Now().After(rec.ExpiresAt) {
		s.bump(ctx, func(st *Stats) { st.Expired++ })
		return nil, ErrExpired
	}
	s.bump(ctx, func(st *Stats) { st.Unwrapped++ })
	return rec.Payload, nil
}

// Lookup returns metadata about a wrapping token without consuming it.
func (s *Store) Lookup(ctx context.Context, token string) (*TokenInfo, error) {
	id := tokenID(token)
	if id == "" {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, err := s.b.Get(ctx, storagePrefix+id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, ErrNotFound
	}
	var rec record
	if err := json.Unmarshal(entry.Value, &rec); err != nil {
		return nil, err
	}
	if time.Now().After(rec.ExpiresAt) {
		if s.b.Delete(ctx, storagePrefix+id) == nil {
			s.bump(ctx, func(st *Stats) { st.Expired++ })
		}
		return nil, ErrExpired
	}
	return &TokenInfo{
		CreatedAt:   rec.CreatedAt,
		ExpiresAt:   rec.ExpiresAt,
		CreationTTL: int(rec.ExpiresAt.Sub(rec.CreatedAt).Seconds()),
	}, nil
}

// Revoke explicitly deletes a wrapping token before it expires.
func (s *Store) Revoke(ctx context.Context, token string) error {
	id := tokenID(token)
	if id == "" {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revokeKey(ctx, storagePrefix+id)
}

// revokeKey deletes one record. An already-expired record counts as expired
// (not revoked) and reports ErrNotFound: it is gone either way. Caller holds mu.
func (s *Store) revokeKey(ctx context.Context, key string) error {
	entry, err := s.b.Get(ctx, key)
	if err != nil {
		return err
	}
	if entry == nil {
		return ErrNotFound
	}
	var rec record
	if err := json.Unmarshal(entry.Value, &rec); err != nil {
		// Unreadable record: still let the operator remove it.
		if derr := s.b.Delete(ctx, key); derr != nil {
			return derr
		}
		s.bump(ctx, func(st *Stats) { st.Revoked++ })
		return nil
	}
	if err := s.b.Delete(ctx, key); err != nil {
		return err
	}
	if time.Now().After(rec.ExpiresAt) {
		s.bump(ctx, func(st *Stats) { st.Expired++ })
		return ErrNotFound
	}
	s.bump(ctx, func(st *Stats) { st.Revoked++ })
	return nil
}

// RevokeExpired deletes all expired wrapping tokens and counts them. Called by
// background GC.
func (s *Store) RevokeExpired(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.scan(ctx)
	return err
}

// scan walks every record, deleting (and counting) expired ones, and returns
// the live ones keyed by storage key. Caller holds mu.
func (s *Store) scan(ctx context.Context) (map[string]*record, error) {
	keys, err := s.b.List(ctx, storagePrefix)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	live := make(map[string]*record, len(keys))
	for _, fullKey := range keys {
		entry, err := s.b.Get(ctx, fullKey)
		if err != nil || entry == nil {
			continue
		}
		var rec record
		if err := json.Unmarshal(entry.Value, &rec); err != nil {
			continue
		}
		if rec.ExpiresAt.Before(now) {
			if s.b.Delete(ctx, fullKey) == nil {
				s.bump(ctx, func(st *Stats) { st.Expired++ })
			}
			continue
		}
		live[fullKey] = &rec
	}
	return live, nil
}

// List returns the metadata of live wrapped responses for which keep returns
// true (nil keeps all), newest first. Expired capsules found on the way are
// deleted and counted. Neither tokens nor payloads are ever returned.
func (s *Store) List(ctx context.Context, keep func(*Capsule) bool) ([]Capsule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	live, err := s.scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Capsule, 0, len(live))
	for key, rec := range live {
		c := capsuleOf(strings.TrimPrefix(key, storagePrefix), rec)
		if keep != nil && !keep(&c) {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].Accessor < out[j].Accessor
	})
	return out, nil
}

// RevokeByAccessor destroys the capsule identified by accessor. allow, when
// non-nil, decides whether the caller may revoke that capsule; a capsule the
// caller may not touch is reported as ErrNotFound so existence does not leak.
func (s *Store) RevokeByAccessor(ctx context.Context, accessor string, allow func(*Capsule) bool) error {
	if !strings.HasPrefix(accessor, AccessorPrefix) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	live, err := s.scan(ctx)
	if err != nil {
		return err
	}
	for key, rec := range live {
		id := strings.TrimPrefix(key, storagePrefix)
		if accessorOf(id) != accessor {
			continue
		}
		if allow != nil {
			c := capsuleOf(id, rec)
			if !allow(&c) {
				return ErrNotFound
			}
		}
		return s.revokeKey(ctx, key)
	}
	return ErrNotFound
}

// Stats returns the lifetime counters.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadStats(ctx); err != nil {
		return Stats{}, err
	}
	return s.stats, nil
}

// loadStats reads the persisted counters once. Caller holds mu.
func (s *Store) loadStats(ctx context.Context) error {
	if s.statsLoaded {
		return nil
	}
	entry, err := s.b.Get(ctx, statsKey)
	if err != nil {
		return err
	}
	var st Stats
	if entry != nil {
		if err := json.Unmarshal(entry.Value, &st); err != nil {
			return err
		}
	}
	if st.Since.IsZero() {
		st.Since = time.Now().UTC()
	}
	s.stats, s.statsLoaded = st, true
	return nil
}

// bump applies fn to the counters and persists them. Counter failures never
// fail the wrapping operation itself. Caller holds mu.
func (s *Store) bump(ctx context.Context, fn func(*Stats)) {
	if err := s.loadStats(ctx); err != nil {
		return
	}
	fn(&s.stats)
	data, err := json.Marshal(s.stats)
	if err != nil {
		return
	}
	_ = s.b.Put(ctx, &physical.Entry{Key: statsKey, Value: data})
}

func capsuleOf(id string, rec *record) Capsule {
	ns := rec.Namespace
	if ns == "" {
		ns = RootNamespace
	}
	return Capsule{
		Accessor:           accessorOf(id),
		CreatedAt:          rec.CreatedAt,
		ExpiresAt:          rec.ExpiresAt,
		CreationTTL:        int(rec.ExpiresAt.Sub(rec.CreatedAt).Seconds()),
		Size:               len(rec.Payload),
		Namespace:          ns,
		CreatorAccessor:    rec.CreatorAccessor,
		CreatorFingerprint: rec.CreatorFingerprint,
		CreatorName:        rec.CreatorName,
		CreationPath:       rec.CreationPath,
	}
}

// accessorOf derives the public accessor of the token whose bare ID is id.
// It is a one-way hash of a 256-bit random value: it identifies the capsule
// but reveals nothing that could be used to unwrap it, and it works for
// records written before accessors existed.
func accessorOf(id string) string {
	h := sha256.Sum256([]byte("tuck-wrapping-accessor/" + id))
	return AccessorPrefix + base64.RawURLEncoding.EncodeToString(h[:16])
}

// tokenID extracts the bare random ID from a wrapping token string.
// Returns "" if the token does not have the expected prefix.
func tokenID(token string) string {
	if !strings.HasPrefix(token, TokenPrefix) {
		return ""
	}
	return strings.TrimPrefix(token, TokenPrefix)
}

func randomID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
