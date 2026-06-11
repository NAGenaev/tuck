package approle

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

// --- in-memory barrier for tests ---

type memBarrier struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMem() *memBarrier { return &memBarrier{data: make(map[string][]byte)} }

func (m *memBarrier) Get(_ context.Context, key string) (*physical.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	c := make([]byte, len(v))
	copy(c, v)
	return &physical.Entry{Key: key, Value: c}, nil
}
func (m *memBarrier) Put(_ context.Context, e *physical.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := make([]byte, len(e.Value))
	copy(c, e.Value)
	m.data[e.Key] = c
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
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, k)
		}
	}
	return out, nil
}

func TestCreateAndLoginRole(t *testing.T) {
	s := NewStore(newMem())
	ctx := context.Background()

	role := &Role{
		Name:     "ci",
		Policies: []string{"ci-reader"},
		TokenTTL: 15 * time.Minute,
	}
	if err := s.PutRole(ctx, role); err != nil {
		t.Fatalf("PutRole: %v", err)
	}
	if role.RoleID == "" {
		t.Fatal("expected RoleID to be auto-generated")
	}

	sid, err := s.GenerateSecretID(ctx, "ci")
	if err != nil {
		t.Fatalf("GenerateSecretID: %v", err)
	}

	result, err := s.Login(ctx, role.RoleID, sid.ID)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.Policies[0] != "ci-reader" {
		t.Fatalf("wrong policies: %v", result.Policies)
	}
	if result.TTL != 15*time.Minute {
		t.Fatalf("wrong TTL: %v", result.TTL)
	}
}

func TestLoginWrongRoleID(t *testing.T) {
	s := NewStore(newMem())
	ctx := context.Background()

	r := &Role{Name: "r", Policies: []string{"p"}}
	s.PutRole(ctx, r)
	sid, _ := s.GenerateSecretID(ctx, "r")

	_, err := s.Login(ctx, "wrong-role-id", sid.ID)
	if err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginWrongSecretID(t *testing.T) {
	s := NewStore(newMem())
	ctx := context.Background()

	r := &Role{Name: "r", Policies: []string{"p"}}
	s.PutRole(ctx, r)

	_, err := s.Login(ctx, r.RoleID, "nonexistent")
	if err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestSecretIDNumUses(t *testing.T) {
	s := NewStore(newMem())
	ctx := context.Background()

	r := &Role{Name: "r", Policies: []string{"p"}, SecretIDNumUses: 2}
	s.PutRole(ctx, r)
	sid, _ := s.GenerateSecretID(ctx, "r")

	if _, err := s.Login(ctx, r.RoleID, sid.ID); err != nil {
		t.Fatalf("first login: %v", err)
	}
	if _, err := s.Login(ctx, r.RoleID, sid.ID); err != nil {
		t.Fatalf("second login: %v", err)
	}
	// Third login must fail — secret-id exhausted.
	if _, err := s.Login(ctx, r.RoleID, sid.ID); err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials after exhausted uses, got %v", err)
	}
}

func TestSecretIDTTL(t *testing.T) {
	s := NewStore(newMem())
	ctx := context.Background()

	r := &Role{Name: "r", Policies: []string{"p"}, SecretIDTTL: 1}
	s.PutRole(ctx, r)
	sid, _ := s.GenerateSecretID(ctx, "r")

	// Manually expire the SecretID by back-dating ExpiresAt.
	sid.ExpiresAt = time.Now().Add(-time.Second)
	s.put(ctx, secretIDsPrefix+sid.ID, sid)

	_, err := s.Login(ctx, r.RoleID, sid.ID)
	if err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for expired secret-id, got %v", err)
	}
}

func TestListAndDeleteRole(t *testing.T) {
	s := NewStore(newMem())
	ctx := context.Background()

	s.PutRole(ctx, &Role{Name: "a", Policies: []string{"p"}})
	s.PutRole(ctx, &Role{Name: "b", Policies: []string{"p"}})

	names, err := s.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(names))
	}

	s.DeleteRole(ctx, "a")
	names, _ = s.ListRoles(ctx)
	if len(names) != 1 {
		t.Fatalf("expected 1 role after delete, got %d", len(names))
	}
}

func TestDestroySecretID(t *testing.T) {
	s := NewStore(newMem())
	ctx := context.Background()

	r := &Role{Name: "r", Policies: []string{"p"}}
	s.PutRole(ctx, r)
	sid, _ := s.GenerateSecretID(ctx, "r")

	if err := s.DestroySecretID(ctx, sid.ID); err != nil {
		t.Fatal(err)
	}
	_, err := s.Login(ctx, r.RoleID, sid.ID)
	if err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials after destroy, got %v", err)
	}
}
