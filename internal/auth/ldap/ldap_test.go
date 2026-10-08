package ldap_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	authlda "github.com/NAGenaev/tuck/internal/auth/ldap"
)

// --- fake LDAP connection ---

type fakeUser struct {
	DN       string
	Password string
	Groups   []string // full DNs, e.g. "CN=admin,DC=test"
}

type fakeLDAPConn struct {
	users      map[string]fakeUser // username → user
	servicesDN string              // expected service account DN
	servicesPS string              // expected service account password
	bound      string              // currently bound DN
}

func newFakeConn(servicesDN, servicesPS string, users map[string]fakeUser) *fakeLDAPConn {
	return &fakeLDAPConn{
		users:      users,
		servicesDN: servicesDN,
		servicesPS: servicesPS,
	}
}

func (f *fakeLDAPConn) Bind(dn, password string) error {
	if dn == f.servicesDN && password == f.servicesPS {
		f.bound = dn
		return nil
	}
	for _, u := range f.users {
		if u.DN == dn && u.Password == password {
			f.bound = dn
			return nil
		}
	}
	return errors.New("ldap: invalid credentials")
}

func (f *fakeLDAPConn) Search(baseDN, filter string, _ []string) ([]*authlda.Entry, error) {
	var out []*authlda.Entry
	// User search: filter contains the username attribute value.
	for username, u := range f.users {
		if strings.Contains(filter, username) || strings.Contains(filter, u.DN) {
			e := &authlda.Entry{
				DN: u.DN,
				Attributes: map[string][]string{
					"uid":      {username},
					"memberOf": u.Groups,
				},
			}
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeLDAPConn) Close() error { return nil }

// dialFactory returns a dial function that always returns conn.
func dialFactory(conn authlda.Conn) func(ctx context.Context, cfg authlda.Config) (authlda.Conn, error) {
	return func(_ context.Context, _ authlda.Config) (authlda.Conn, error) {
		return conn, nil
	}
}

// --- helpers ---

func sampleCfg() authlda.Config {
	return authlda.Config{
		URLs:         []string{"ldap://localhost:389"},
		BindDN:       "cn=svc,dc=test",
		BindPassword: "svcpass",
		UserDN:       "ou=users,dc=test",
	}
}

func sampleUsers() map[string]fakeUser {
	return map[string]fakeUser{
		"alice": {DN: "uid=alice,ou=users,dc=test", Password: "alicepass", Groups: []string{"CN=admin,DC=test", "CN=devs,DC=test"}},
		"bob":   {DN: "uid=bob,ou=users,dc=test", Password: "bobpass", Groups: []string{"CN=devs,DC=test"}},
	}
}

// --- tests ---

// TestLogin_SuccessGroupMatch verifies a user who belongs to a group gets the role's policies.
func TestLogin_SuccessGroupMatch(t *testing.T) {
	users := sampleUsers()
	conn := newFakeConn("cn=svc,dc=test", "svcpass", users)
	cfg := sampleCfg()

	roles := []*authlda.Role{
		{Name: "admin-role", Groups: []string{"admin"}, Policies: []string{"admin-policy"}, TTL: time.Hour},
	}

	auth := authlda.NewAuthenticator(cfg, authlda.WithDialer(dialFactory(conn)))
	result, err := auth.Login(context.Background(), roles, "alice", "alicepass")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.Username != "alice" {
		t.Errorf("Username = %q, want \"alice\"", result.Username)
	}
	if len(result.Policies) == 0 || result.Policies[0] != "admin-policy" {
		t.Errorf("Policies = %v, want [admin-policy]", result.Policies)
	}
	if result.TTL != time.Hour {
		t.Errorf("TTL = %v, want 1h", result.TTL)
	}
}

// TestLogin_ZeroTTLFallsBackToOneHour guards against a role with no TTL
// configured minting an eternal token: token.Generate leaves ExpiresAt at
// its zero value when ttl <= 0, which IsExpired() documents as "never
// expires" — so an unset role TTL must be given a real default here, the
// same way AppRole/JWT/GitHub already do.
func TestLogin_ZeroTTLFallsBackToOneHour(t *testing.T) {
	users := sampleUsers()
	conn := newFakeConn("cn=svc,dc=test", "svcpass", users)
	cfg := sampleCfg()

	roles := []*authlda.Role{
		{Name: "admin-role", Groups: []string{"admin"}, Policies: []string{"admin-policy"}}, // TTL unset
	}

	auth := authlda.NewAuthenticator(cfg, authlda.WithDialer(dialFactory(conn)))
	result, err := auth.Login(context.Background(), roles, "alice", "alicepass")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.TTL != time.Hour {
		t.Errorf("TTL with unset role TTL = %v, want 1h fallback, not eternal", result.TTL)
	}
}

// TestLogin_SuccessUserMatch verifies that a direct username match in a role works.
func TestLogin_SuccessUserMatch(t *testing.T) {
	users := sampleUsers()
	conn := newFakeConn("cn=svc,dc=test", "svcpass", users)
	cfg := sampleCfg()

	roles := []*authlda.Role{
		{Name: "bob-role", Users: []string{"bob"}, Policies: []string{"readonly"}},
	}

	auth := authlda.NewAuthenticator(cfg, authlda.WithDialer(dialFactory(conn)))
	result, err := auth.Login(context.Background(), roles, "bob", "bobpass")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if len(result.Policies) == 0 || result.Policies[0] != "readonly" {
		t.Errorf("Policies = %v, want [readonly]", result.Policies)
	}
}

// TestLogin_WrongPassword verifies ErrInvalidCredentials is returned.
func TestLogin_WrongPassword(t *testing.T) {
	users := sampleUsers()
	conn := newFakeConn("cn=svc,dc=test", "svcpass", users)
	cfg := sampleCfg()

	auth := authlda.NewAuthenticator(cfg, authlda.WithDialer(dialFactory(conn)))
	_, err := auth.Login(context.Background(), nil, "alice", "wrongpass")
	if !errors.Is(err, authlda.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

// TestLogin_UnknownUser verifies ErrInvalidCredentials for a non-existent user.
func TestLogin_UnknownUser(t *testing.T) {
	users := sampleUsers()
	conn := newFakeConn("cn=svc,dc=test", "svcpass", users)
	cfg := sampleCfg()

	auth := authlda.NewAuthenticator(cfg, authlda.WithDialer(dialFactory(conn)))
	_, err := auth.Login(context.Background(), nil, "nobody", "pass")
	if !errors.Is(err, authlda.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

// TestLogin_NoMatchingRole verifies ErrNoRole when user is valid but no role applies.
func TestLogin_NoMatchingRole(t *testing.T) {
	users := sampleUsers()
	conn := newFakeConn("cn=svc,dc=test", "svcpass", users)
	cfg := sampleCfg()

	roles := []*authlda.Role{
		{Name: "other-role", Groups: []string{"nobody"}, Policies: []string{"p1"}},
	}

	auth := authlda.NewAuthenticator(cfg, authlda.WithDialer(dialFactory(conn)))
	_, err := auth.Login(context.Background(), roles, "alice", "alicepass")
	if !errors.Is(err, authlda.ErrNoRole) {
		t.Errorf("expected ErrNoRole, got %v", err)
	}
}

// TestLogin_PolicyUnion verifies that policies from multiple matching roles are merged.
func TestLogin_PolicyUnion(t *testing.T) {
	users := sampleUsers()
	conn := newFakeConn("cn=svc,dc=test", "svcpass", users)
	cfg := sampleCfg()

	roles := []*authlda.Role{
		{Name: "admin-role", Groups: []string{"admin"}, Policies: []string{"admin-policy"}},
		{Name: "devs-role", Groups: []string{"devs"}, Policies: []string{"dev-policy"}},
	}

	auth := authlda.NewAuthenticator(cfg, authlda.WithDialer(dialFactory(conn)))
	result, err := auth.Login(context.Background(), roles, "alice", "alicepass")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if len(result.Policies) != 2 {
		t.Errorf("Policies count = %d, want 2; got %v", len(result.Policies), result.Policies)
	}
}

// TestLogin_ConnectError verifies that a dial failure is surfaced.
func TestLogin_ConnectError(t *testing.T) {
	cfg := sampleCfg()
	failDial := func(_ context.Context, _ authlda.Config) (authlda.Conn, error) {
		return nil, errors.New("connection refused")
	}

	auth := authlda.NewAuthenticator(cfg, authlda.WithDialer(failDial))
	_, err := auth.Login(context.Background(), nil, "alice", "pass")
	if err == nil {
		t.Error("expected error on dial failure, got nil")
	}
}

type dupConn struct{ *fakeLDAPConn }

func (d dupConn) Search(_, _ string, _ []string) ([]*authlda.Entry, error) {
	return []*authlda.Entry{
		{DN: "uid=alice,ou=a,dc=test"}, {DN: "uid=alice,ou=b,dc=test"},
	}, nil
}

// TestLogin_AmbiguousUserRejected verifies that two directory entries matching
// one username never resolve to either of them.
func TestLogin_AmbiguousUserRejected(t *testing.T) {
	conn := dupConn{newFakeConn("cn=svc,dc=test", "svcpass", sampleUsers())}
	auth := authlda.NewAuthenticator(sampleCfg(), authlda.WithDialer(dialFactory(conn)))
	_, err := auth.Login(context.Background(), nil, "alice", "alicepass")
	if !errors.Is(err, authlda.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_NamespaceSelection(t *testing.T) {
	conn := newFakeConn("cn=svc,dc=test", "svcpass", sampleUsers())
	auth := authlda.NewAuthenticator(sampleCfg(), authlda.WithDialer(dialFactory(conn)))
	roles := []*authlda.Role{
		{Name: "a", Groups: []string{"admin"}, Policies: []string{"pa"}, TuckNamespace: "team-a"},
		{Name: "b", Groups: []string{"devs"}, Policies: []string{"pb"}, TuckNamespace: "team-b"},
		{Name: "b2", Groups: []string{"devs"}, Policies: []string{"pb2"}, TuckNamespace: "team-b"},
	}
	ctx := context.Background()

	// alice is in both; without a choice she gets the list, not a token.
	_, err := auth.Login(ctx, roles, "alice", "alicepass")
	var choice *authlda.NamespaceChoiceError
	if !errors.As(err, &choice) || len(choice.Namespaces) != 2 || choice.Namespaces[0] != "team-a" || choice.Namespaces[1] != "team-b" {
		t.Fatalf("want a namespace choice [team-a team-b], got %v", err)
	}

	// Choosing merges only that namespace's roles.
	ns := "team-b"
	res, err := auth.LoginNamespace(ctx, roles, "alice", "alicepass", &ns)
	if err != nil || res.TuckNamespace != "team-b" || len(res.Policies) != 2 {
		t.Fatalf("team-b login: %+v %v", res, err)
	}

	// bob only has team-b, so no choice is needed.
	res, err = auth.Login(ctx, roles, "bob", "bobpass")
	if err != nil || res.TuckNamespace != "team-b" {
		t.Fatalf("bob: %+v %v", res, err)
	}

	// Asking for a namespace without a role there is "no role", and the root
	// namespace is just another choice.
	ns = "team-z"
	if _, err := auth.LoginNamespace(ctx, roles, "alice", "alicepass", &ns); !errors.Is(err, authlda.ErrNoRole) {
		t.Fatalf("foreign namespace: %v", err)
	}
	roles = append(roles, &authlda.Role{Name: "r", Groups: []string{"admin"}, Policies: []string{"pr"}})
	ns = ""
	if res, err := auth.LoginNamespace(ctx, roles, "alice", "alicepass", &ns); err != nil || res.TuckNamespace != "" || res.Policies[0] != "pr" {
		t.Fatalf("root login: %+v %v", res, err)
	}
}

// unavailableConn fails the way a dead directory does: after a successful dial,
// the named operation returns an error that realConn marks as ErrUnavailable.
type unavailableConn struct {
	*fakeLDAPConn
	failBind   string // DN whose Bind reports the directory as unavailable
	failSearch bool
}

func (u unavailableConn) Bind(dn, password string) error {
	if dn == u.failBind {
		return fmt.Errorf("%w: network error", authlda.ErrUnavailable)
	}
	return u.fakeLDAPConn.Bind(dn, password)
}

func (u unavailableConn) Search(b, f string, a []string) ([]*authlda.Entry, error) {
	if u.failSearch {
		return nil, errors.New("connection reset")
	}
	return u.fakeLDAPConn.Search(b, f, a)
}

// TestLogin_DirectoryFailuresAreUnavailable verifies that everything that is
// not the user's fault maps to ErrUnavailable, never to ErrInvalidCredentials.
func TestLogin_DirectoryFailuresAreUnavailable(t *testing.T) {
	base := newFakeConn("cn=svc,dc=test", "svcpass", sampleUsers())
	cases := map[string]authlda.Conn{
		"service bind": unavailableConn{fakeLDAPConn: base, failBind: "cn=svc,dc=test"},
		"user bind":    unavailableConn{fakeLDAPConn: base, failBind: "uid=alice,ou=users,dc=test"},
		"search":       unavailableConn{fakeLDAPConn: base, failSearch: true},
	}
	for name, conn := range cases {
		t.Run(name, func(t *testing.T) {
			auth := authlda.NewAuthenticator(sampleCfg(), authlda.WithDialer(dialFactory(conn)))
			_, err := auth.Login(context.Background(), nil, "alice", "alicepass")
			if !errors.Is(err, authlda.ErrUnavailable) {
				t.Fatalf("expected ErrUnavailable, got %v", err)
			}
			if errors.Is(err, authlda.ErrInvalidCredentials) {
				t.Fatalf("directory failure must not look like a wrong password: %v", err)
			}
		})
	}
}
