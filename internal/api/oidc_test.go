package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

const (
	oidcTestClient   = "tuck-ui"
	oidcTestRedirect = "https://tuck.example.com/ui/auth/oidc/callback"
	oidcTestIssuer   = "https://idp.example.com"
)

// fakeIdP plays the identity provider's token endpoint. Authorization in the
// browser is simulated by authorize(), which turns a Tuck auth URL into a code.
type fakeIdP struct {
	env    *jwtTestEnv
	srv    *httptest.Server
	secret string

	mu        sync.Mutex
	codes     map[string]fakeGrant
	lastBasic [2]string
	claims    func(nonce string) gojwt.MapClaims
}

type fakeGrant struct{ nonce, challenge string }

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	idp := &fakeIdP{env: newJWKSServer(t), codes: map[string]fakeGrant{}}
	idp.claims = func(nonce string) gojwt.MapClaims {
		return gojwt.MapClaims{
			"iss": oidcTestIssuer, "aud": oidcTestClient, "sub": "alice", "nonce": nonce,
			"groups": []string{"team-a-devs"},
			"exp":    time.Now().Add(time.Hour).Unix(),
		}
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	idp.srv = httptest.NewUnstartedServer(http.HandlerFunc(idp.token))
	idp.srv.Listener = &rstListener{ln}
	idp.srv.Start()
	t.Cleanup(idp.srv.Close)
	return idp
}

func (i *fakeIdP) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	i.mu.Lock()
	defer i.mu.Unlock()
	u, p, _ := r.BasicAuth()
	i.lastBasic = [2]string{u, p}
	g, ok := i.codes[r.PostForm.Get("code")]
	delete(i.codes, r.PostForm.Get("code")) // authorization codes are single-use
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || r.PostForm.Get("redirect_uri") != oidcTestRedirect ||
		base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge ||
		(i.secret != "" && (u != oidcTestClient || p != i.secret)) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, i.claims(g.nonce))
	tok.Header["kid"] = "test-key"
	s, _ := tok.SignedString(i.env.priv)
	_ = json.NewEncoder(w).Encode(map[string]string{"id_token": s, "access_token": "unused"})
}

// authorize is what the user's browser + IdP do between auth-url and callback.
func (i *fakeIdP) authorize(t *testing.T, authURL, code string) {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("client_id") != oidcTestClient ||
		q.Get("response_type") != "code" || q.Get("redirect_uri") != oidcTestRedirect {
		t.Fatalf("bad authorization request: %v", q)
	}
	i.mu.Lock()
	i.codes[code] = fakeGrant{nonce: q.Get("nonce"), challenge: q.Get("code_challenge")}
	i.mu.Unlock()
}

func configureOIDC(t *testing.T, ts *httptest.Server, root string, idp *fakeIdP, secret string) {
	t.Helper()
	cfg := map[string]any{
		"jwks_uri":               idp.env.jwksURL,
		"issuer":                 oidcTestIssuer,
		"default_ttl":            "1h",
		"oidc_authorization_url": "http://127.0.0.1:1/authorize",
		"oidc_token_url":         idp.srv.URL,
		"oidc_client_id":         oidcTestClient,
		"oidc_redirect_uri":      oidcTestRedirect,
		"oidc_scopes":            "openid profile groups",
	}
	if secret != "" {
		cfg["oidc_client_secret"] = secret
		idp.secret = secret
	}
	if s, b := doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/config", mustJSON(cfg), root); s != http.StatusOK {
		t.Fatalf("configure: %d %s", s, b)
	}
}

func putGroupRole(t *testing.T, ts *httptest.Server, root, name string, groups []string, policy, tuckNS string) {
	t.Helper()
	body := mustJSON(map[string]any{
		"bound_groups": groups, "policies": []string{policy}, "ttl": "1h", "tuck_namespace": tuckNS,
	})
	if s, b := doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/role/"+name, body, root); s != http.StatusOK {
		t.Fatalf("put role: %d %s", s, b)
	}
}

func startOIDC(t *testing.T, ts *httptest.Server, idp *fakeIdP) (authURL, state string) {
	t.Helper()
	s, b := doJSON(t, ts, http.MethodGet, "/v1/auth/oidc/auth-url", "", "")
	if s != http.StatusOK {
		t.Fatalf("auth-url: %d %s", s, b)
	}
	return jsonField(t, b, "url"), jsonField(t, b, "state")
}

func callback(t *testing.T, ts *httptest.Server, code, state string) (int, []byte) {
	t.Helper()
	return doJSON(t, ts, http.MethodPost, "/v1/auth/oidc/callback", mustJSON(map[string]string{"code": code, "state": state}), "")
}

func TestOIDCHappyPathNamespaceBound(t *testing.T) {
	ts, _, root := newTestServer(t)
	idp := newFakeIdP(t)
	configureOIDC(t, ts, root, idp, "")

	if s, b := doJSON(t, ts, http.MethodPost, "/v1/sys/namespaces", `{"name":"team-a"}`, root); s != http.StatusCreated {
		t.Fatalf("ns: %d %s", s, b)
	}
	nsDo := func(method, path, body, tok, ns string) (int, string) {
		t.Helper()
		resp, err := http.DefaultClient.Do(authedNsReq(t, method, ts.URL+path, body, tok, ns))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	nsDo("PUT", "/v1/policy/rd", `{"rules":[{"path":"secret/**","capabilities":["read"]}]}`, root, "team-a")
	nsDo("PUT", "/v1/secret/prod/pass", `{"value":"A-SECRET"}`, root, "team-a")
	putGroupRole(t, ts, root, "devs", []string{"team-a-devs"}, "rd", "team-a")

	authURL, state := startOIDC(t, ts, idp)
	idp.authorize(t, authURL, "code-1")
	s, b := callback(t, ts, "code-1", state)
	if s != http.StatusOK {
		t.Fatalf("callback: %d %s", s, b)
	}
	tok := jsonField(t, b, "token")

	if s, body := nsDo("GET", "/v1/secret/prod/pass", "", tok, ""); s != 200 || !strings.Contains(body, "A-SECRET") {
		t.Fatalf("token must default to its bound namespace: %d %s", s, body)
	}
	if s, _ := nsDo("GET", "/v1/secret/prod/pass", "", tok, "other"); s != http.StatusForbidden {
		t.Fatalf("cross-namespace header: %d, want 403", s)
	}
}

func TestOIDCStateIsOneTimeAndChecked(t *testing.T) {
	ts, _, root := newTestServer(t)
	idp := newFakeIdP(t)
	configureOIDC(t, ts, root, idp, "")
	createTestPolicy(t, ts, root, "p", `[{"path":"secret/**","capabilities":["read"]}]`)
	putGroupRole(t, ts, root, "devs", []string{"team-a-devs"}, "p", "")

	authURL, state := startOIDC(t, ts, idp)
	idp.authorize(t, authURL, "c1")

	if s, _ := callback(t, ts, "c1", "forged-state"); s != http.StatusBadRequest {
		t.Fatalf("forged state: %d, want 400", s)
	}
	if s, b := callback(t, ts, "c1", state); s != http.StatusOK {
		t.Fatalf("real state: %d %s", s, b)
	}
	// Replay: both the state and the code are spent.
	idp.authorize(t, authURL, "c1")
	if s, _ := callback(t, ts, "c1", state); s != http.StatusBadRequest {
		t.Fatalf("replayed state: %d, want 400", s)
	}
	if s, _ := callback(t, ts, "", ""); s != http.StatusBadRequest {
		t.Fatalf("empty body: %d, want 400", s)
	}
}

func TestOIDCStateConsumedEvenOnFailure(t *testing.T) {
	ts, _, root := newTestServer(t)
	idp := newFakeIdP(t)
	configureOIDC(t, ts, root, idp, "")
	putGroupRole(t, ts, root, "devs", []string{"team-a-devs"}, "root", "")

	authURL, state := startOIDC(t, ts, idp)
	// Code was never issued by the IdP.
	if s, _ := callback(t, ts, "bogus", state); s != http.StatusBadRequest {
		t.Fatalf("bogus code: %d, want 400", s)
	}
	idp.authorize(t, authURL, "late")
	if s, _ := callback(t, ts, "late", state); s != http.StatusBadRequest {
		t.Fatalf("state must be burned after a failed attempt: %d, want 400", s)
	}
}

func TestOIDCPKCEVerifierMismatch(t *testing.T) {
	ts, _, root := newTestServer(t)
	idp := newFakeIdP(t)
	configureOIDC(t, ts, root, idp, "")
	putGroupRole(t, ts, root, "devs", []string{"team-a-devs"}, "root", "")

	_, state := startOIDC(t, ts, idp)
	// A code bound to a different challenge (an attacker's own flow) must not
	// redeem against this state's verifier.
	other, _ := startOIDC(t, ts, idp)
	idp.authorize(t, other, "stolen")
	if s, _ := callback(t, ts, "stolen", state); s != http.StatusBadRequest {
		t.Fatalf("code from another flow: %d, want 400", s)
	}
}

func TestOIDCIDTokenValidation(t *testing.T) {
	cases := map[string]struct {
		mutate func(gojwt.MapClaims)
		want   int
	}{
		"ok":             {func(gojwt.MapClaims) {}, http.StatusOK},
		"wrong nonce":    {func(c gojwt.MapClaims) { c["nonce"] = "attacker" }, http.StatusUnauthorized},
		"missing nonce":  {func(c gojwt.MapClaims) { delete(c, "nonce") }, http.StatusUnauthorized},
		"expired":        {func(c gojwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }, http.StatusUnauthorized},
		"wrong audience": {func(c gojwt.MapClaims) { c["aud"] = "another-client" }, http.StatusUnauthorized},
		"wrong issuer":   {func(c gojwt.MapClaims) { c["iss"] = "https://evil.example.com" }, http.StatusUnauthorized},
		"other group":    {func(c gojwt.MapClaims) { c["groups"] = []string{"marketing"} }, http.StatusForbidden},
		"no groups":      {func(c gojwt.MapClaims) { delete(c, "groups") }, http.StatusForbidden},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ts, _, root := newTestServer(t)
			idp := newFakeIdP(t)
			configureOIDC(t, ts, root, idp, "")
			createTestPolicy(t, ts, root, "p", `[{"path":"secret/**","capabilities":["read"]}]`)
			putGroupRole(t, ts, root, "devs", []string{"team-a-devs"}, "p", "")
			base := idp.claims
			idp.claims = func(n string) gojwt.MapClaims { c := base(n); tc.mutate(c); return c }

			authURL, state := startOIDC(t, ts, idp)
			idp.authorize(t, authURL, "c")
			if s, b := callback(t, ts, "c", state); s != tc.want {
				t.Fatalf("%s: %d %s, want %d", name, s, b, tc.want)
			}
		})
	}
}

func TestOIDCClientSecretSentAndNeverReturned(t *testing.T) {
	ts, _, root := newTestServer(t)
	idp := newFakeIdP(t)
	configureOIDC(t, ts, root, idp, "s3cret-value")
	createTestPolicy(t, ts, root, "p", `[{"path":"secret/**","capabilities":["read"]}]`)
	putGroupRole(t, ts, root, "devs", []string{"team-a-devs"}, "p", "")

	s, b := doJSON(t, ts, http.MethodGet, "/v1/auth/jwt/config", "", root)
	if s != http.StatusOK {
		t.Fatalf("get config: %d %s", s, b)
	}
	if strings.Contains(string(b), "s3cret-value") {
		t.Fatalf("client secret leaked in GET: %s", b)
	}
	if jsonField(t, b, "oidc_client_secret_set") != "true" {
		t.Fatalf("oidc_client_secret_set missing: %s", b)
	}

	// Updating without the secret keeps it; the IdP still sees it.
	upd := map[string]any{
		"jwks_uri": idp.env.jwksURL, "issuer": oidcTestIssuer, "oidc_authorization_url": "http://127.0.0.1:1/authorize",
		"oidc_token_url": idp.srv.URL, "oidc_client_id": oidcTestClient, "oidc_redirect_uri": oidcTestRedirect,
		"oidc_scopes": "openid groups",
	}
	if s, b := doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/config", mustJSON(upd), root); s != http.StatusOK {
		t.Fatalf("update: %d %s", s, b)
	}
	authURL, state := startOIDC(t, ts, idp)
	idp.authorize(t, authURL, "c")
	if s, b := callback(t, ts, "c", state); s != http.StatusOK {
		t.Fatalf("login with kept secret: %d %s", s, b)
	}

	// Explicit empty string clears it.
	upd["oidc_client_secret"] = ""
	if s, b := doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/config", mustJSON(upd), root); s != http.StatusOK {
		t.Fatalf("clear: %d %s", s, b)
	}
	_, b = doJSON(t, ts, http.MethodGet, "/v1/auth/jwt/config", "", root)
	if jsonField(t, b, "oidc_client_secret_set") != "false" {
		t.Fatalf("secret not cleared: %s", b)
	}
}

func TestOIDCConfigValidation(t *testing.T) {
	ts, _, root := newTestServer(t)
	idp := newFakeIdP(t)
	bad := map[string]any{
		"jwks_uri": idp.env.jwksURL, "issuer": oidcTestIssuer,
		"oidc_authorization_url": "http://idp.example.com/authorize", // plain http, non-loopback
		"oidc_token_url":         idp.srv.URL, "oidc_client_id": "x", "oidc_redirect_uri": oidcTestRedirect,
	}
	if s, _ := doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/config", mustJSON(bad), root); s != http.StatusBadRequest {
		t.Fatalf("plain http authorize url: %d, want 400", s)
	}
	delete(bad, "oidc_client_id")
	bad["oidc_authorization_url"] = "https://idp.example.com/authorize"
	if s, _ := doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/config", mustJSON(bad), root); s != http.StatusBadRequest {
		t.Fatalf("missing client id: %d, want 400", s)
	}
}

func TestOIDCStatusAndDisabled(t *testing.T) {
	ts, _, root := newTestServer(t)

	if s, b := doJSON(t, ts, http.MethodGet, "/v1/auth/oidc/status", "", ""); s != http.StatusOK || jsonField(t, b, "enabled") != "false" {
		t.Fatalf("unconfigured status: %d %s", s, b)
	}
	if s, _ := doJSON(t, ts, http.MethodGet, "/v1/auth/oidc/auth-url", "", ""); s != http.StatusServiceUnavailable {
		t.Fatalf("auth-url unconfigured: %d, want 503", s)
	}
	if s, _ := callback(t, ts, "c", "s"); s != http.StatusServiceUnavailable {
		t.Fatalf("callback unconfigured: %d, want 503", s)
	}

	// Plain JWT config without oidc_* leaves the browser flow off.
	env := newJWKSServer(t)
	configureJWT(t, ts, root, env.jwksURL, "")
	if _, b := doJSON(t, ts, http.MethodGet, "/v1/auth/oidc/status", "", ""); jsonField(t, b, "enabled") != "false" {
		t.Fatalf("plain jwt must not enable oidc: %s", b)
	}

	idp := newFakeIdP(t)
	configureOIDC(t, ts, root, idp, "")
	if _, b := doJSON(t, ts, http.MethodGet, "/v1/auth/oidc/status", "", ""); jsonField(t, b, "enabled") != "true" {
		t.Fatalf("configured status: %s", b)
	}
}

// bound_groups also applies to plain JWT login, not only the browser flow.
func TestJWTLoginBoundGroups(t *testing.T) {
	ts, _, root := newTestServer(t)
	env := newJWKSServer(t)
	configureJWT(t, ts, root, env.jwksURL, "")
	createTestPolicy(t, ts, root, "p", `[{"path":"secret/**","capabilities":["read"]}]`)
	putGroupRole(t, ts, root, "devs", []string{"team-a-devs", "ops"}, "p", "")

	sign := func(groups any) string {
		c := gojwt.MapClaims{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()}
		if groups != nil {
			c["groups"] = groups
		}
		return env.sign(t, c)
	}
	if s, b := jwtLogin(t, ts, sign([]string{"x", "ops"})); s != http.StatusOK {
		t.Fatalf("matching group: %d %s", s, b)
	}
	if s, _ := jwtLogin(t, ts, sign([]string{"x"})); s != http.StatusForbidden {
		t.Fatalf("other group: %d, want 403", s)
	}
	if s, _ := jwtLogin(t, ts, sign(nil)); s != http.StatusForbidden {
		t.Fatalf("no groups: %d, want 403", s)
	}

	s, b := doJSON(t, ts, http.MethodGet, "/v1/auth/jwt/role/devs", "", root)
	if s != http.StatusOK || !strings.Contains(string(b), "team-a-devs") {
		t.Fatalf("bound_groups not persisted: %d %s", s, b)
	}
}
