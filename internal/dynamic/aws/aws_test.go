package aws_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	dynaws "github.com/NAGenaev/tuck/internal/dynamic/aws"
	"github.com/NAGenaev/tuck/internal/physical"
)

// --- fake IAM client ---

type fakeIAMClient struct {
	mu    sync.Mutex
	users map[string]bool
	keys  map[string]string // keyID -> secret
}

func newFakeIAM() *fakeIAMClient {
	return &fakeIAMClient{users: make(map[string]bool), keys: make(map[string]string)}
}

func (f *fakeIAMClient) CreateUser(_ context.Context, username string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[username] = true
	return nil
}

func (f *fakeIAMClient) PutUserPolicy(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeIAMClient) AttachUserPolicy(_ context.Context, _, _ string) error { return nil }
func (f *fakeIAMClient) DetachUserPolicy(_ context.Context, _, _ string) error { return nil }
func (f *fakeIAMClient) DeleteUserPolicy(_ context.Context, _, _ string) error { return nil }

func (f *fakeIAMClient) CreateAccessKey(_ context.Context, username string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := len(username)
	if n > 10 {
		n = 10
	}
	keyID := "AKIA" + username[:n]
	secret := "secret-" + username
	f.keys[keyID] = secret
	return keyID, secret, nil
}

func (f *fakeIAMClient) DeleteAccessKey(_ context.Context, _, keyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.keys, keyID)
	return nil
}

func (f *fakeIAMClient) DeleteUser(_ context.Context, username string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.users, username)
	return nil
}

func (f *fakeIAMClient) userCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.users)
}

// --- fake STS client ---

type fakeSTSClient struct{}

func (f *fakeSTSClient) AssumeRole(_ context.Context, _, sessionName string, duration time.Duration, _ string) (string, string, string, time.Time, error) {
	n := len(sessionName)
	if n > 8 {
		n = 8
	}
	expiry := time.Now().Add(duration)
	return "ASIA" + sessionName[:n], "sts-secret", "session-token", expiry, nil
}

// --- in-memory barrier matching physical.Entry API ---

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

func makeEngine(t *testing.T) (*dynaws.Engine, *fakeIAMClient, *fakeSTSClient) {
	t.Helper()
	iamFake := newFakeIAM()
	stsFake := &fakeSTSClient{}
	e := dynaws.New(newMemBarrier(),
		dynaws.WithIAMClient(func(_ *dynaws.Config) (dynaws.IAMClient, error) { return iamFake, nil }),
		dynaws.WithSTSClient(func(_ *dynaws.Config) (dynaws.STSClient, error) { return stsFake, nil }),
	)
	_ = e.PutConfig(context.Background(), &dynaws.Config{Region: "us-east-1"})
	return e, iamFake, stsFake
}

// --- tests ---

func TestConfigRoundtrip(t *testing.T) {
	e, _, _ := makeEngine(t)
	ctx := context.Background()

	cfg, err := e.GetConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Region != "us-east-1" {
		t.Fatalf("want us-east-1, got %s", cfg.Region)
	}

	if err := e.DeleteConfig(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = e.GetConfig(ctx)
	if !errors.Is(err, dynaws.ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestRoleRoundtrip(t *testing.T) {
	e, _, _ := makeEngine(t)
	ctx := context.Background()

	role := &dynaws.Role{
		Name:           "test",
		CredentialType: dynaws.CredTypeAssumedRole,
		RoleARNs:       []string{"arn:aws:iam::123456789012:role/test"},
	}
	if err := e.PutRole(ctx, role); err != nil {
		t.Fatal(err)
	}
	got, err := e.GetRole(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != role.Name || len(got.RoleARNs) != 1 {
		t.Fatalf("unexpected role: %+v", got)
	}

	names, err := e.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "test" {
		t.Fatalf("want [test], got %v", names)
	}

	if err := e.DeleteRole(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	_, err = e.GetRole(ctx, "test")
	if !errors.Is(err, dynaws.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGenerateCredsAssumedRole(t *testing.T) {
	e, _, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &dynaws.Role{
		Name:           "app",
		CredentialType: dynaws.CredTypeAssumedRole,
		RoleARNs:       []string{"arn:aws:iam::123:role/app"},
		DefaultTTL:     1 * time.Hour,
	})
	res, err := e.GenerateCreds(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionToken == "" {
		t.Fatal("want session_token")
	}
	if res.LeaseID == "" {
		t.Fatal("want lease_id")
	}
	if res.ExpiresAt.IsZero() {
		t.Fatal("want expires_at")
	}

	lease, err := e.GetLease(ctx, res.LeaseID)
	if err != nil {
		t.Fatal(err)
	}
	if lease.CredentialType != dynaws.CredTypeAssumedRole {
		t.Fatalf("bad credential_type: %s", lease.CredentialType)
	}
}

func TestGenerateCredsIAMUser(t *testing.T) {
	e, iamFake, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &dynaws.Role{
		Name:           "svc",
		CredentialType: dynaws.CredTypeIAMUser,
		PolicyARNs:     []string{"arn:aws:iam::aws:policy/ReadOnlyAccess"},
	})
	res, err := e.GenerateCreds(ctx, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionToken != "" {
		t.Fatal("iam_user must not have session_token")
	}
	if res.SecretAccessKey == "" {
		t.Fatal("want secret_access_key")
	}
	if iamFake.userCount() != 1 {
		t.Fatalf("want 1 IAM user, got %d", iamFake.userCount())
	}
}

func TestRevokeIAMUserLease(t *testing.T) {
	e, iamFake, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &dynaws.Role{
		Name:           "svc",
		CredentialType: dynaws.CredTypeIAMUser,
	})
	res, err := e.GenerateCreds(ctx, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if iamFake.userCount() != 1 {
		t.Fatal("want 1 IAM user after generate")
	}

	if err := e.RevokeLease(ctx, res.LeaseID); err != nil {
		t.Fatal(err)
	}
	if iamFake.userCount() != 0 {
		t.Fatalf("want 0 IAM users after revoke, got %d", iamFake.userCount())
	}

	// Idempotent second revoke.
	if err := e.RevokeLease(ctx, res.LeaseID); err != nil {
		t.Fatal(err)
	}
}

func TestRevokeExpired(t *testing.T) {
	e, _, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &dynaws.Role{
		Name:           "app",
		CredentialType: dynaws.CredTypeAssumedRole,
		RoleARNs:       []string{"arn:aws:iam::123:role/app"},
		DefaultTTL:     -1 * time.Second, // already expired
	})
	res, err := e.GenerateCreds(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}

	if err := e.RevokeExpired(ctx); err != nil {
		t.Fatal(err)
	}

	lease, err := e.GetLease(ctx, res.LeaseID)
	if err != nil {
		t.Fatal(err)
	}
	if !lease.Revoked {
		t.Fatal("want lease revoked after RevokeExpired")
	}
}

func TestListLeases(t *testing.T) {
	e, _, _ := makeEngine(t)
	ctx := context.Background()

	_ = e.PutRole(ctx, &dynaws.Role{
		Name:           "app",
		CredentialType: dynaws.CredTypeAssumedRole,
		RoleARNs:       []string{"arn:aws:iam::123:role/app"},
	})
	for i := 0; i < 3; i++ {
		if _, err := e.GenerateCreds(ctx, "app"); err != nil {
			t.Fatal(err)
		}
	}

	ids, err := e.ListLeases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 {
		t.Fatalf("want 3 leases, got %d", len(ids))
	}
}

func TestNoConfig(t *testing.T) {
	e := dynaws.New(newMemBarrier(),
		dynaws.WithIAMClient(func(_ *dynaws.Config) (dynaws.IAMClient, error) { return nil, nil }),
		dynaws.WithSTSClient(func(_ *dynaws.Config) (dynaws.STSClient, error) { return nil, nil }),
	)
	ctx := context.Background()
	_ = e.PutRole(ctx, &dynaws.Role{
		Name:           "r",
		CredentialType: dynaws.CredTypeAssumedRole,
		RoleARNs:       []string{"arn:aws:iam::1:role/r"},
	})
	_, err := e.GenerateCreds(ctx, "r")
	if !errors.Is(err, dynaws.ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}
