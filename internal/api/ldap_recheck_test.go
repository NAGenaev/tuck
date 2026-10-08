package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	authlda "github.com/NAGenaev/tuck/internal/auth/ldap"
	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

// liveDir is a directory the test can change after people have logged in.
type liveDir struct {
	mu     sync.Mutex
	users  map[string]fakeLDAPUser
	down   bool
	binds  int
}

func (d *liveDir) dial(_ context.Context, _ authlda.Config) (authlda.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.down {
		return nil, errors.New("connection refused")
	}
	cp := make(map[string]fakeLDAPUser, len(d.users))
	for k, v := range d.users {
		cp[k] = v
	}
	return &fakeLDAPConn{users: cp, serviceDN: "cn=svc,dc=test", servicePS: "svcpass"}, nil
}

func (d *liveDir) set(f func(users map[string]fakeLDAPUser)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f(d.users)
}

func newRecheckServer(t *testing.T) (*httptest.Server, *liveDir, string) {
	t.Helper()
	c := core.New(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	res, err := c.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dir := &liveDir{users: map[string]fakeLDAPUser{
		"alice": {dn: "uid=alice,ou=users,dc=test", password: "alicepass", groups: []string{"CN=admin,DC=test", "CN=devs,DC=test"}},
		"bob":   {dn: "uid=bob,ou=users,dc=test", password: "bobpass", groups: []string{"CN=devs,DC=test"}},
	}}
	c.SetLDAPDialer(dir.dial)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(New(c).Handler())
	ts.Listener = &rstListener{ln}
	ts.Start()
	t.Cleanup(ts.Close)
	return ts, dir, res.RootToken.ID
}

func recheck(t *testing.T, ts *httptest.Server, root string) string {
	t.Helper()
	s, b := doJSON(t, ts, http.MethodPost, "/v1/auth/ldap/recheck", "", root)
	if s != http.StatusOK {
		t.Fatalf("recheck: %d %s", s, b)
	}
	return string(b)
}

func tokenWorks(t *testing.T, ts *httptest.Server, tok string) bool {
	t.Helper()
	s, _ := doJSON(t, ts, http.MethodGet, "/v1/auth/token/lookup-self", "", tok)
	return s == http.StatusOK
}

func TestLDAPRecheckRevokesWhoLeftTheGroup(t *testing.T) {
	ts, dir, root := newRecheckServer(t)
	putLDAPConfig(t, ts, root)
	createTestPolicy(t, ts, root, "p-admin", `[{"path":"secret/a/*","capabilities":["read","write"]}]`)
	createTestPolicy(t, ts, root, "p-dev", `[{"path":"secret/d/*","capabilities":["read"]}]`)
	putLDAPRole(t, ts, root, "admins", []string{"admin"}, nil, []string{"p-admin"}, "8h")
	putLDAPRole(t, ts, root, "devs", []string{"devs"}, nil, []string{"p-dev"}, "8h")

	login := func(u, p string) string {
		s, b := ldapLogin(t, ts, u, p)
		if s != http.StatusOK {
			t.Fatalf("%s login: %d %s", u, s, b)
		}
		return jsonField(t, b, "token")
	}
	alice, bob := login("alice", "alicepass"), login("bob", "bobpass")

	// A hand-made token that only carries an "ldap:" name must never be touched.
	s, b := doJSON(t, ts, http.MethodPost, "/v1/auth/token", `{"display_name":"ldap:alice","policies":["p-dev"]}`, root)
	if s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("manual token: %d %s", s, b)
	}
	manual := jsonField(t, b, "id")

	// Nothing changed in the directory: nothing is revoked.
	if out := recheck(t, ts, root); !strings.Contains(out, `"revoked":0`) || !strings.Contains(out, `"narrowed":0`) {
		t.Fatalf("idle recheck changed something: %s", out)
	}
	if !tokenWorks(t, ts, alice) || !tokenWorks(t, ts, bob) {
		t.Fatal("tokens must survive an idle recheck")
	}

	// alice leaves "admin" but stays in "devs": narrowed to the dev policy.
	dir.set(func(u map[string]fakeLDAPUser) {
		a := u["alice"]
		a.groups = []string{"CN=devs,DC=test"}
		u["alice"] = a
	})
	if out := recheck(t, ts, root); !strings.Contains(out, `"narrowed":1`) {
		t.Fatalf("alice should be narrowed: %s", out)
	}
	if s, _ := doJSON(t, ts, http.MethodPut, "/v1/secret/a/x", `{"v":"1"}`, alice); s != http.StatusForbidden {
		t.Errorf("alice kept admin write access: %d", s)
	}
	if s, _ := doJSON(t, ts, http.MethodGet, "/v1/secret/d/x", "", alice); s == http.StatusForbidden {
		t.Errorf("alice lost the access she still has")
	}

	// bob is removed from every group: revoked.
	dir.set(func(u map[string]fakeLDAPUser) {
		b := u["bob"]
		b.groups = nil
		u["bob"] = b
	})
	out := recheck(t, ts, root)
	if !strings.Contains(out, `"revoked":1`) || !strings.Contains(out, "bob") {
		t.Fatalf("bob should be revoked: %s", out)
	}
	if tokenWorks(t, ts, bob) {
		t.Error("bob's token still works")
	}
	if !tokenWorks(t, ts, alice) || !tokenWorks(t, ts, manual) {
		t.Error("alice's token and the hand-made token must survive")
	}

	// alice's account is deleted from the directory: revoked.
	dir.set(func(u map[string]fakeLDAPUser) { delete(u, "alice") })
	recheck(t, ts, root)
	if tokenWorks(t, ts, alice) {
		t.Error("token of a deleted account still works")
	}
	if !tokenWorks(t, ts, manual) {
		t.Error("hand-made token was revoked")
	}
}

func TestLDAPRecheckNeverRevokesWhenDirectoryIsDown(t *testing.T) {
	ts, dir, root := newRecheckServer(t)
	putLDAPConfig(t, ts, root)
	putLDAPRole(t, ts, root, "devs", []string{"devs"}, nil, []string{"default"}, "8h")
	s, b := ldapLogin(t, ts, "bob", "bobpass")
	if s != http.StatusOK {
		t.Fatalf("login: %d %s", s, b)
	}
	bob := jsonField(t, b, "token")

	dir.mu.Lock()
	dir.down = true
	dir.mu.Unlock()
	out := recheck(t, ts, root)
	if !strings.Contains(out, `"revoked":0`) || !strings.Contains(out, `"errors":1`) {
		t.Fatalf("outage must count as an error, not a revocation: %s", out)
	}
	if !tokenWorks(t, ts, bob) {
		t.Error("an unreachable directory revoked a token")
	}
}

func TestLDAPRevokeUserEndpoint(t *testing.T) {
	ts, _, root := newRecheckServer(t)
	putLDAPConfig(t, ts, root)
	putLDAPRole(t, ts, root, "devs", []string{"devs"}, nil, []string{"default"}, "8h")
	var toks []string
	for i := 0; i < 2; i++ {
		s, b := ldapLogin(t, ts, "bob", "bobpass")
		if s != http.StatusOK {
			t.Fatalf("login: %d %s", s, b)
		}
		toks = append(toks, jsonField(t, b, "token"))
	}
	s, b := doJSON(t, ts, http.MethodPost, "/v1/auth/ldap/users/bob/revoke", "", root)
	if s != http.StatusOK || !strings.Contains(string(b), `"revoked":2`) {
		t.Fatalf("revoke-user: %d %s", s, b)
	}
	for _, tk := range toks {
		if tokenWorks(t, ts, tk) {
			t.Error("a token of the revoked user still works")
		}
	}
	// A person without the right cannot do it.
	s, b = ldapLogin(t, ts, "bob", "bobpass")
	if s == http.StatusOK {
		if s2, _ := doJSON(t, ts, http.MethodPost, "/v1/auth/ldap/users/bob/revoke", "", jsonField(t, b, "token")); s2 != http.StatusForbidden {
			t.Errorf("unprivileged revoke-user: %d, want 403", s2)
		}
	}
}
