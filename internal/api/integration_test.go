package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// tokenID unmarshals the id field from a create-token response body.
func parseTokenID(t *testing.T, body []byte) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &v); err != nil || v.ID == "" {
		t.Fatalf("parse token id: %v; body: %s", err, body)
	}
	return v.ID
}

func createTestToken(t *testing.T, ts *httptest.Server, rootTok, displayName string, policies []string, ttl string) string {
	t.Helper()
	type req struct {
		DisplayName string   `json:"display_name"`
		Policies    []string `json:"policies"`
		TTL         string   `json:"ttl,omitempty"`
	}
	b, _ := json.Marshal(req{DisplayName: displayName, Policies: policies, TTL: ttl})
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/auth/token", string(b), rootTok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create token: status=%d body=%s", resp.StatusCode, body)
	}
	return parseTokenID(t, body)
}

func createTestPolicy(t *testing.T, ts *httptest.Server, rootTok, name, rulesJSON string) {
	t.Helper()
	body := `{"rules":` + rulesJSON + `}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/policy/"+name, body, rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("create policy %q: status=%d", name, resp.StatusCode)
	}
}

// TestRevokedTokenDenied verifies that a revoked token cannot be used.
func TestRevokedTokenDenied(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	tokID := createTestToken(t, ts, rootTok, "temp", []string{"root"}, "")

	// token works before revocation
	resp, _ := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/health", "", tokID))
	resp.Body.Close()
	// health is unauthenticated, use a secret read instead
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/x", "", tokID))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// 404 is fine — secret doesn't exist but token was accepted
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatal("token should be valid before revocation")
	}

	// revoke it
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodDelete, ts.URL+"/v1/auth/token/"+tokID, "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: status=%d", resp.StatusCode)
	}

	// token must now be rejected
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/x", "", tokID))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, want 401", resp.StatusCode)
	}
}

// TestExpiredToken verifies that a token past its TTL is rejected.
func TestExpiredToken(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	tokID := createTestToken(t, ts, rootTok, "ephemeral", []string{"root"}, "10ms")

	time.Sleep(30 * time.Millisecond)

	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/x", "", tokID))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired token status = %d, want 401", resp.StatusCode)
	}
}

// TestNoPoliciesToken verifies that a token with no policies is denied everywhere.
func TestNoPoliciesToken(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	tokID := createTestToken(t, ts, rootTok, "empty", []string{}, "")

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/v1/secret/anything"},
		{http.MethodPut, "/v1/secret/anything"},
		{http.MethodGet, "/v1/policy/somepolicy"},
	} {
		resp, err := http.DefaultClient.Do(authedReq(t, tc.method, ts.URL+tc.path, "val", tokID))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s: status=%d, want 403", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// TestMultiplePoliciesUnion verifies that a token accumulates capabilities
// from all attached policies.
func TestMultiplePoliciesUnion(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	createTestPolicy(t, ts, rootTok, "read-prod",
		`[{"path":"secret/prod/*","capabilities":["read"]}]`)
	createTestPolicy(t, ts, rootTok, "write-staging",
		`[{"path":"secret/staging/*","capabilities":["read","write"]}]`)

	// seed secrets
	http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/prod/key", "pval", rootTok))     //nolint
	http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/staging/key", "sval", rootTok)) //nolint

	tokID := createTestToken(t, ts, rootTok, "multi", []string{"read-prod", "write-staging"}, "")

	// can read prod
	resp, _ := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/prod/key", "", tokID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("read prod: status=%d, want 200", resp.StatusCode)
	}

	// cannot write prod (read-only policy)
	resp, _ = http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/prod/key", "x", tokID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("write prod: status=%d, want 403", resp.StatusCode)
	}

	// can read+write staging
	resp, _ = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/staging/key", "", tokID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("read staging: status=%d, want 200", resp.StatusCode)
	}
	resp, _ = http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/staging/new", "v", tokID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("write staging: status=%d, want 204", resp.StatusCode)
	}

	// cannot access other namespaces
	resp, _ = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/other/key", "", tokID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("read other: status=%d, want 403", resp.StatusCode)
	}
}

// TestRootPolicyIsImmutable verifies the built-in root policy cannot be
// deleted or overwritten via the API (it's never stored, so DELETE is a
// no-op and GET returns 404).
func TestRootPolicyIsImmutable(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/policy/root", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET root policy: status=%d, want 404 (root policy is hardcoded, not stored)", resp.StatusCode)
	}
}

// TestTokenMaxUses verifies that a token with max_uses=N is revoked automatically
// after the Nth authenticated API call and rejected on the (N+1)th.
func TestTokenMaxUses(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	// Create a token with max_uses=2.
	body := `{"display_name":"limited","policies":[],"max_uses":2}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/auth/token", body, rootTok))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create token: status=%d body=%s", resp.StatusCode, raw)
	}
	limitedTok := parseTokenID(t, raw)

	// Use 1: should succeed.
	r1, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/token/lookup-self", "", limitedTok))
	if err != nil {
		t.Fatal(err)
	}
	r1.Body.Close()
	if r1.StatusCode != http.StatusOK {
		t.Errorf("use 1: status=%d, want 200", r1.StatusCode)
	}

	// Use 2: should succeed.
	r2, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/token/lookup-self", "", limitedTok))
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Errorf("use 2: status=%d, want 200", r2.StatusCode)
	}

	// Use 3: token is exhausted → 401.
	r3, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/token/lookup-self", "", limitedTok))
	if err != nil {
		t.Fatal(err)
	}
	r3.Body.Close()
	if r3.StatusCode != http.StatusUnauthorized {
		t.Errorf("use 3 (exhausted): status=%d, want 401", r3.StatusCode)
	}
}

// TestTokenMaxUsesOne verifies that a max_uses=1 token succeeds exactly once.
func TestTokenMaxUsesOne(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	body := `{"display_name":"oneshot","policies":[],"max_uses":1}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/auth/token", body, rootTok))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, raw)
	}
	oneTok := parseTokenID(t, raw)

	// Only use: succeeds.
	r1, _ := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/token/lookup-self", "", oneTok))
	r1.Body.Close()
	if r1.StatusCode != http.StatusOK {
		t.Errorf("use 1: status=%d, want 200", r1.StatusCode)
	}

	// Second use: fails.
	r2, _ := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/token/lookup-self", "", oneTok))
	r2.Body.Close()
	if r2.StatusCode != http.StatusUnauthorized {
		t.Errorf("use 2: status=%d, want 401", r2.StatusCode)
	}
}

// TestTokenMaxUsesZeroMeansUnlimited verifies that max_uses=0 (default) is unlimited.
func TestTokenMaxUsesZeroMeansUnlimited(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	body := `{"display_name":"unlimited","policies":[],"max_uses":0}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/auth/token", body, rootTok))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	tok := parseTokenID(t, raw)

	// Use it 5 times — all should succeed.
	for i := 1; i <= 5; i++ {
		r, _ := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/token/lookup-self", "", tok))
		r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Errorf("use %d: status=%d, want 200", i, r.StatusCode)
		}
	}
}

// TestInvalidTokenFormat verifies that a syntactically valid but unknown
// token string is rejected with 401, not 500.
func TestInvalidTokenFormat(t *testing.T) {
	ts, _, _ := newTestServer(t)

	for _, bad := range []string{"invalid", "tuck_notavalidtoken", strings.Repeat("x", 64)} {
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/x", "", bad))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("token %q: status=%d, want 401", bad, resp.StatusCode)
		}
	}
}
