package api

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// --- JWT test helpers ---

// jwtTestEnv holds the RSA key pair and the JWKS server for a test.
type jwtTestEnv struct {
	priv    *rsa.PrivateKey
	jwksURL string
}

// newJWKSServer generates an RSA key pair and starts a local JWKS server.
func newJWKSServer(t *testing.T) *jwtTestEnv {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	pub := &priv.PublicKey
	nB64 := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	eB64 := base64.RawURLEncoding.EncodeToString(eBytes)

	jwksBody, _ := json.Marshal(map[string]any{
		"keys": []map[string]any{{
			"kty": "RSA",
			"kid": "test-key",
			"use": "sig",
			"alg": "RS256",
			"n":   nB64,
			"e":   eB64,
		}},
	})

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksBody)
	}))
	srv.Listener = &rstListener{ln}
	srv.Start()
	t.Cleanup(srv.Close)

	return &jwtTestEnv{priv: priv, jwksURL: srv.URL}
}

// sign signs a JWT with the test RSA key.
func (e *jwtTestEnv) sign(t *testing.T, claims gojwt.MapClaims) string {
	t.Helper()
	tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "test-key"
	s, err := tok.SignedString(e.priv)
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return s
}

// configureJWT calls PUT /v1/auth/jwt/config and fails the test on error.
func configureJWT(t *testing.T, ts *httptest.Server, root, jwksURI, issuer string) {
	t.Helper()
	body := mustJSON(map[string]any{
		"jwks_uri":    jwksURI,
		"issuer":      issuer,
		"default_ttl": "1h",
	})
	status, resp := doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/config", body, root)
	if status != http.StatusOK {
		t.Fatalf("PUT /v1/auth/jwt/config: %d %s", status, resp)
	}
}

// putJWTRole calls PUT /v1/auth/jwt/role/{name} and fails on error.
func putJWTRole(t *testing.T, ts *httptest.Server, root, name, subject string, policies []string, ttl string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"bound_subject": subject,
		"policies":      policies,
		"ttl":           ttl,
	})
	status, resp := doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/role/"+name, string(body), root)
	if status != http.StatusOK {
		t.Fatalf("PUT /v1/auth/jwt/role/%s: %d %s", name, status, resp)
	}
}

// jwtLogin posts a JWT to /v1/auth/jwt/login.
func jwtLogin(t *testing.T, ts *httptest.Server, jwtStr string) (int, []byte) {
	t.Helper()
	body := mustJSON(map[string]string{"jwt": jwtStr})
	return doJSON(t, ts, http.MethodPost, "/v1/auth/jwt/login", body, "")
}

// --- tests ---

// TestJWTFullFlow covers the complete path: configure JWKS → create role →
// sign JWT → login → use the resulting token within policy scope.
func TestJWTFullFlow(t *testing.T) {
	ts, _, root := newTestServer(t)
	env := newJWKSServer(t)

	const issuer = "https://issuer.example.com"
	configureJWT(t, ts, root, env.jwksURL, issuer)

	createTestPolicy(t, ts, root, "jwt-policy",
		`[{"path":"secret/jwt/*","capabilities":["read","write"]}]`)
	putJWTRole(t, ts, root, "app-role", "svc-account", []string{"jwt-policy"}, "1h")

	jwtStr := env.sign(t, gojwt.MapClaims{
		"iss": issuer,
		"sub": "svc-account",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	status, body := jwtLogin(t, ts, jwtStr)
	if status != http.StatusOK {
		t.Fatalf("jwt login: %d %s", status, body)
	}
	tok := jsonField(t, body, "token")
	if tok == "" {
		t.Fatalf("token empty, body: %s", body)
	}

	// Token works within policy scope.
	status, _ = doJSON(t, ts, http.MethodPut, "/v1/secret/jwt/cred", `"secret"`, tok)
	if status != http.StatusNoContent {
		t.Errorf("write: %d, want 204", status)
	}
	status, body = doJSON(t, ts, http.MethodGet, "/v1/secret/jwt/cred", "", tok)
	if status != http.StatusOK {
		t.Errorf("read: %d %s, want 200", status, body)
	}

	// Token is denied outside policy scope.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/secret/other/key", "", tok)
	if status != http.StatusForbidden {
		t.Errorf("out-of-scope: %d, want 403", status)
	}
}

// TestJWTTokenRenewable verifies that JWT-issued tokens carry renewable=true.
func TestJWTTokenRenewable(t *testing.T) {
	ts, _, root := newTestServer(t)
	env := newJWKSServer(t)

	configureJWT(t, ts, root, env.jwksURL, "")
	putJWTRole(t, ts, root, "r", "sub1", []string{"root"}, "500ms")

	jwtStr := env.sign(t, gojwt.MapClaims{
		"sub": "sub1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	status, body := jwtLogin(t, ts, jwtStr)
	if status != http.StatusOK {
		t.Fatalf("login: %d %s", status, body)
	}
	tok := jsonField(t, body, "token")

	// lookup-self must show renewable=true.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/auth/token/lookup-self", "", tok)
	if status != http.StatusOK {
		t.Fatalf("lookup-self: %d %s", status, body)
	}
	if jsonField(t, body, "renewable") != "true" {
		t.Errorf("JWT token renewable=%s, want true", jsonField(t, body, "renewable"))
	}

	// Renew while still valid.
	renewBody := mustJSON(map[string]string{"ttl": "10s"})
	status, body = doJSON(t, ts, http.MethodPost, "/v1/auth/token/renew-self", renewBody, tok)
	if status != http.StatusOK {
		t.Fatalf("renew-self: %d %s, want 200", status, body)
	}
}

// TestJWTBoundClaims verifies that bound_claims matching works end-to-end.
func TestJWTBoundClaims(t *testing.T) {
	ts, _, root := newTestServer(t)
	env := newJWKSServer(t)

	configureJWT(t, ts, root, env.jwksURL, "")

	// Role requires tenant=acme in JWT claims.
	body, _ := json.Marshal(map[string]any{
		"bound_claims": map[string]string{"tenant": "acme"},
		"policies":     []string{"root"},
		"ttl":          "1h",
	})
	status, resp := doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/role/acme-role", string(body), root)
	if status != http.StatusOK {
		t.Fatalf("create role: %d %s", status, resp)
	}

	// JWT with correct tenant → success.
	okJWT := env.sign(t, gojwt.MapClaims{
		"sub":    "u1",
		"tenant": "acme",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	status, resp = jwtLogin(t, ts, okJWT)
	if status != http.StatusOK {
		t.Errorf("correct tenant: %d %s, want 200", status, resp)
	}

	// JWT with wrong tenant → 403.
	badJWT := env.sign(t, gojwt.MapClaims{
		"sub":    "u1",
		"tenant": "other",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	status, _ = jwtLogin(t, ts, badJWT)
	if status != http.StatusForbidden {
		t.Errorf("wrong tenant: %d, want 403", status)
	}
}

// TestJWTExpiredToken verifies 401 on an expired JWT.
func TestJWTExpiredToken(t *testing.T) {
	ts, _, root := newTestServer(t)
	env := newJWKSServer(t)

	configureJWT(t, ts, root, env.jwksURL, "")
	putJWTRole(t, ts, root, "r", "u1", []string{"root"}, "1h")

	expiredJWT := env.sign(t, gojwt.MapClaims{
		"sub": "u1",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	status, _ := jwtLogin(t, ts, expiredJWT)
	if status != http.StatusUnauthorized {
		t.Errorf("expired JWT: %d, want 401", status)
	}
}

// TestJWTNoMatchingRole verifies 403 when no role matches the JWT subject.
func TestJWTNoMatchingRole(t *testing.T) {
	ts, _, root := newTestServer(t)
	env := newJWKSServer(t)

	configureJWT(t, ts, root, env.jwksURL, "")
	putJWTRole(t, ts, root, "r", "expected-sub", []string{"root"}, "1h")

	jwtStr := env.sign(t, gojwt.MapClaims{
		"sub": "different-sub",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	status, _ := jwtLogin(t, ts, jwtStr)
	if status != http.StatusForbidden {
		t.Errorf("no matching role: %d, want 403", status)
	}
}

// TestJWTNotConfigured verifies 503 when JWT auth has not been configured.
func TestJWTNotConfigured(t *testing.T) {
	ts, _, _ := newTestServer(t)
	// No config — must return 503.
	body := mustJSON(map[string]string{"jwt": "header.payload.sig"})
	status, _ := doJSON(t, ts, http.MethodPost, "/v1/auth/jwt/login", body, "")
	if status != http.StatusServiceUnavailable {
		t.Errorf("not configured: %d, want 503", status)
	}
}

// TestJWTMissingField verifies 400 when the jwt field is absent.
func TestJWTMissingField(t *testing.T) {
	ts, _, _ := newTestServer(t)
	status, _ := doJSON(t, ts, http.MethodPost, "/v1/auth/jwt/login", `{}`, "")
	if status != http.StatusBadRequest {
		t.Errorf("missing jwt field: %d, want 400", status)
	}
}

// TestJWTRoleCRUD exercises role management endpoints.
func TestJWTRoleCRUD(t *testing.T) {
	ts, _, root := newTestServer(t)
	env := newJWKSServer(t)

	configureJWT(t, ts, root, env.jwksURL, "")

	// Create.
	putJWTRole(t, ts, root, "myrole", "svc", []string{"root"}, "2h")

	// Read.
	status, body := doJSON(t, ts, http.MethodGet, "/v1/auth/jwt/role/myrole", "", root)
	if status != http.StatusOK {
		t.Fatalf("GET role: %d %s", status, body)
	}
	if !strings.Contains(string(body), "myrole") {
		t.Errorf("GET role missing name: %s", body)
	}

	// List.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/auth/jwt/role/?list=true", "", root)
	if status != http.StatusOK {
		t.Fatalf("LIST roles: %d %s", status, body)
	}
	if !strings.Contains(string(body), "myrole") {
		t.Errorf("LIST missing myrole: %s", body)
	}

	// Delete.
	status, _ = doJSON(t, ts, http.MethodDelete, "/v1/auth/jwt/role/myrole", "", root)
	if status != http.StatusNoContent {
		t.Errorf("DELETE role: %d, want 204", status)
	}

	// Must be gone.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/auth/jwt/role/myrole", "", root)
	if status != http.StatusNotFound {
		t.Errorf("GET deleted role: %d, want 404", status)
	}
}

// TestJWTWrongIssuer verifies 401 when the JWT issuer doesn't match config.
func TestJWTWrongIssuer(t *testing.T) {
	ts, _, root := newTestServer(t)
	env := newJWKSServer(t)

	configureJWT(t, ts, root, env.jwksURL, "https://expected-issuer.com")
	putJWTRole(t, ts, root, "r", "u1", []string{"root"}, "1h")

	jwtStr := env.sign(t, gojwt.MapClaims{
		"iss": "https://wrong-issuer.com",
		"sub": "u1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	status, _ := jwtLogin(t, ts, jwtStr)
	if status != http.StatusUnauthorized {
		t.Errorf("wrong issuer: %d, want 401", status)
	}
}

// TestJWTBoundAudiences verifies audience matching end-to-end.
func TestJWTBoundAudiences(t *testing.T) {
	ts, _, root := newTestServer(t)
	env := newJWKSServer(t)

	configureJWT(t, ts, root, env.jwksURL, "")

	body, _ := json.Marshal(map[string]any{
		"bound_audiences": []string{"tuck-api"},
		"policies":        []string{"root"},
		"ttl":             "1h",
	})
	status, resp := doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/role/aud-role", string(body), root)
	if status != http.StatusOK {
		t.Fatalf("create role: %d %s", status, resp)
	}

	// JWT with correct audience → 200.
	okJWT := env.sign(t, gojwt.MapClaims{
		"sub": "u1",
		"aud": "tuck-api",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	status, resp = jwtLogin(t, ts, okJWT)
	if status != http.StatusOK {
		t.Errorf("correct aud: %d %s, want 200", status, resp)
	}

	// JWT with wrong audience → 403.
	badJWT := env.sign(t, gojwt.MapClaims{
		"sub": "u1",
		"aud": "other-api",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	status, _ = jwtLogin(t, ts, badJWT)
	if status != http.StatusForbidden {
		t.Errorf("wrong aud: %d, want 403", status)
	}
}
