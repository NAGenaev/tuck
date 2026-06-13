package lease

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeBackend is a simple in-memory Backend for tests.
type fakeBackend struct {
	leases    map[string]*fakeEntry
	createdAt map[string]time.Time
	maxTTL    time.Duration // zero = no cap
}

type fakeEntry struct {
	expiresAt time.Time
	revoked   bool
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{leases: map[string]*fakeEntry{}, createdAt: map[string]time.Time{}}
}

func (f *fakeBackend) add(id string, ttl time.Duration) {
	now := time.Now()
	f.leases[id] = &fakeEntry{expiresAt: now.Add(ttl)}
	f.createdAt[id] = now
}

func (f *fakeBackend) RenewLease(_ context.Context, id string, increment time.Duration) (time.Time, error) {
	e, ok := f.leases[id]
	if !ok {
		return time.Time{}, ErrNotFound
	}
	newExpiry := time.Now().Add(increment)
	if f.maxTTL > 0 {
		if cap := f.createdAt[id].Add(f.maxTTL); newExpiry.After(cap) {
			newExpiry = cap
		}
	}
	e.expiresAt = newExpiry
	return e.expiresAt, nil
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

// nonRenewableBackend implements Backend but NOT RenewableBackend.
type nonRenewableBackend struct{}

func (n *nonRenewableBackend) GetLeaseInfo(_ context.Context, id string) (time.Time, bool, error) {
	return time.Now().Add(time.Hour), false, nil
}
func (n *nonRenewableBackend) RevokeLease(_ context.Context, _ string) error { return nil }
func (n *nonRenewableBackend) ListLeases(_ context.Context) ([]string, error) { return nil, nil }

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

func TestRenew_basic(t *testing.T) {
	db := newFakeBackend()
	db.add("abc", time.Hour)
	m := New(map[string]Backend{"database": db})

	newExp, err := m.Renew(context.Background(), "database/abc", 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if newExp.Before(time.Now().Add(time.Hour)) {
		t.Errorf("renewed expiry %v is too soon", newExp)
	}
}

func TestRenew_notRenewable(t *testing.T) {
	m := New(map[string]Backend{"database": &nonRenewableBackend{}})
	_, err := m.Renew(context.Background(), "database/abc", time.Hour)
	if err == nil {
		t.Fatal("expected error for non-renewable backend")
	}
}

func TestRenew_withMaxTTLCap(t *testing.T) {
	db := newFakeBackend()
	db.maxTTL = 90 * time.Minute
	db.add("abc", time.Hour)
	m := New(map[string]Backend{"database": db})

	// Request 2h renewal; cap at 90m from createdAt so actual extension is <90m.
	newExp, err := m.Renew(context.Background(), "database/abc", 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if newExp.After(db.createdAt["abc"].Add(90 * time.Minute).Add(time.Second)) {
		t.Errorf("renewed expiry %v exceeds MaxTTL cap", newExp)
	}
}

func TestRenew_notFound(t *testing.T) {
	m, _, _ := newTestManager()
	_, err := m.Renew(context.Background(), "database/ghost", time.Hour)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
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
