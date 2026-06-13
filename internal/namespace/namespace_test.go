package namespace_test

import (
	"context"
	"testing"

	"github.com/NAGenaev/tuck/internal/namespace"
	"github.com/NAGenaev/tuck/internal/physical"
)

func newStore(t *testing.T) *namespace.Store {
	t.Helper()
	return namespace.NewStore(physical.NewInMem())
}

func TestValidateName(t *testing.T) {
	valid := []string{"dev", "prod", "team-a", "ns1", "a1b2"}
	for _, n := range valid {
		if err := namespace.ValidateName(n); err != nil {
			t.Errorf("expected %q to be valid: %v", n, err)
		}
	}
	invalid := []string{"root", "", "Root", "DEV", "ns/foo", "ns_foo", "-bad"}
	for _, n := range invalid {
		if err := namespace.ValidateName(n); err == nil {
			t.Errorf("expected %q to be invalid", n)
		}
	}
}

func TestStoragePrefix(t *testing.T) {
	cases := []struct{ ns, want string }{
		{"", ""},
		{"root", ""},
		{"dev", "ns/dev/"},
		{"prod", "ns/prod/"},
	}
	for _, tc := range cases {
		got := namespace.StoragePrefix(tc.ns)
		if got != tc.want {
			t.Errorf("StoragePrefix(%q) = %q, want %q", tc.ns, got, tc.want)
		}
	}
}

func TestCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	// Create
	if err := s.Put(ctx, &namespace.Namespace{Name: "dev"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get
	got, err := s.Get(ctx, "dev")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "dev" {
		t.Errorf("got name %q, want %q", got.Name, "dev")
	}

	// List
	if err := s.Put(ctx, &namespace.Namespace{Name: "prod"}); err != nil {
		t.Fatalf("Put prod: %v", err)
	}
	names, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("List returned %d names, want 2: %v", len(names), names)
	}

	// Delete
	if err := s.Delete(ctx, "dev"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Get(ctx, "dev")
	if err == nil {
		t.Error("expected ErrNotFound after Delete")
	}
}
