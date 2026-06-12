package cubbyhole_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/NAGenaev/tuck/internal/cubbyhole"
	"github.com/NAGenaev/tuck/internal/physical"
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

func (m *memBarrier) count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// --- tests ---

func TestPutGet(t *testing.T) {
	s := cubbyhole.NewStore(newMemBarrier())
	ctx := context.Background()

	data := map[string]interface{}{"key": "value", "num": float64(42)}
	if err := s.Put(ctx, "tok-1", "mypath", data); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, "tok-1", "mypath")
	if err != nil {
		t.Fatal(err)
	}
	if got["key"] != "value" {
		t.Fatalf("unexpected key: %v", got)
	}
	if got["num"] != float64(42) {
		t.Fatalf("unexpected num: %v", got)
	}
}

func TestGetNotFound(t *testing.T) {
	s := cubbyhole.NewStore(newMemBarrier())
	ctx := context.Background()

	_, err := s.Get(ctx, "tok-1", "missing")
	if !errors.Is(err, cubbyhole.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	s := cubbyhole.NewStore(newMemBarrier())
	ctx := context.Background()

	_ = s.Put(ctx, "tok-1", "p", map[string]interface{}{"x": 1})
	if err := s.Delete(ctx, "tok-1", "p"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Get(ctx, "tok-1", "p")
	if !errors.Is(err, cubbyhole.ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestList(t *testing.T) {
	s := cubbyhole.NewStore(newMemBarrier())
	ctx := context.Background()

	for _, p := range []string{"a", "b", "c/d"} {
		_ = s.Put(ctx, "tok-1", p, map[string]interface{}{"v": p})
	}

	keys, err := s.List(ctx, "tok-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("want 3 keys, got %d: %v", len(keys), keys)
	}
}

func TestIsolation(t *testing.T) {
	s := cubbyhole.NewStore(newMemBarrier())
	ctx := context.Background()

	_ = s.Put(ctx, "tok-A", "secret", map[string]interface{}{"v": "A"})
	_ = s.Put(ctx, "tok-B", "secret", map[string]interface{}{"v": "B"})

	gotA, _ := s.Get(ctx, "tok-A", "secret")
	gotB, _ := s.Get(ctx, "tok-B", "secret")

	if gotA["v"] != "A" || gotB["v"] != "B" {
		t.Fatalf("isolation broken: A=%v B=%v", gotA, gotB)
	}

	// tok-A cannot read tok-B
	_, err := s.Get(ctx, "tok-A", "only-in-b")
	if !errors.Is(err, cubbyhole.ErrNotFound) {
		t.Fatalf("want ErrNotFound for cross-token read, got %v", err)
	}
}

func TestPurgeToken(t *testing.T) {
	mb := newMemBarrier()
	s := cubbyhole.NewStore(mb)
	ctx := context.Background()

	_ = s.Put(ctx, "tok-1", "a", map[string]interface{}{"x": 1})
	_ = s.Put(ctx, "tok-1", "b", map[string]interface{}{"x": 2})
	_ = s.Put(ctx, "tok-2", "a", map[string]interface{}{"x": 3}) // different token

	if err := s.PurgeToken(ctx, "tok-1"); err != nil {
		t.Fatal(err)
	}

	// tok-1 entries gone.
	_, err := s.Get(ctx, "tok-1", "a")
	if !errors.Is(err, cubbyhole.ErrNotFound) {
		t.Fatal("expected tok-1/a to be purged")
	}
	_, err = s.Get(ctx, "tok-1", "b")
	if !errors.Is(err, cubbyhole.ErrNotFound) {
		t.Fatal("expected tok-1/b to be purged")
	}

	// tok-2 entry survives.
	got, err := s.Get(ctx, "tok-2", "a")
	if err != nil {
		t.Fatalf("tok-2/a should survive purge of tok-1: %v", err)
	}
	if got["x"] != float64(3) {
		t.Fatalf("unexpected value: %v", got)
	}
}

func TestOverwrite(t *testing.T) {
	s := cubbyhole.NewStore(newMemBarrier())
	ctx := context.Background()

	_ = s.Put(ctx, "tok-1", "p", map[string]interface{}{"v": "original"})
	_ = s.Put(ctx, "tok-1", "p", map[string]interface{}{"v": "updated"})

	got, err := s.Get(ctx, "tok-1", "p")
	if err != nil {
		t.Fatal(err)
	}
	if got["v"] != "updated" {
		t.Fatalf("want updated, got %v", got)
	}
}
