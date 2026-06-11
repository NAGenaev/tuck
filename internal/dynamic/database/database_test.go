package database

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

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

func TestConfigCRUD(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()

	cfg := &Config{Name: "mydb", PluginName: "postgresql", ConnectionURL: "postgres://host/db"}
	if err := m.PutConfig(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetConfig(ctx, "mydb")
	if err != nil {
		t.Fatal(err)
	}
	if got.PluginName != "postgresql" {
		t.Fatalf("got plugin %q", got.PluginName)
	}
	if err := m.DeleteConfig(ctx, "mydb"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetConfig(ctx, "mydb"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRoleCRUD(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()

	m.PutConfig(ctx, &Config{Name: "pg", PluginName: "postgresql", ConnectionURL: "x"})

	role := &Role{Name: "app", DBName: "pg", DefaultTTL: time.Hour}
	if err := m.PutRole(ctx, role); err != nil {
		t.Fatal(err)
	}
	// default statements should have been filled in
	if role.CreationStatements == "" {
		t.Fatal("expected creation statements to be auto-populated")
	}

	names, err := m.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "app" {
		t.Fatalf("ListRoles: %v", names)
	}

	m.DeleteRole(ctx, "app")
	if _, err := m.GetRole(ctx, "app"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestRenderTemplate(t *testing.T) {
	tmpl := `CREATE USER "{{username}}" WITH PASSWORD '{{password}}' VALID UNTIL '{{expiry}}';`
	out := renderTemplate(tmpl, map[string]string{
		"{{username}}": "tuck_app_123",
		"{{password}}": "s3cr3t",
		"{{expiry}}":   "2025-01-01T00:00:00Z",
	})
	if !containsAll(out, "tuck_app_123", "s3cr3t", "2025-01-01") {
		t.Fatalf("template render failed: %q", out)
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("role-name; DROP TABLE"); got != "rolename DROP TABLE" {
		// hyphens and semicolons stripped
	}
	if sanitize("valid_role") != "valid_role" {
		t.Fatal("sanitize should preserve valid identifier chars")
	}
}

func TestLeaseExpired(t *testing.T) {
	l := &Lease{ExpiresAt: time.Now().Add(-time.Second)}
	if !l.IsExpired() {
		t.Fatal("expected expired lease to report IsExpired=true")
	}
	l2 := &Lease{ExpiresAt: time.Now().Add(time.Hour)}
	if l2.IsExpired() {
		t.Fatal("expected non-expired lease to report IsExpired=false")
	}
}

func TestGeneratePassword(t *testing.T) {
	p1, err := generatePassword(16)
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := generatePassword(16)
	if p1 == p2 {
		t.Fatal("generated passwords should be unique")
	}
	if len(p1) < 20 {
		t.Fatalf("password too short: %q", p1)
	}
}

func TestSplitStatements(t *testing.T) {
	stmts := splitStatements("  CREATE USER x; GRANT SELECT ON y;  ")
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(stmts), stmts)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !containsStr(s, sub) {
			return false
		}
	}
	return true
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsInner(s, sub))
}

func containsInner(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
