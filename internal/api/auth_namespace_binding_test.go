package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
)

// nsReq performs a request with an explicit X-Tuck-Namespace header.
func nsReq(t *testing.T, ts *httptest.Server, method, path, body, tok, ns string) (int, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(authedNsReq(t, method, ts.URL+path, body, tok, ns))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, sb.String()
}

// seedTeams creates namespaces team-a and team-b, a read policy "rd" inside
// team-a, and one secret in each namespace.
func seedTeams(t *testing.T, ts *httptest.Server, root string) {
	t.Helper()
	for _, n := range []string{"team-a", "team-b"} {
		if s, b := nsReq(t, ts, http.MethodPost, "/v1/sys/namespaces", `{"name":"`+n+`"}`, root, ""); s != http.StatusCreated {
			t.Fatalf("create namespace %s: %d %s", n, s, b)
		}
		if s, b := nsReq(t, ts, http.MethodPut, "/v1/secret/prod/pass", `{"value":"`+strings.ToUpper(n)+`-SECRET"}`, root, n); s != http.StatusNoContent {
			t.Fatalf("seed %s: %d %s", n, s, b)
		}
	}
	rules := `{"rules":[{"path":"secret/**","capabilities":["read"]}]}`
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/policy/rd", rules, root, "team-a"); s != http.StatusNoContent {
		t.Fatalf("policy rd: %d %s", s, b)
	}
}

// assertBoundToTeamA checks that a login token lives in team-a only.
func assertBoundToTeamA(t *testing.T, ts *httptest.Server, tok string) {
	t.Helper()
	if tok == "" {
		t.Fatal("login returned an empty token")
	}
	if s, b := nsReq(t, ts, http.MethodGet, "/v1/secret/prod/pass", "", tok, ""); s != http.StatusOK || !strings.Contains(b, "TEAM-A-SECRET") {
		t.Fatalf("no header: want team-a secret, got %d %s", s, b)
	}
	if s, b := nsReq(t, ts, http.MethodGet, "/v1/secret/prod/pass", "", tok, "team-a"); s != http.StatusOK {
		t.Fatalf("own namespace: %d %s", s, b)
	}
	if s, b := nsReq(t, ts, http.MethodGet, "/v1/secret/prod/pass", "", tok, "team-b"); s != http.StatusForbidden || strings.Contains(b, "TEAM-B-SECRET") {
		t.Fatalf("team-b via header must be refused: %d %s", s, b)
	}
}

func TestK8sLoginRoleBoundToNamespace(t *testing.T) {
	reviewer := &mockReviewer{result: &k8sauth.ReviewResult{
		Authenticated: true,
		Username:      "system:serviceaccount:default:myapp",
	}}
	ts, _, root := newTestServerWithK8s(t, reviewer)
	seedTeams(t, ts, root)

	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/kubernetes/role/default/myapp", `{"policies":["rd"],"tuck_namespace":"team-a"}`, root, ""); s != http.StatusNoContent {
		t.Fatalf("put role: %d %s", s, b)
	}
	if s, b := nsReq(t, ts, http.MethodGet, "/v1/auth/kubernetes/role/default/myapp", "", root, ""); s != http.StatusOK || !strings.Contains(b, `"tuck_namespace":"team-a"`) {
		t.Fatalf("get role: %d %s", s, b)
	}

	// A client-supplied namespace header on login must not influence the token.
	resp := loginK8s(t, ts, "sa")
	defer func() { _ = resp.Body.Close() }()
	var sb strings.Builder
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	sb.Write(buf[:n])
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: %d %s", resp.StatusCode, sb.String())
	}
	assertBoundToTeamA(t, ts, jsonField(t, []byte(sb.String()), "token"))
}

func TestK8sLoginUnboundRoleStaysInRoot(t *testing.T) {
	reviewer := &mockReviewer{result: &k8sauth.ReviewResult{
		Authenticated: true,
		Username:      "system:serviceaccount:default:myapp",
	}}
	ts, c, root := newTestServerWithK8s(t, reviewer)
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/kubernetes/role/default/myapp", `{"policies":["default"]}`, root, ""); s != http.StatusNoContent {
		t.Fatalf("put role: %d %s", s, b)
	}
	resp := loginK8s(t, ts, "sa")
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	tok, err := c.LookupToken(t.Context(), jsonField(t, buf[:n], "token"))
	if err != nil {
		t.Fatal(err)
	}
	if tok.Namespace != "" {
		t.Errorf("unbound role must issue a root-namespace token, got %q", tok.Namespace)
	}
}

func TestRoleNamespaceValidation(t *testing.T) {
	ts, _, root := newTestServer(t)
	seedTeams(t, ts, root)

	// Unknown namespace is rejected.
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/approle/role/r", `{"policies":["rd"],"tuck_namespace":"nope"}`, root, ""); s != http.StatusNotFound {
		t.Errorf("unknown namespace: %d %s, want 404", s, b)
	}
	// A caller inside team-a cannot create a role bound to team-b ...
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/approle/role/r", `{"policies":["rd"],"tuck_namespace":"team-b"}`, root, "team-a"); s != http.StatusForbidden {
		t.Errorf("cross-namespace role: %d %s, want 403", s, b)
	}
	// ... and an unspecified namespace defaults to the caller's.
	s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/approle/role/r", `{"policies":["rd"]}`, root, "team-a")
	if s != http.StatusOK || !strings.Contains(b, `"tuck_namespace":"team-a"`) {
		t.Errorf("default namespace: %d %s", s, b)
	}
}

func TestAppRoleLoginRoleBoundToNamespace(t *testing.T) {
	ts, _, root := newTestServer(t)
	seedTeams(t, ts, root)

	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/approle/role/ar", `{"policies":["rd"],"tuck_namespace":"team-a"}`, root, ""); s != http.StatusOK {
		t.Fatalf("put role: %d %s", s, b)
	}
	_, b := nsReq(t, ts, http.MethodGet, "/v1/auth/approle/role/ar", "", root, "")
	roleID := jsonField(t, []byte(b), "role_id")
	_, b = nsReq(t, ts, http.MethodPost, "/v1/auth/approle/role/ar/secret-id", "", root, "")
	secretID := jsonField(t, []byte(b), "id")

	// Login with a hostile header: it must be ignored.
	s, b := nsReq(t, ts, http.MethodPost, "/v1/auth/approle/login", mustJSON(map[string]string{"role_id": roleID, "secret_id": secretID}), "", "team-b")
	if s != http.StatusOK {
		t.Fatalf("login: %d %s", s, b)
	}
	assertBoundToTeamA(t, ts, jsonField(t, []byte(b), "token"))
}

func TestJWTLoginRoleBoundToNamespace(t *testing.T) {
	ts, _, root := newTestServer(t)
	seedTeams(t, ts, root)
	env := newJWKSServer(t)
	const issuer = "https://issuer.example.com"
	configureJWT(t, ts, root, env.jwksURL, issuer)

	body := `{"bound_subject":"svc","policies":["rd"],"ttl":"1h","tuck_namespace":"team-a"}`
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/jwt/role/jr", body, root, ""); s != http.StatusOK {
		t.Fatalf("put role: %d %s", s, b)
	}
	jwtStr := env.sign(t, gojwt.MapClaims{"iss": issuer, "sub": "svc", "exp": time.Now().Add(time.Hour).Unix()})
	s, b := nsReq(t, ts, http.MethodPost, "/v1/auth/jwt/login", mustJSON(map[string]string{"jwt": jwtStr}), "", "team-b")
	if s != http.StatusOK {
		t.Fatalf("login: %d %s", s, b)
	}
	assertBoundToTeamA(t, ts, jsonField(t, []byte(b), "token"))
}

func TestLDAPLoginRoleBoundToNamespace(t *testing.T) {
	ts, _, root := newLDAPTestServer(t)
	seedTeams(t, ts, root)
	putLDAPConfig(t, ts, root)

	body := `{"groups":["admin"],"policies":["rd"],"ttl":"1h","tuck_namespace":"team-a"}`
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/ldap/role/lr", body, root, ""); s != http.StatusNoContent {
		t.Fatalf("put role: %d %s", s, b)
	}
	s, b := nsReq(t, ts, http.MethodPost, "/v1/auth/ldap/login", mustJSON(map[string]string{"username": "alice", "password": "alicepass"}), "", "team-b")
	if s != http.StatusOK {
		t.Fatalf("login: %d %s", s, b)
	}
	assertBoundToTeamA(t, ts, jsonField(t, []byte(b), "token"))

	// alice also matches a role bound to team-b: a token can only live in one
	// namespace, so the login must be refused rather than silently merged.
	body = `{"groups":["devs"],"policies":["rd"],"ttl":"1h","tuck_namespace":"team-b"}`
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/ldap/role/lr-b", body, root, ""); s != http.StatusNoContent {
		t.Fatalf("put role b: %d %s", s, b)
	}
	s, b = nsReq(t, ts, http.MethodPost, "/v1/auth/ldap/login", mustJSON(map[string]string{"username": "alice", "password": "alicepass"}), "", "")
	if s != http.StatusConflict || !strings.Contains(b, `"namespaces":["team-a","team-b"]`) {
		t.Fatalf("ambiguous login must ask for a namespace: %d %s", s, b)
	}
	// The choice list is only revealed after the password checked out.
	if s, b := nsReq(t, ts, http.MethodPost, "/v1/auth/ldap/login", mustJSON(map[string]string{"username": "alice", "password": "nope"}), "", ""); s != http.StatusUnauthorized || strings.Contains(b, "team-") {
		t.Fatalf("wrong password must be a plain 401: %d %s", s, b)
	}
	// Picking a namespace logs into exactly that one.
	s, b = nsReq(t, ts, http.MethodPost, "/v1/auth/ldap/login", mustJSON(map[string]string{"username": "alice", "password": "alicepass", "namespace": "team-a"}), "", "")
	if s != http.StatusOK {
		t.Fatalf("login team-a: %d %s", s, b)
	}
	assertBoundToTeamA(t, ts, jsonField(t, []byte(b), "token"))
	s, b = nsReq(t, ts, http.MethodPost, "/v1/auth/ldap/login", mustJSON(map[string]string{"username": "alice", "password": "alicepass", "namespace": "team-b"}), "", "")
	if s != http.StatusOK {
		t.Fatalf("login team-b: %d %s", s, b)
	}
	if s, b := nsReq(t, ts, http.MethodGet, "/v1/secret/prod/pass", "", jsonField(t, []byte(b), "token"), ""); strings.Contains(b, "TEAM-A-SECRET") {
		t.Fatalf("team-b token must not see team-a: %d %s", s, b)
	}
	// A namespace the user holds no role in is refused.
	if s, b := nsReq(t, ts, http.MethodPost, "/v1/auth/ldap/login", mustJSON(map[string]string{"username": "alice", "password": "alicepass", "namespace": "team-c"}), "", ""); s != http.StatusForbidden {
		t.Fatalf("foreign namespace must be refused: %d %s", s, b)
	}
}

// A role bound to a namespace that was later deleted must stop logging in.
func TestLoginRefusedWhenBoundNamespaceDeleted(t *testing.T) {
	reviewer := &mockReviewer{result: &k8sauth.ReviewResult{
		Authenticated: true,
		Username:      "system:serviceaccount:default:myapp",
	}}
	ts, _, root := newTestServerWithK8s(t, reviewer)
	seedTeams(t, ts, root)
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/kubernetes/role/default/myapp", `{"policies":["rd"],"tuck_namespace":"team-a"}`, root, ""); s != http.StatusNoContent {
		t.Fatalf("put role: %d %s", s, b)
	}
	if s, b := nsReq(t, ts, http.MethodDelete, "/v1/sys/namespaces/team-a", "", root, ""); s >= 300 {
		t.Fatalf("delete namespace: %d %s", s, b)
	}
	resp := loginK8s(t, ts, "sa")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("login into deleted namespace: got %d, want 403", resp.StatusCode)
	}
}

// Deleting a namespace must destroy its data: recreating the name may not
// bring the old secrets and policies back.
func TestDeletedNamespaceDataDoesNotResurrect(t *testing.T) {
	ts, _, root := newLDAPTestServer(t)
	seedTeams(t, ts, root)
	if s, b := nsReq(t, ts, http.MethodDelete, "/v1/sys/namespaces/team-a", "", root, ""); s >= 300 {
		t.Fatalf("delete: %d %s", s, b)
	}
	if s, b := nsReq(t, ts, http.MethodPost, "/v1/sys/namespaces", `{"name":"team-a"}`, root, ""); s != http.StatusCreated {
		t.Fatalf("recreate: %d %s", s, b)
	}
	if s, b := nsReq(t, ts, http.MethodGet, "/v1/secret/prod/pass", "", root, "team-a"); s != http.StatusNotFound || strings.Contains(b, "TEAM-A-SECRET") {
		t.Fatalf("old secret came back: %d %s", s, b)
	}
	if s, b := nsReq(t, ts, http.MethodGet, "/v1/policy/rd", "", root, "team-a"); s != http.StatusNotFound {
		t.Fatalf("old policy came back: %d %s", s, b)
	}
	if s, b := nsReq(t, ts, http.MethodGet, "/v1/secret/prod/pass", "", root, "team-b"); s != http.StatusOK || !strings.Contains(b, "TEAM-B-SECRET") {
		t.Fatalf("team-b must be untouched: %d %s", s, b)
	}
}
