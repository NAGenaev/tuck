package wrapping_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/wrapping"
)

// --- in-memory barrier ---

type memBarrier struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMemBarrier() *memBarrier { return &memBarrier{data: make(map[string][]byte)} }

func (m *memBarrier) Get(_ context.Context, key string) (*physical.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return &physical.Entry{Key: key, Value: cp}, nil
}

func (m *memBarrier) Put(_ context.Context, entry *physical.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(entry.Value))
	copy(cp, entry.Value)
	m.data[entry.Key] = cp
	return nil
}

func (m *memBarrier) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *memBarrier) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

func newStore(t *testing.T) *wrapping.Store {
	t.Helper()
	return wrapping.NewStore(newMemBarrier())
}

func payload(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// --- tests ---

func TestWrapUnwrap(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	data := payload(t, map[string]string{"secret": "hunter2"})
	token, expiresAt, err := s.Wrap(ctx, data, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, wrapping.TokenPrefix) {
		t.Fatalf("token should start with %s, got %s", wrapping.TokenPrefix, token)
	}
	if expiresAt.IsZero() {
		t.Fatal("want non-zero expires_at")
	}

	got, err := s.Unwrap(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["secret"] != "hunter2" {
		t.Fatalf("unexpected payload: %v", m)
	}
}

func TestUnwrapSingleUse(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	token, _, err := s.Wrap(ctx, payload(t, "once"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Unwrap(ctx, token); err != nil {
		t.Fatal(err)
	}
	// Second call must fail — token consumed.
	_, err = s.Unwrap(ctx, token)
	if !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("want ErrNotFound on second unwrap, got %v", err)
	}
}

func TestUnwrapExpired(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	token, _, err := s.Wrap(ctx, payload(t, "short"), -1*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Unwrap(ctx, token)
	if !errors.Is(err, wrapping.ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}

	// Token must be cleaned up after expired unwrap attempt.
	_, err = s.Unwrap(ctx, token)
	if !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("want ErrNotFound after expired token cleaned up, got %v", err)
	}
}

func TestLookup(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	token, _, err := s.Wrap(ctx, payload(t, "look"), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	info, err := s.Lookup(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if info.CreationTTL != 30 {
		t.Fatalf("want 30s TTL, got %d", info.CreationTTL)
	}
	if info.ExpiresAt.IsZero() {
		t.Fatal("want non-zero expires_at")
	}

	// Lookup does NOT consume the token.
	if _, err := s.Unwrap(ctx, token); err != nil {
		t.Fatalf("token should still be valid after lookup: %v", err)
	}
}

func TestLookupNotFound(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	_, err := s.Lookup(ctx, wrapping.TokenPrefix+"nonexistent")
	if !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestRevoke(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	token, _, err := s.Wrap(ctx, payload(t, "revocable"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Revoke(ctx, token); err != nil {
		t.Fatal(err)
	}

	_, err = s.Unwrap(ctx, token)
	if !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("want ErrNotFound after revoke, got %v", err)
	}

	// Revoking a non-existent token returns ErrNotFound.
	err = s.Revoke(ctx, token)
	if !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("want ErrNotFound on double revoke, got %v", err)
	}
}

func TestRevokeExpired(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	// One expired, one valid.
	_, _, err := s.Wrap(ctx, payload(t, "old"), -1*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	validToken, _, err := s.Wrap(ctx, payload(t, "fresh"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.RevokeExpired(ctx); err != nil {
		t.Fatal(err)
	}

	// Valid token still works.
	if _, err := s.Unwrap(ctx, validToken); err != nil {
		t.Fatalf("valid token should survive RevokeExpired: %v", err)
	}
}

func TestInvalidTokenPrefix(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	_, err := s.Unwrap(ctx, "tuck_invalid_token")
	if !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("want ErrNotFound for wrong prefix, got %v", err)
	}
}

func TestDefaultAndMaxTTL(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	// Zero TTL → DefaultTTL applied.
	token, expiresAt, err := s.Wrap(ctx, payload(t, "x"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(expiresAt) < 4*time.Minute {
		t.Fatal("default TTL should be ~5 minutes")
	}
	_ = s.Revoke(ctx, token)

	// Oversized TTL → capped at MaxTTL.
	_, expiresAt, err = s.Wrap(ctx, payload(t, "y"), 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(expiresAt) > wrapping.MaxTTL+time.Minute {
		t.Fatal("TTL should be capped at MaxTTL")
	}
}

// --- listing, accessors, stats ---

func TestListMetadataOnly(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	meta := wrapping.Meta{
		CreatorAccessor: "tuck_acc_alice", CreatorFingerprint: "abc123", CreatorName: "alice",
		Namespace: "team-a", CreationPath: "secret/db",
	}
	body := payload(t, map[string]string{"secret": "hunter2"})
	token, _, err := s.WrapWithMeta(ctx, body, time.Hour, meta)
	if err != nil {
		t.Fatal(err)
	}

	list, err := s.List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 capsule, got %d", len(list))
	}
	c := list[0]
	if c.Size != len(body) || c.CreationTTL != 3600 || c.Namespace != "team-a" ||
		c.CreatorAccessor != "tuck_acc_alice" || c.CreatorFingerprint != "abc123" ||
		c.CreatorName != "alice" || c.CreationPath != "secret/db" {
		t.Fatalf("unexpected capsule: %+v", c)
	}
	if !strings.HasPrefix(c.Accessor, wrapping.AccessorPrefix) {
		t.Fatalf("bad accessor %q", c.Accessor)
	}

	// Neither the token (nor its secret half) nor the payload may leak.
	raw, _ := json.Marshal(list)
	secretHalf := strings.TrimPrefix(token, wrapping.TokenPrefix)
	for _, leak := range []string{token, secretHalf, "hunter2"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("list output leaks %q: %s", leak, raw)
		}
	}
	if c.Accessor == token || strings.Contains(c.Accessor, secretHalf) {
		t.Fatal("accessor must not contain the token")
	}
}

func TestListFilterRootNamespaceAndSweep(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	_, _, _ = s.WrapWithMeta(ctx, payload(t, 1), time.Hour, wrapping.Meta{Namespace: "root", CreatorAccessor: "a"})
	_, _, _ = s.WrapWithMeta(ctx, payload(t, 2), time.Hour, wrapping.Meta{Namespace: "ns1", CreatorAccessor: "b"})
	_, _, _ = s.WrapWithMeta(ctx, payload(t, 3), -time.Second, wrapping.Meta{CreatorAccessor: "a"}) // already expired

	all, err := s.List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expired capsule must not be listed: got %d", len(all))
	}
	mine, _ := s.List(ctx, func(c *wrapping.Capsule) bool { return c.CreatorAccessor == "a" })
	if len(mine) != 1 || mine[0].Namespace != "root" {
		t.Fatalf("filter/root namespace wrong: %+v", mine)
	}

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Wrapped != 3 || st.Expired != 1 {
		t.Fatalf("want wrapped=3 expired=1 (swept once), got %+v", st)
	}
	// A second list must not count the same capsule twice.
	_, _ = s.List(ctx, nil)
	if st2, _ := s.Stats(ctx); st2.Expired != 1 {
		t.Fatalf("expired double-counted: %+v", st2)
	}
}

func TestRevokeByAccessor(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	token, _, _ := s.WrapWithMeta(ctx, payload(t, "x"), time.Hour, wrapping.Meta{CreatorAccessor: "owner"})
	list, _ := s.List(ctx, nil)
	acc := list[0].Accessor

	// A caller who may not touch it sees ErrNotFound and the capsule survives.
	err := s.RevokeByAccessor(ctx, acc, func(c *wrapping.Capsule) bool { return c.CreatorAccessor == "someone-else" })
	if !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("want ErrNotFound for forbidden revoke, got %v", err)
	}
	if _, err := s.Lookup(ctx, token); err != nil {
		t.Fatalf("capsule must survive a refused revoke: %v", err)
	}

	if err := s.RevokeByAccessor(ctx, acc, func(c *wrapping.Capsule) bool { return c.CreatorAccessor == "owner" }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unwrap(ctx, token); !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("token must be dead after accessor revoke, got %v", err)
	}
	if err := s.RevokeByAccessor(ctx, acc, nil); !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("double revoke: want ErrNotFound, got %v", err)
	}
	if err := s.RevokeByAccessor(ctx, "garbage", nil); !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("bad accessor: want ErrNotFound, got %v", err)
	}
	// The token itself is not an accessor.
	if err := s.RevokeByAccessor(ctx, token, nil); !errors.Is(err, wrapping.ErrNotFound) {
		t.Fatalf("token passed as accessor must not work, got %v", err)
	}
}

func TestStatsCounters(t *testing.T) {
	b := newMemBarrier()
	s := wrapping.NewStore(b)
	ctx := context.Background()

	t1, _, _ := s.Wrap(ctx, payload(t, 1), time.Hour)
	t2, _, _ := s.Wrap(ctx, payload(t, 2), time.Hour)
	_, _, _ = s.Wrap(ctx, payload(t, 3), -time.Second)
	t4, _, _ := s.Wrap(ctx, payload(t, 4), -time.Second)

	if _, err := s.Unwrap(ctx, t1); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, t2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Unwrap(ctx, t4); !errors.Is(err, wrapping.ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
	if err := s.RevokeExpired(ctx); err != nil { // sweeps the remaining expired one
		t.Fatal(err)
	}
	// Failed attempts count nothing.
	_, _ = s.Unwrap(ctx, t1)
	_ = s.Revoke(ctx, t2)

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Wrapped != 4 || st.Unwrapped != 1 || st.Revoked != 1 || st.Expired != 2 {
		t.Fatalf("unexpected counters: %+v", st)
	}
	if st.Since.IsZero() {
		t.Fatal("Since must be set")
	}

	// Counters survive a restart (new Store over the same barrier).
	s2 := wrapping.NewStore(b)
	st2, err := s2.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st2.Wrapped != 4 || st2.Unwrapped != 1 || st2.Revoked != 1 || st2.Expired != 2 {
		t.Fatalf("counters not persisted: %+v", st2)
	}
}

func TestLegacyRecordWithoutCreator(t *testing.T) {
	b := newMemBarrier()
	// A record exactly as written before creator metadata existed.
	legacy := `{"payload":{"a":1},"created_at":"` + time.Now().Format(time.RFC3339Nano) +
		`","expires_at":"` + time.Now().Add(time.Hour).Format(time.RFC3339Nano) + `"}`
	_ = b.Put(context.Background(), &physical.Entry{Key: "sys/wrapping/legacyid", Value: []byte(legacy)})

	s := wrapping.NewStore(b)
	ctx := context.Background()
	list, err := s.List(ctx, nil)
	if err != nil || len(list) != 1 {
		t.Fatalf("legacy record must list: %v %+v", err, list)
	}
	if list[0].CreatorAccessor != "" || list[0].Namespace != "root" {
		t.Fatalf("unexpected legacy capsule: %+v", list[0])
	}
	if _, err := s.Unwrap(ctx, wrapping.TokenPrefix+"legacyid"); err != nil {
		t.Fatalf("legacy record must still unwrap: %v", err)
	}
}

func TestConcurrentCountersExact(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, _, err := s.Wrap(ctx, payload(t, "c"), time.Hour)
			if err == nil {
				_, _ = s.Unwrap(ctx, tok)
			}
		}()
	}
	wg.Wait()
	st, _ := s.Stats(ctx)
	if st.Wrapped != 20 || st.Unwrapped != 20 {
		t.Fatalf("counters drifted under concurrency: %+v", st)
	}
}
