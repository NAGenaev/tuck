package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- helpers ---

type route struct {
	method string
	path   string
	status int
	body   any
}

func testServer(t *testing.T, routes []route) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for _, rt := range routes {
		rt := rt
		mux.HandleFunc(rt.path, func(w http.ResponseWriter, r *http.Request) {
			if rt.method != "" && r.Method != rt.method {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(rt.status)
			if rt.body != nil {
				json.NewEncoder(w).Encode(rt.body)
			}
		})
	}
	return httptest.NewServer(mux)
}

// --- NamespaceClient ---

func TestNamespaceCreate(t *testing.T) {
	srv := testServer(t, []route{{
		method: http.MethodPost,
		path:   "/v1/sys/namespaces",
		status: http.StatusOK,
		body:   map[string]string{"name": "prod", "created_at": "2026-01-01T00:00:00Z"},
	}})
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	ns, err := c.Namespaces().Create(context.Background(), "prod")
	if err != nil {
		t.Fatal(err)
	}
	if ns.Name != "prod" {
		t.Fatalf("expected prod, got %s", ns.Name)
	}
}

func TestNamespaceGet(t *testing.T) {
	srv := testServer(t, []route{{
		method: http.MethodGet,
		path:   "/v1/sys/namespaces/dev",
		status: http.StatusOK,
		body:   map[string]string{"name": "dev"},
	}})
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	ns, err := c.Namespaces().Get(context.Background(), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if ns.Name != "dev" {
		t.Fatalf("expected dev, got %s", ns.Name)
	}
}

func TestNamespaceDelete(t *testing.T) {
	srv := testServer(t, []route{{
		method: http.MethodDelete,
		path:   "/v1/sys/namespaces/dev",
		status: http.StatusNoContent,
	}})
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	if err := c.Namespaces().Delete(context.Background(), "dev"); err != nil {
		t.Fatal(err)
	}
}

func TestNamespaceList(t *testing.T) {
	srv := testServer(t, []route{{
		method: "LIST",
		path:   "/v1/sys/namespaces/",
		status: http.StatusOK,
		body:   map[string][]string{"keys": {"dev", "prod"}},
	}})
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	keys, err := c.Namespaces().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "dev" {
		t.Fatalf("unexpected keys: %v", keys)
	}
}

// --- TokenRoleClient ---

func TestTokenRolePutGet(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v1/auth/token/roles/reader":
			json.NewDecoder(r.Body).Decode(&got)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/token/roles/reader":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"name": "reader", "policies": []string{"read-only"}, "renewable": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	if err := c.TokenRoles().Put(context.Background(), TokenRole{Name: "reader", Policies: []string{"read-only"}}); err != nil {
		t.Fatal(err)
	}
	role, err := c.TokenRoles().Get(context.Background(), "reader")
	if err != nil {
		t.Fatal(err)
	}
	if role.Name != "reader" {
		t.Fatalf("expected reader, got %s", role.Name)
	}
}

// --- AuditClient ---

func TestAuditEnableWebhook(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/v1/sys/audit/webhook/events" {
			json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	if err := c.Audit().EnableWebhook(context.Background(), "events", "http://logger:9000/", 10); err != nil {
		t.Fatal(err)
	}
	if gotBody["url"] != "http://logger:9000/" {
		t.Fatalf("unexpected url: %v", gotBody["url"])
	}
}

func TestAuditList(t *testing.T) {
	srv := testServer(t, []route{{
		method: "LIST",
		path:   "/v1/sys/audit/",
		status: http.StatusOK,
		body: map[string]any{
			"sinks": []map[string]any{
				{"name": "events", "type": "webhook", "errors": 0},
			},
		},
	}})
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	sinks, err := c.Audit().List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sinks) != 1 || sinks[0].Name != "events" {
		t.Fatalf("unexpected sinks: %v", sinks)
	}
}

// --- AuthClient ---

func TestLoginAppRole(t *testing.T) {
	srv := testServer(t, []route{{
		method: http.MethodPost,
		path:   "/v1/auth/approle/login",
		status: http.StatusOK,
		body:   map[string]any{"token": "tok-123", "policies": []string{"default"}},
	}})
	defer srv.Close()

	c := New(srv.URL, "", WithHTTPClient(srv.Client()))
	res, err := c.Auth().LoginAppRole(context.Background(), "role-id", "secret-id")
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "tok-123" {
		t.Fatalf("expected tok-123, got %s", res.Token)
	}
}

func TestLoginGitHub(t *testing.T) {
	srv := testServer(t, []route{{
		method: http.MethodPost,
		path:   "/v1/auth/github/login",
		status: http.StatusOK,
		body:   map[string]any{"token": "gh-tok", "policies": []string{"ci"}},
	}})
	defer srv.Close()

	c := New(srv.URL, "", WithHTTPClient(srv.Client()))
	res, err := c.Auth().LoginGitHub(context.Background(), "oidc-jwt", "ci-role")
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "gh-tok" {
		t.Fatalf("expected gh-tok, got %s", res.Token)
	}
}

func TestLookupSelf(t *testing.T) {
	srv := testServer(t, []route{{
		method: http.MethodGet,
		path:   "/v1/auth/token/self",
		status: http.StatusOK,
		body:   map[string]any{"id": "tok-abc", "policies": []string{"default"}},
	}})
	defer srv.Close()

	c := New(srv.URL, "tok-abc", WithHTTPClient(srv.Client()))
	tok, err := c.LookupSelf(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.ID != "tok-abc" {
		t.Fatalf("expected tok-abc, got %s", tok.ID)
	}
}

// --- WrappingClient ---

func TestWrappingWrapUnwrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/sys/wrapping/wrap":
			json.NewEncoder(w).Encode(map[string]any{"token": "wrap-tok", "ttl": 300})
		case "/v1/sys/wrapping/unwrap":
			json.NewEncoder(w).Encode(map[string]string{"secret": "mysecret"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	wt, err := c.Wrapping().Wrap(context.Background(), map[string]string{"key": "val"})
	if err != nil {
		t.Fatal(err)
	}
	if wt.Token != "wrap-tok" {
		t.Fatalf("expected wrap-tok, got %s", wt.Token)
	}

	var out map[string]string
	if err := c.Wrapping().Unwrap(context.Background(), wt.Token, &out); err != nil {
		t.Fatal(err)
	}
	if out["secret"] != "mysecret" {
		t.Fatalf("unexpected unwrap result: %v", out)
	}
}

// --- CubbyholeClient ---

func TestCubbyholeGetPut(t *testing.T) {
	var stored []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v1/cubbyhole/temp/pass":
			stored, _ = json.Marshal(map[string]string{"value": "secret123"})
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/cubbyhole/temp/pass":
			w.Header().Set("Content-Type", "application/json")
			w.Write(stored)
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	if err := c.Cubbyhole().Put(context.Background(), "temp/pass", []byte("secret123")); err != nil {
		t.Fatal(err)
	}
	val, err := c.Cubbyhole().Get(context.Background(), "temp/pass")
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "secret123" {
		t.Fatalf("expected secret123, got %s", val)
	}
}

func TestCubbyholeNotFound(t *testing.T) {
	srv := testServer(t, []route{{
		method: http.MethodGet,
		path:   "/v1/cubbyhole/missing",
		status: http.StatusNotFound,
		body:   map[string]string{"error": "not found"},
	}})
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	val, err := c.Cubbyhole().Get(context.Background(), "missing")
	if err != nil {
		t.Fatalf("expected nil error for 404, got %v", err)
	}
	if val != nil {
		t.Fatal("expected nil value")
	}
}

// --- Scoped / namespace ---

func TestScopedNamespaceHeader(t *testing.T) {
	var gotNS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotNS = r.Header.Get("X-Tuck-Namespace")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"value": "v"})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", WithHTTPClient(srv.Client()))
	sc := c.Scoped("prod")
	sc.GetSecret(context.Background(), "key")
	if gotNS != "prod" {
		t.Fatalf("expected prod namespace header, got %q", gotNS)
	}
}
