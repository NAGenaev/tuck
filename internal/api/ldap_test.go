package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	authlda "github.com/NAGenaev/tuck/internal/auth/ldap"
	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

// --- fake LDAP connection (mirrors the one in ldap_test package) ---

type fakeLDAPUser struct {
	dn       string
	password string
	groups   []string
}

type fakeLDAPConn struct {
	users      map[string]fakeLDAPUser
	serviceDN  string
	servicePS  string
}

func (f *fakeLDAPConn) Bind(dn, password string) error {
	if dn == f.serviceDN && password == f.servicePS {
		return nil
	}
	for _, u := range f.users {
		if u.dn == dn && u.password == password {
			return nil
		}
	}
	return errors.New("ldap: invalid credentials")
}

func (f *fakeLDAPConn) Search(_, filter string, _ []string) ([]*authlda.Entry, error) {
	var out []*authlda.Entry
	for username, u := range f.users {
		if strings.Contains(filter, username) || strings.Contains(filter, u.dn) {
			out = append(out, &authlda.Entry{
				DN: u.dn,
				Attributes: map[string][]string{
					"uid":      {username},
					"memberOf": u.groups,
				},
			})
		}
	}
	return out, nil
}

func (f *fakeLDAPConn) Close() error { return nil }

// newLDAPTestServer creates a test server with a fake LDAP dialer injected.
// The fake directory has two users:
//   - alice  password=alicepass  groups=[admin, devs]
//   - bob    password=bobpass    groups=[devs]
func newLDAPTestServer(t *testing.T) (*httptest.Server, *core.Core, string) {
	t.Helper()
	c := core.New(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	result, err := c.Start(context.Background())
	if err != nil {
		t.Fatalf("core start: %v", err)
	}

	users := map[string]fakeLDAPUser{
		"alice": {dn: "uid=alice,ou=users,dc=test", password: "alicepass", groups: []string{"CN=admin,DC=test", "CN=devs,DC=test"}},
		"bob":   {dn: "uid=bob,ou=users,dc=test", password: "bobpass", groups: []string{"CN=devs,DC=test"}},
	}
	conn := &fakeLDAPConn{
		users:     users,
		serviceDN: "cn=svc,dc=test",
		servicePS: "svcpass",
	}
	c.SetLDAPDialer(func(_ context.Context, _ authlda.Config) (authlda.Conn, error) {
		return conn, nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ts := httptest.NewUnstartedServer(New(c).Handler())
	ts.Listener = &rstListener{ln}
	ts.Start()
	t.Cleanup(ts.Close)
	return ts, c, result.RootToken.ID
}

// putLDAPConfig PUTs the LDAP config and fails the test if the request fails.
func putLDAPConfig(t *testing.T, ts *httptest.Server, root string) {
	t.Helper()
	body := mustJSON(map[string]any{
		"urls":          []string{"ldap://fake:389"},
		"bind_dn":       "cn=svc,dc=test",
		"bind_password": "svcpass",
		"user_dn":       "ou=users,dc=test",
	})
	status, resp := doJSON(t, ts, http.MethodPut, "/v1/auth/ldap/config", body, root)
	if status != http.StatusNoContent {
		t.Fatalf("PUT /v1/auth/ldap/config: %d %s", status, resp)
	}
}

// putLDAPRole creates a named LDAP role and fails the test if the request fails.
func putLDAPRole(t *testing.T, ts *httptest.Server, root, name string, groups, users, policies []string, ttl string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"groups":   groups,
		"users":    users,
		"policies": policies,
		"ttl":      ttl,
	})
	status, resp := doJSON(t, ts, http.MethodPut, "/v1/auth/ldap/role/"+name, string(body), root)
	if status != http.StatusNoContent {
		t.Fatalf("PUT /v1/auth/ldap/role/%s: %d %s", name, status, resp)
	}
}

// ldapLogin posts credentials to the LDAP login endpoint.
func ldapLogin(t *testing.T, ts *httptest.Server, username, password string) (int, []byte) {
	t.Helper()
	body := mustJSON(map[string]string{"username": username, "password": password})
	return doJSON(t, ts, http.MethodPost, "/v1/auth/ldap/login", body, "")
}

// --- tests ---

// TestLDAPFullFlow configures LDAP, creates a role, logs in, and uses the token.
func TestLDAPFullFlow(t *testing.T) {
	ts, _, root := newLDAPTestServer(t)

	putLDAPConfig(t, ts, root)

	// Policy for alice.
	createTestPolicy(t, ts, root, "ldap-policy",
		`[{"path":"secret/ldap/*","capabilities":["read","write"]}]`)

	// Role matching alice's group "admin".
	putLDAPRole(t, ts, root, "admin-role", []string{"admin"}, nil, []string{"ldap-policy"}, "1h")

	// Login as alice.
	status, body := ldapLogin(t, ts, "alice", "alicepass")
	if status != http.StatusOK {
		t.Fatalf("alice login: %d %s", status, body)
	}
	tok := jsonField(t, body, "token")
	if tok == "" {
		t.Fatalf("token empty, body: %s", body)
	}

	// Alice can write and read within her policy scope.
	status, _ = doJSON(t, ts, http.MethodPut, "/v1/secret/ldap/cred", `{"val":"pass"}`, tok)
	if status != http.StatusNoContent {
		t.Errorf("alice write: %d, want 204", status)
	}
	status, body = doJSON(t, ts, http.MethodGet, "/v1/secret/ldap/cred", "", tok)
	if status != http.StatusOK {
		t.Errorf("alice read: %d, want 200; body: %s", status, body)
	}
	if !strings.Contains(string(body), "pass") {
		t.Errorf("alice read body %s missing written value", body)
	}

	// Alice is denied outside her policy scope.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/secret/other/key", "", tok)
	if status != http.StatusForbidden {
		t.Errorf("alice out-of-scope: %d, want 403", status)
	}
}

// TestLDAPTokenRenewable verifies that LDAP-issued tokens are renewable.
func TestLDAPTokenRenewable(t *testing.T) {
	ts, _, root := newLDAPTestServer(t)

	putLDAPConfig(t, ts, root)
	putLDAPRole(t, ts, root, "devs-role", []string{"devs"}, nil, []string{"root"}, "500ms")

	// Bob is in group "devs".
	status, body := ldapLogin(t, ts, "bob", "bobpass")
	if status != http.StatusOK {
		t.Fatalf("bob login: %d %s", status, body)
	}
	bobTok := jsonField(t, body, "token")

	// lookup-self must report renewable=true.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/auth/token/lookup-self", "", bobTok)
	if status != http.StatusOK {
		t.Fatalf("lookup-self: %d %s", status, body)
	}
	if jsonField(t, body, "renewable") != "true" {
		t.Errorf("LDAP token renewable=%s, want true", jsonField(t, body, "renewable"))
	}

	// Renew while still valid.
	renewBody := mustJSON(map[string]string{"ttl": "10s"})
	status, body = doJSON(t, ts, http.MethodPost, "/v1/auth/token/renew-self", renewBody, bobTok)
	if status != http.StatusOK {
		t.Fatalf("renew-self: %d %s, want 200", status, body)
	}
}

// TestLDAPWrongPassword verifies 401 on bad credentials.
func TestLDAPWrongPassword(t *testing.T) {
	ts, _, root := newLDAPTestServer(t)

	putLDAPConfig(t, ts, root)
	putLDAPRole(t, ts, root, "r", []string{"devs"}, nil, []string{"root"}, "1h")

	status, _ := ldapLogin(t, ts, "alice", "wrongpassword")
	if status != http.StatusUnauthorized {
		t.Errorf("wrong password: %d, want 401", status)
	}
}

// TestLDAPNoMatchingRole verifies 403 when user is valid but no role matches.
func TestLDAPNoMatchingRole(t *testing.T) {
	ts, _, root := newLDAPTestServer(t)

	putLDAPConfig(t, ts, root)
	// Role that matches group "superadmin" — neither alice nor bob belong to it.
	putLDAPRole(t, ts, root, "superadmin-role", []string{"superadmin"}, nil, []string{"root"}, "1h")

	status, _ := ldapLogin(t, ts, "alice", "alicepass")
	if status != http.StatusForbidden {
		t.Errorf("no matching role: %d, want 403", status)
	}
}

// TestLDAPNotConfigured verifies 503 when LDAP config is missing.
func TestLDAPNotConfigured(t *testing.T) {
	ts, _, _ := newLDAPTestServer(t)
	// No config set — login must fail with 503.
	status, _ := ldapLogin(t, ts, "alice", "alicepass")
	if status != http.StatusServiceUnavailable {
		t.Errorf("not configured: %d, want 503", status)
	}
}

// TestLDAPRoleCRUD exercises the full role management lifecycle.
func TestLDAPRoleCRUD(t *testing.T) {
	ts, _, root := newLDAPTestServer(t)

	putLDAPConfig(t, ts, root)

	// Create.
	putLDAPRole(t, ts, root, "myrole", []string{"admin"}, nil, []string{"root"}, "2h")

	// Read back.
	status, body := doJSON(t, ts, http.MethodGet, "/v1/auth/ldap/role/myrole", "", root)
	if status != http.StatusOK {
		t.Fatalf("GET role: %d %s", status, body)
	}
	if !strings.Contains(string(body), "myrole") {
		t.Errorf("GET role body missing name: %s", body)
	}

	// List.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/auth/ldap/role/?list=true", "", root)
	if status != http.StatusOK {
		t.Fatalf("LIST roles: %d %s", status, body)
	}
	if !strings.Contains(string(body), "myrole") {
		t.Errorf("LIST roles missing myrole: %s", body)
	}

	// Delete.
	status, _ = doJSON(t, ts, http.MethodDelete, "/v1/auth/ldap/role/myrole", "", root)
	if status != http.StatusNoContent {
		t.Errorf("DELETE role: %d, want 204", status)
	}

	// Must be gone.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/auth/ldap/role/myrole", "", root)
	if status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("GET deleted role: %d, want 404 or 500", status)
	}
}

// TestLDAPUserDirectMatch verifies that a role matching by username (not group) works.
func TestLDAPUserDirectMatch(t *testing.T) {
	ts, _, root := newLDAPTestServer(t)

	putLDAPConfig(t, ts, root)
	// Role matches user "bob" directly, not via group.
	putLDAPRole(t, ts, root, "bob-role", nil, []string{"bob"}, []string{"root"}, "1h")

	status, body := ldapLogin(t, ts, "bob", "bobpass")
	if status != http.StatusOK {
		t.Fatalf("bob direct-match login: %d %s", status, body)
	}
	if jsonField(t, body, "token") == "" {
		t.Errorf("token empty, body: %s", body)
	}
}

// TestLDAPMissingCredentials verifies 400 when username or password is absent.
func TestLDAPMissingCredentials(t *testing.T) {
	ts, _, _ := newLDAPTestServer(t)

	for _, body := range []string{
		`{"username":"alice"}`,
		`{"password":"pass"}`,
		`{}`,
	} {
		status, _ := doJSON(t, ts, http.MethodPost, "/v1/auth/ldap/login", body, "")
		if status != http.StatusBadRequest {
			t.Errorf("body %s: %d, want 400", body, status)
		}
	}
}

// TestLDAPPolicyUnion verifies that a user matching multiple roles gets merged policies.
func TestLDAPPolicyUnion(t *testing.T) {
	ts, _, root := newLDAPTestServer(t)

	putLDAPConfig(t, ts, root)

	createTestPolicy(t, ts, root, "pol-admin", `[{"path":"secret/admin/*","capabilities":["read","write"]}]`)
	createTestPolicy(t, ts, root, "pol-devs", `[{"path":"secret/devs/*","capabilities":["read"]}]`)

	// Alice belongs to both "admin" and "devs" groups.
	putLDAPRole(t, ts, root, "admin-role", []string{"admin"}, nil, []string{"pol-admin"}, "1h")
	putLDAPRole(t, ts, root, "devs-role", []string{"devs"}, nil, []string{"pol-devs"}, "1h")

	status, body := ldapLogin(t, ts, "alice", "alicepass")
	if status != http.StatusOK {
		t.Fatalf("alice login: %d %s", status, body)
	}
	aliceTok := jsonField(t, body, "token")

	// Can access admin scope (from admin-role).
	status, _ = doJSON(t, ts, http.MethodPut, "/v1/secret/admin/key", `"val"`, aliceTok)
	if status != http.StatusNoContent {
		t.Errorf("alice admin write: %d, want 204", status)
	}

	// Can read devs scope (from devs-role).
	_, _ = doJSON(t, ts, http.MethodPut, "/v1/secret/devs/key", `"val"`, root)
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/secret/devs/key", "", aliceTok)
	if status != http.StatusOK {
		t.Errorf("alice devs read: %d, want 200", status)
	}
}
