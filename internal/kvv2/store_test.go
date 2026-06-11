package kvv2

import (
	"context"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

func TestWriteRead(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	ver, err := s.Write(ctx, "db/password", []byte("s3cr3t"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ver != 1 {
		t.Fatalf("expected version 1, got %d", ver)
	}

	val, vm, err := s.Read(ctx, "db/password", 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "s3cr3t" {
		t.Fatalf("got %q", val)
	}
	if vm.Version != 1 {
		t.Fatalf("expected meta version 1, got %d", vm.Version)
	}
}

func TestMultipleVersions(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	s.Write(ctx, "k", []byte("v1"), nil)
	s.Write(ctx, "k", []byte("v2"), nil)
	v3, _ := s.Write(ctx, "k", []byte("v3"), nil)

	val, _, _ := s.Read(ctx, "k", 0)
	if string(val) != "v3" {
		t.Fatalf("expected v3, got %s", val)
	}

	val, _, _ = s.Read(ctx, "k", 1)
	if string(val) != "v1" {
		t.Fatalf("expected v1, got %s", val)
	}

	if v3 != 3 {
		t.Fatalf("expected version 3, got %d", v3)
	}
}

func TestCAS(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	s.Write(ctx, "k", []byte("v1"), nil)

	cas := 1
	_, err := s.Write(ctx, "k", []byte("v2"), &cas)
	if err != nil {
		t.Fatalf("CAS with correct version: %v", err)
	}

	wrongCAS := 0
	_, err = s.Write(ctx, "k", []byte("v3"), &wrongCAS)
	if err == nil {
		t.Fatal("expected CAS mismatch error")
	}
}

func TestSoftDelete(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	s.Write(ctx, "k", []byte("v1"), nil)
	if err := s.SoftDelete(ctx, "k", []int{1}); err != nil {
		t.Fatal(err)
	}

	val, vm, err := s.Read(ctx, "k", 1)
	if err != nil {
		t.Fatal(err)
	}
	if val != nil {
		t.Fatal("expected nil value for soft-deleted version")
	}
	if vm.DeletedAt == nil {
		t.Fatal("expected DeletedAt to be set")
	}
}

func TestUndelete(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	s.Write(ctx, "k", []byte("v1"), nil)
	s.SoftDelete(ctx, "k", []int{1})
	s.Undelete(ctx, "k", []int{1})

	val, _, err := s.Read(ctx, "k", 1)
	if err != nil || string(val) != "v1" {
		t.Fatalf("expected v1 after undelete, got %q err=%v", val, err)
	}
}

func TestDestroy(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	s.Write(ctx, "k", []byte("secret"), nil)
	if err := s.Destroy(ctx, "k", []int{1}); err != nil {
		t.Fatal(err)
	}

	_, _, err := s.Read(ctx, "k", 1)
	if err == nil {
		t.Fatal("expected error reading destroyed version")
	}
}

func TestDeleteAll(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	s.Write(ctx, "k", []byte("v1"), nil)
	s.Write(ctx, "k", []byte("v2"), nil)
	if err := s.DeleteAll(ctx, "k"); err != nil {
		t.Fatal(err)
	}

	meta, err := s.GetMeta(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if meta != nil {
		t.Fatal("expected nil metadata after DeleteAll")
	}
}

func TestList(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	s.Write(ctx, "db/password", []byte("x"), nil)
	s.Write(ctx, "db/user", []byte("y"), nil)
	s.Write(ctx, "other/key", []byte("z"), nil)

	keys, err := s.List(ctx, "db/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys under db/, got %d: %v", len(keys), keys)
	}
}

func TestMaxVersionsEnforced(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	s.UpdateMeta(ctx, "k", 2)
	s.Write(ctx, "k", []byte("v1"), nil)
	s.Write(ctx, "k", []byte("v2"), nil)
	s.Write(ctx, "k", []byte("v3"), nil) // should destroy v1

	_, _, err := s.Read(ctx, "k", 1)
	if err == nil {
		t.Fatal("expected error reading destroyed version 1")
	}

	val, _, err := s.Read(ctx, "k", 2)
	if err != nil || string(val) != "v2" {
		t.Fatalf("expected v2, got %q err=%v", val, err)
	}
}

// --- in-memory barrier for tests ---

type memBarrier struct{ data map[string][]byte }

func newMem() *memBarrier { return &memBarrier{data: map[string][]byte{}} }

func (m *memBarrier) Get(_ context.Context, key string) (*physical.Entry, error) {
	v, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	return &physical.Entry{Key: key, Value: v}, nil
}

func (m *memBarrier) Put(_ context.Context, e *physical.Entry) error {
	m.data[e.Key] = append([]byte{}, e.Value...)
	return nil
}

func (m *memBarrier) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *memBarrier) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	return keys, nil
}
