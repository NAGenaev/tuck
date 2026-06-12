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
