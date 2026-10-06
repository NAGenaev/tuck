package namespace_test

import (
	"context"
	"errors"
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

func TestMetadataValidate(t *testing.T) {
	ok := namespace.Metadata{Description: "line1\nline2", Owner: "platform team", Labels: map[string]string{"env": "prod"}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid metadata rejected: %v", err)
	}
	long := make([]byte, namespace.MaxDescription+1)
	for i := range long {
		long[i] = 'a'
	}
	many := map[string]string{}
	for i := 0; i <= namespace.MaxLabels; i++ {
		many[string(rune('a'+i%26))+string(rune('a'+i/26))] = "v"
	}
	bad := []namespace.Metadata{
		{Description: string(long)},
		{Owner: "a\nb"},
		{Description: "ctl\x00"},
		{Labels: map[string]string{"": "v"}},
		{Labels: map[string]string{"k": "a\tb"}},
		{Labels: many},
	}
	for i, m := range bad {
		if err := m.Validate(); !errors.Is(err, namespace.ErrInvalidMetadata) {
			t.Errorf("case %d: expected ErrInvalidMetadata, got %v", i, err)
		}
	}
}

func TestMetadataRoundTripAndLegacy(t *testing.T) {
	ctx := context.Background()
	mem := physical.NewInMem()
	s := namespace.NewStore(mem)

	// A record written before metadata existed still loads.
	if err := mem.Put(ctx, &physical.Entry{Key: "sys/namespaces/old", Value: []byte(`{"name":"old","created_at":"2024-01-01T00:00:00Z"}`)}); err != nil {
		t.Fatal(err)
	}
	old, err := s.Get(ctx, "old")
	if err != nil || old.Description != "" || old.Labels != nil {
		t.Fatalf("legacy load: %+v, %v", old, err)
	}

	old.Apply(namespace.Metadata{Description: "d", Owner: "o", Labels: map[string]string{"a": "b"}})
	if err := s.Put(ctx, old); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "old")
	if err != nil || got.Description != "d" || got.Owner != "o" || got.Labels["a"] != "b" {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	got.Apply(namespace.Metadata{})
	if got.Description != "" || got.Labels != nil {
		t.Fatalf("clear: %+v", got)
	}
}
