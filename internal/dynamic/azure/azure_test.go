package azure_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/dynamic/azure"
	"github.com/NAGenaev/tuck/internal/physical"
)

// --- fake Graph client ---

type fakeGraphClient struct {
	mu   sync.Mutex
	keys map[string]string // keyID -> secretText
}

func newFakeGraph() *fakeGraphClient {
	return &fakeGraphClient{keys: make(map[string]string)}
}

func (f *fakeGraphClient) AddPassword(_ context.Context, appObjectID, displayName string, expiresAt time.Time) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	keyID := "key-" + appObjectID + "-" + displayName
	secret := "secret-value-for-" + appObjectID
	f.keys[keyID] = secret
	return keyID, secret, nil
}

func (f *fakeGraphClient) RemovePassword(_ context.Context, _ string, keyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.keys, keyID)
	return nil
}

func (f *fakeGraphClient) keyCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.keys)
}

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

// --- test helpers ---

func makeEngine(t *testing.T) (*azure.Engine, *fakeGraphClient) {
	t.Helper()
	fake := newFakeGraph()
	e := azure.New(newMemBarrier(),
		azure.WithGraphClient(func(_ *azure.Config) (azure.AzureGraphClient, error) { return fake, nil }),
	)
	_ = e.PutConfig(context.Background(), &azure.Config{TenantID: "tenant-abc"})
	return e, fake
}

func makeRole(objectID, appID string) *azure.Role {
	return &azure.Role{
		Name:                "webrole",
		ApplicationObjectID: objectID,
		ApplicationID:       appID,
		DefaultTTL:          1 * time.Hour,
	}
}

// --- tests ---

func TestConfigRoundtrip(t *testing.T) {
	e, _ := makeEngine(t)
	ctx := context.Background()

	cfg, err := e.GetConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TenantID != "tenant-abc" {
		t.Fatalf("want tenant-abc, got %s", cfg.TenantID)
	}

	if err := e.DeleteConfig(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = e.GetConfig(ctx)
	if !errors.Is(err, azure.ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestRoleRoundtrip(t *testing.T) {
	e, _ := makeEngine(t)
	ctx := context.Background()

	role := makeRole("obj-id-123", "app-id-456")
	if err := e.PutRole(ctx, role); err != nil {
		t.Fatal(err)
	}
	got, err := e.GetRole(ctx, "webrole")
	if err != nil {
		t.Fatal(err)
	}
	if got.ApplicationObjectID != role.ApplicationObjectID {
		t.Fatalf("unexpected object_id: %s", got.ApplicationObjectID)
	}

	names, err := e.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "webrole" {
		t.Fatalf("want [webrole], got %v", names)
	}

	if err := e.DeleteRole(ctx, "webrole"); err != nil {
		t.Fatal(err)
	}
	_, err = e.GetRole(ctx, "webrole")
	if !errors.Is(err, azure.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGenerateClientSecret(t *testing.T) {
	e, fake := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, makeRole("obj-123", "app-456"))

	res, err := e.GenerateCreds(ctx, "webrole")
	if err != nil {
		t.Fatal(err)
	}
	if res.ClientSecret == "" {
		t.Fatal("want client_secret")
	}
	if res.ClientID != "app-456" {
		t.Fatalf("want app-456, got %s", res.ClientID)
	}
	if res.TenantID != "tenant-abc" {
		t.Fatalf("want tenant-abc, got %s", res.TenantID)
	}
	if res.LeaseID == "" {
		t.Fatal("want lease_id")
	}
	if fake.keyCount() != 1 {
		t.Fatalf("want 1 key in fake, got %d", fake.keyCount())
	}

	lease, err := e.GetLease(ctx, res.LeaseID)
	if err != nil {
		t.Fatal(err)
	}
	if lease.ApplicationObjectID != "obj-123" {
		t.Fatalf("unexpected object_id in lease: %s", lease.ApplicationObjectID)
	}
}

func TestRevokeClientSecret(t *testing.T) {
	e, fake := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, makeRole("obj-123", "app-456"))
	res, err := e.GenerateCreds(ctx, "webrole")
	if err != nil {
		t.Fatal(err)
	}
	if fake.keyCount() != 1 {
		t.Fatal("want 1 key after generate")
	}

	if err := e.RevokeLease(ctx, res.LeaseID); err != nil {
		t.Fatal(err)
	}
	if fake.keyCount() != 0 {
		t.Fatalf("want 0 keys after revoke, got %d", fake.keyCount())
	}

	// Idempotent.
	if err := e.RevokeLease(ctx, res.LeaseID); err != nil {
		t.Fatal(err)
	}
}

func TestRevokeExpired(t *testing.T) {
	e, fake := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &azure.Role{
		Name:                "webrole",
		ApplicationObjectID: "obj-123",
		ApplicationID:       "app-456",
		DefaultTTL:          -1 * time.Second, // already expired
	})
	res, err := e.GenerateCreds(ctx, "webrole")
	if err != nil {
		t.Fatal(err)
	}
	if fake.keyCount() != 1 {
		t.Fatal("want 1 key")
	}

	if err := e.RevokeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.keyCount() != 0 {
		t.Fatalf("want 0 keys after RevokeExpired, got %d", fake.keyCount())
	}

	lease, err := e.GetLease(ctx, res.LeaseID)
	if err != nil {
		t.Fatal(err)
	}
	if !lease.Revoked {
		t.Fatal("want lease revoked")
	}
}

func TestNoConfig(t *testing.T) {
	e := azure.New(newMemBarrier(),
		azure.WithGraphClient(func(_ *azure.Config) (azure.AzureGraphClient, error) { return nil, nil }),
	)
	ctx := context.Background()
	_ = e.PutRole(ctx, makeRole("obj-123", "app-456"))
	_, err := e.GenerateCreds(ctx, "webrole")
	if !errors.Is(err, azure.ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestListLeases(t *testing.T) {
	e, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, makeRole("obj-123", "app-456"))
	for i := 0; i < 4; i++ {
		if _, err := e.GenerateCreds(ctx, "webrole"); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := e.ListLeases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 4 {
		t.Fatalf("want 4 leases, got %d", len(ids))
	}
}

func TestMissingFields(t *testing.T) {
	e, _ := makeEngine(t)
	ctx := context.Background()

	// Role without application_object_id
	_ = e.PutRole(ctx, &azure.Role{Name: "bad", ApplicationID: "app-456"})
	_, err := e.GenerateCreds(ctx, "bad")
	if err == nil {
		t.Fatal("want error for missing application_object_id")
	}

	// Role without application_id
	_ = e.PutRole(ctx, &azure.Role{Name: "bad2", ApplicationObjectID: "obj-123"})
	_, err = e.GenerateCreds(ctx, "bad2")
	if err == nil {
		t.Fatal("want error for missing application_id")
	}
}
