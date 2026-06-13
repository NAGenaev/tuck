package mount

import (
	"context"
	"errors"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return New(physical.NewInMem())
}

func TestRegister_and_Get(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	e, err := s.Register(ctx, "my-kv/", "kv", "test store")
	if err != nil {
		t.Fatal(err)
	}
	if e.Path != "my-kv/" {
		t.Errorf("path = %q, want my-kv/", e.Path)
	}
	if e.Type != "kv" {
		t.Errorf("type = %q, want kv", e.Type)
	}
	if e.Builtin {
		t.Error("registered mount must not be builtin")
	}
	if e.Accessor == "" {
		t.Error("accessor must be set")
	}

	got, err := s.Get(ctx, "my-kv/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != e.Path || got.Accessor != e.Accessor {
		t.Errorf("Get returned %+v, want %+v", got, e)
	}
}

func TestRegister_duplicate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.Register(ctx, "dup/", "kv", ""); err != nil {
		t.Fatal(err)
	}
	_, err := s.Register(ctx, "dup/", "kv", "")
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate register err = %v, want ErrAlreadyExists", err)
	}
}

func TestRegisterBuiltin_idempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.RegisterBuiltin(ctx, "secret/", "kv", "Default KV"); err != nil {
		t.Fatal(err)
	}
	// Second call must not fail.
	if err := s.RegisterBuiltin(ctx, "secret/", "kv", "Default KV"); err != nil {
		t.Fatalf("second RegisterBuiltin: %v", err)
	}
	e, _ := s.Get(ctx, "secret/")
	if !e.Builtin {
		t.Error("builtin mount must have Builtin=true")
	}
}

func TestDelete_nonBuiltin(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.Register(ctx, "custom/", "transit", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "custom/"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "custom/"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDelete_builtin(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.RegisterBuiltin(ctx, "secret/", "kv", ""); err != nil {
		t.Fatal(err)
	}
	err := s.Delete(ctx, "secret/")
	if !errors.Is(err, ErrBuiltin) {
		t.Fatalf("delete builtin err = %v, want ErrBuiltin", err)
	}
}

func TestDelete_notFound(t *testing.T) {
	s := newTestStore(t)
	err := s.Delete(context.Background(), "ghost/")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_ = s.RegisterBuiltin(ctx, "secret/", "kv", "")
	_ = s.RegisterBuiltin(ctx, "pki/", "pki", "")
	_, _ = s.Register(ctx, "custom/", "transit", "")

	entries, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("list count = %d, want 3", len(entries))
	}
}

func TestNormalizePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"secret", "secret/"},
		{"secret/", "secret/"},
		{"/secret/", "secret/"},
		{"", "/"},
	}
	for _, tc := range cases {
		if got := normalizePath(tc.in); got != tc.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuiltinsCount(t *testing.T) {
	builtins := Builtins()
	if len(builtins) == 0 {
		t.Fatal("Builtins must not be empty")
	}
	seen := map[string]bool{}
	for _, b := range builtins {
		if seen[b.Path] {
			t.Errorf("duplicate builtin path: %q", b.Path)
		}
		seen[b.Path] = true
		if b.Type == "" {
			t.Errorf("builtin at %q has empty type", b.Path)
		}
	}
}
