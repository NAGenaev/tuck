package lease

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeBackend is a simple in-memory Backend for tests.
type fakeBackend struct {
	leases map[string]*fakeEntry
}

type fakeEntry struct {
	expiresAt time.Time
	revoked   bool
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{leases: map[string]*fakeEntry{}}
}

func (f *fakeBackend) add(id string, ttl time.Duration) {
	f.leases[id] = &fakeEntry{expiresAt: time.Now().Add(ttl)}
}

func (f *fakeBackend) GetLeaseInfo(_ context.Context, id string) (time.Time, bool, error) {
	e, ok := f.leases[id]
	if !ok {
		return time.Time{}, false, ErrNotFound
	}
	return e.expiresAt, e.revoked, nil
}

func (f *fakeBackend) RevokeLease(_ context.Context, id string) error {
	e, ok := f.leases[id]
	if !ok {
		return ErrNotFound
	}
	e.revoked = true
	return nil
}

func (f *fakeBackend) ListLeases(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(f.leases))
	for id := range f.leases {
		out = append(out, id)
	}
	return out, nil
}

func newTestManager() (*Manager, *fakeBackend, *fakeBackend) {
	db := newFakeBackend()
	aws := newFakeBackend()
	m := New(map[string]Backend{
		"database": db,
		"aws":      aws,
	})
	return m, db, aws
}

func TestLookup_found(t *testing.T) {
	m, db, _ := newTestManager()
	db.add("abc123", time.Hour)

	info, err := m.Lookup(context.Background(), "database/abc123")
	if err != nil {
		t.Fatal(err)
	}
	if info.Backend != "database" {
		t.Errorf("backend = %q, want database", info.Backend)
	}
	if info.InternalID != "abc123" {
		t.Errorf("internal_id = %q, want abc123", info.InternalID)
	}
	if info.ID != "database/abc123" {
		t.Errorf("id = %q, want database/abc123", info.ID)
	}
	if info.Revoked {
		t.Error("expected not revoked")
	}
}

func TestLookup_notFound(t *testing.T) {
	m, _, _ := newTestManager()
	_, err := m.Lookup(context.Background(), "database/nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLookup_unknownBackend(t *testing.T) {
	m, _, _ := newTestManager()
	_, err := m.Lookup(context.Background(), "gcp/abc")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestLookup_invalidID(t *testing.T) {
	m, _, _ := newTestManager()
	_, err := m.Lookup(context.Background(), "noslash")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRevoke(t *testing.T) {
	m, db, _ := newTestManager()
	db.add("xyz", time.Hour)

	if err := m.Revoke(context.Background(), "database/xyz"); err != nil {
		t.Fatal(err)
	}
	info, _ := m.Lookup(context.Background(), "database/xyz")
	if !info.Revoked {
		t.Error("expected revoked after Revoke call")
	}
}

func TestRevoke_notFound(t *testing.T) {
	m, _, _ := newTestManager()
	err := m.Revoke(context.Background(), "database/ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestList_aggregates(t *testing.T) {
	m, db, aws := newTestManager()
	db.add("d1", time.Hour)
	db.add("d2", time.Hour)
	aws.add("a1", 30*time.Minute)

	leases, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 3 {
		t.Fatalf("got %d leases, want 3", len(leases))
	}
	backends := map[string]int{}
	for _, l := range leases {
		backends[l.Backend]++
	}
	if backends["database"] != 2 {
		t.Errorf("database leases = %d, want 2", backends["database"])
	}
	if backends["aws"] != 1 {
		t.Errorf("aws leases = %d, want 1", backends["aws"])
	}
}

func TestSplitID(t *testing.T) {
	cases := []struct{ id, backend, internal string; wantErr bool }{
		{"database/abc", "database", "abc", false},
		{"aws/x/y", "aws", "x/y", false}, // internal IDs may contain slashes
		{"noslash", "", "", true},
		{"/noid", "", "", true},
		{"backend/", "", "", true},
	}
	for _, tc := range cases {
		b, i, err := splitID(tc.id)
		if tc.wantErr {
			if err == nil {
				t.Errorf("splitID(%q): expected error", tc.id)
			}
		} else {
			if err != nil {
				t.Errorf("splitID(%q): unexpected error: %v", tc.id, err)
			}
			if b != tc.backend || i != tc.internal {
				t.Errorf("splitID(%q) = (%q,%q), want (%q,%q)", tc.id, b, i, tc.backend, tc.internal)
			}
		}
	}
}
