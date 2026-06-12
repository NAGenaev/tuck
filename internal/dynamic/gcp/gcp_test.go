package gcp_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/dynamic/gcp"
	"github.com/NAGenaev/tuck/internal/physical"
)

// --- fake admin client ---

type fakeAdminClient struct {
	mu   sync.Mutex
	keys map[string]string // gcpKeyName -> fake JSON
}

func newFakeAdmin() *fakeAdminClient {
	return &fakeAdminClient{keys: make(map[string]string)}
}

func (f *fakeAdminClient) CreateKey(_ context.Context, serviceAccount, _ string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := "projects/proj/serviceAccounts/" + serviceAccount + "/keys/key-" + serviceAccount
	json := `{"type":"service_account","client_email":"` + serviceAccount + `"}`
	f.keys[name] = json
	return name, json, nil
}

func (f *fakeAdminClient) DeleteKey(_ context.Context, gcpKeyName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.keys, gcpKeyName)
	return nil
}

func (f *fakeAdminClient) keyCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.keys)
}

// --- fake token client ---

type fakeTokenClient struct{}

func (f *fakeTokenClient) GenerateAccessToken(_ context.Context, serviceAccount string, _ []string, lifetime time.Duration) (string, time.Time, error) {
	token := "ya29.fake-token-for-" + serviceAccount
	return token, time.Now().Add(lifetime), nil
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

func makeEngine(t *testing.T) (*gcp.Engine, *fakeAdminClient, *fakeTokenClient) {
	t.Helper()
	adminFake := newFakeAdmin()
	tokenFake := &fakeTokenClient{}
	e := gcp.New(newMemBarrier(),
		gcp.WithAdminClient(func(_ *gcp.Config) (gcp.GCPAdminClient, error) { return adminFake, nil }),
		gcp.WithTokenClient(func(_ *gcp.Config) (gcp.GCPTokenClient, error) { return tokenFake, nil }),
	)
	_ = e.PutConfig(context.Background(), &gcp.Config{})
	return e, adminFake, tokenFake
}

// --- tests ---

func TestConfigRoundtrip(t *testing.T) {
	e, _, _ := makeEngine(t)
	ctx := context.Background()

	if _, err := e.GetConfig(ctx); err != nil {
		t.Fatal(err)
	}

	if err := e.DeleteConfig(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := e.GetConfig(ctx)
	if !errors.Is(err, gcp.ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestRoleRoundtrip(t *testing.T) {
	e, _, _ := makeEngine(t)
	ctx := context.Background()

	role := &gcp.Role{
		Name:                "sa-key",
		CredentialType:      gcp.CredTypeServiceAccountKey,
		ServiceAccountEmail: "svc@project.iam.gserviceaccount.com",
	}
	if err := e.PutRole(ctx, role); err != nil {
		t.Fatal(err)
	}
	got, err := e.GetRole(ctx, "sa-key")
	if err != nil {
		t.Fatal(err)
	}
	if got.ServiceAccountEmail != role.ServiceAccountEmail {
		t.Fatalf("unexpected email: %s", got.ServiceAccountEmail)
	}

	names, err := e.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "sa-key" {
		t.Fatalf("want [sa-key], got %v", names)
	}

	if err := e.DeleteRole(ctx, "sa-key"); err != nil {
		t.Fatal(err)
	}
	_, err = e.GetRole(ctx, "sa-key")
	if !errors.Is(err, gcp.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGenerateServiceAccountKey(t *testing.T) {
	e, adminFake, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &gcp.Role{
		Name:                "deploy",
		CredentialType:      gcp.CredTypeServiceAccountKey,
		ServiceAccountEmail: "deploy@project.iam.gserviceaccount.com",
		DefaultTTL:          1 * time.Hour,
	})
	res, err := e.GenerateCreds(ctx, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if res.PrivateKey == "" {
		t.Fatal("want private_key")
	}
	if res.AccessToken != "" {
		t.Fatal("service_account_key must not have access_token")
	}
	if res.LeaseID == "" {
		t.Fatal("want lease_id")
	}
	if adminFake.keyCount() != 1 {
		t.Fatalf("want 1 GCP key, got %d", adminFake.keyCount())
	}

	lease, err := e.GetLease(ctx, res.LeaseID)
	if err != nil {
		t.Fatal(err)
	}
	if lease.CredentialType != gcp.CredTypeServiceAccountKey {
		t.Fatalf("bad credential_type: %s", lease.CredentialType)
	}
}

func TestGenerateAccessToken(t *testing.T) {
	e, _, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &gcp.Role{
		Name:                "viewer",
		CredentialType:      gcp.CredTypeAccessToken,
		ServiceAccountEmail: "viewer@project.iam.gserviceaccount.com",
		Scopes:              []string{"https://www.googleapis.com/auth/bigquery.readonly"},
		DefaultTTL:          30 * time.Minute,
	})
	res, err := e.GenerateCreds(ctx, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	if res.AccessToken == "" {
		t.Fatal("want access_token")
	}
	if res.TokenType != "Bearer" {
		t.Fatalf("want Bearer, got %s", res.TokenType)
	}
	if res.PrivateKey != "" {
		t.Fatal("access_token must not have private_key")
	}
}

func TestRevokeServiceAccountKey(t *testing.T) {
	e, adminFake, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &gcp.Role{
		Name:                "deploy",
		CredentialType:      gcp.CredTypeServiceAccountKey,
		ServiceAccountEmail: "deploy@project.iam.gserviceaccount.com",
	})
	res, err := e.GenerateCreds(ctx, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if adminFake.keyCount() != 1 {
		t.Fatal("want 1 GCP key after generate")
	}

	if err := e.RevokeLease(ctx, res.LeaseID); err != nil {
		t.Fatal(err)
	}
	if adminFake.keyCount() != 0 {
		t.Fatalf("want 0 GCP keys after revoke, got %d", adminFake.keyCount())
	}

	// Idempotent revoke.
	if err := e.RevokeLease(ctx, res.LeaseID); err != nil {
		t.Fatal(err)
	}
}

func TestRevokeExpired(t *testing.T) {
	e, adminFake, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &gcp.Role{
		Name:                "deploy",
		CredentialType:      gcp.CredTypeServiceAccountKey,
		ServiceAccountEmail: "deploy@project.iam.gserviceaccount.com",
		DefaultTTL:          -1 * time.Second, // already expired
	})
	res, err := e.GenerateCreds(ctx, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if adminFake.keyCount() != 1 {
		t.Fatal("want 1 GCP key")
	}

	if err := e.RevokeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if adminFake.keyCount() != 0 {
		t.Fatalf("want 0 GCP keys after RevokeExpired, got %d", adminFake.keyCount())
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
	e := gcp.New(newMemBarrier(),
		gcp.WithAdminClient(func(_ *gcp.Config) (gcp.GCPAdminClient, error) { return nil, nil }),
		gcp.WithTokenClient(func(_ *gcp.Config) (gcp.GCPTokenClient, error) { return nil, nil }),
	)
	ctx := context.Background()
	_ = e.PutRole(ctx, &gcp.Role{
		Name:                "r",
		CredentialType:      gcp.CredTypeServiceAccountKey,
		ServiceAccountEmail: "svc@project.iam.gserviceaccount.com",
	})
	_, err := e.GenerateCreds(ctx, "r")
	if !errors.Is(err, gcp.ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestListLeases(t *testing.T) {
	e, _, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &gcp.Role{
		Name:                "viewer",
		CredentialType:      gcp.CredTypeAccessToken,
		ServiceAccountEmail: "viewer@project.iam.gserviceaccount.com",
	})
	for i := 0; i < 4; i++ {
		if _, err := e.GenerateCreds(ctx, "viewer"); err != nil {
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
