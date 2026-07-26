package api

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- helpers ---

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func doJSON(t *testing.T, ts *httptest.Server, method, urlPath, body, tok string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL+urlPath, r)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "" {
		req.Header.Set("X-Tuck-Token", tok)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode, b
}

func jsonField(t *testing.T, body []byte, field string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse JSON: %v; body: %s", err, body)
	}
	v, ok := m[field]
	if !ok {
		t.Fatalf("field %q not found in: %s", field, body)
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strings.TrimRight(strings.TrimRight(string(mustJSON(x)), "0"), ".")
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// ---

// TestAppRoleFullFlow covers the complete AppRole auth cycle:
// create role → generate secret-id → login → use the resulting token.
func TestAppRoleFullFlow(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Create a policy for the role.
	createTestPolicy(t, ts, root, "ar-policy",
		`[{"path":"secret/app/*","capabilities":["read","write","delete"]}]`)

	// Create the AppRole.
	status, body := doJSON(t, ts, http.MethodPut, "/v1/auth/approle/role/myapp",
		`{"policies":["ar-policy"],"token_ttl":"1h"}`, root)
	if status != http.StatusOK {
		t.Fatalf("create role: %d %s", status, body)
	}

	// Read back the role and get the role_id.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/auth/approle/role/myapp", "", root)
	if status != http.StatusOK {
		t.Fatalf("get role: %d %s", status, body)
	}
	roleID := jsonField(t, body, "role_id")
	if roleID == "" {
		t.Fatalf("role_id is empty, body: %s", body)
	}

	// Generate a secret-id (response field is "id", not "secret_id").
	status, body = doJSON(t, ts, http.MethodPost, "/v1/auth/approle/role/myapp/secret-id", "", root)
	if status != http.StatusOK {
		t.Fatalf("gen secret-id: %d %s", status, body)
	}
	secretID := jsonField(t, body, "id")
	if secretID == "" {
		t.Fatalf("secret id is empty, body: %s", body)
	}

	// Login with role_id + secret_id.
	loginBody := mustJSON(map[string]string{"role_id": roleID, "secret_id": secretID})
	status, body = doJSON(t, ts, http.MethodPost, "/v1/auth/approle/login", loginBody, "")
	if status != http.StatusOK {
		t.Fatalf("approle login: %d %s", status, body)
	}
	appTok := jsonField(t, body, "token")
	if appTok == "" {
		t.Fatalf("token is empty, body: %s", body)
	}

	// Use the token to write and read a secret.
	status, _ = doJSON(t, ts, http.MethodPut, "/v1/secret/app/db-pass", `secret123`, appTok)
	if status != http.StatusNoContent {
		t.Errorf("write with approle token: %d, want 204", status)
	}
	status, body = doJSON(t, ts, http.MethodGet, "/v1/secret/app/db-pass", "", appTok)
	if status != http.StatusOK {
		t.Errorf("read with approle token: %d, want 200; body: %s", status, body)
	}
	if !strings.Contains(string(body), "secret123") {
		t.Errorf("read body %s does not contain written value", body)
	}

	// Token must be denied outside its policy scope.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/secret/other/key", "", appTok)
	if status != http.StatusForbidden {
		t.Errorf("out-of-scope read: %d, want 403", status)
	}
}

// TestAppRoleSecretIDNumUses verifies that a secret-id with num_uses=1 is
// consumed after the first login and rejected on the second.
func TestAppRoleSecretIDNumUses(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPut, "/v1/auth/approle/role/onetime",
		`{"policies":["root"],"secret_id_num_uses":1}`, root)

	status, body := doJSON(t, ts, http.MethodGet, "/v1/auth/approle/role/onetime", "", root)
	if status != http.StatusOK {
		t.Fatalf("get role: %d", status)
	}
	roleID := jsonField(t, body, "role_id")

	status, body = doJSON(t, ts, http.MethodPost, "/v1/auth/approle/role/onetime/secret-id", "", root)
	if status != http.StatusOK {
		t.Fatalf("gen secret-id: %d", status)
	}
	secretID := jsonField(t, body, "id")

	loginPayload := mustJSON(map[string]string{"role_id": roleID, "secret_id": secretID})

	// First login: succeeds.
	status, _ = doJSON(t, ts, http.MethodPost, "/v1/auth/approle/login", loginPayload, "")
	if status != http.StatusOK {
		t.Fatalf("first login: %d, want 200", status)
	}

	// Second login: secret-id exhausted → 401.
	status, _ = doJSON(t, ts, http.MethodPost, "/v1/auth/approle/login", loginPayload, "")
	if status != http.StatusUnauthorized {
		t.Errorf("second login with consumed secret-id: %d, want 401", status)
	}
}

// TestKVv2Versioning exercises the full KV v2 write/read/version cycle.
func TestKVv2Versioning(t *testing.T) {
	ts, _, root := newTestServer(t)

	write := func(val string) int {
		t.Helper()
		status, body := doJSON(t, ts, http.MethodPut, "/v2/secret/versioned/key", val, root)
		if status != http.StatusOK {
			t.Fatalf("write %q: %d %s", val, status, body)
		}
		var resp struct {
			Version int `json:"version"`
		}
		_ = json.Unmarshal(body, &resp)
		return resp.Version
	}

	v1 := write("value-one")
	v2 := write("value-two")
	v3 := write("value-three")

	if v1 != 1 || v2 != 2 || v3 != 3 {
		t.Fatalf("versions: got %d/%d/%d, want 1/2/3", v1, v2, v3)
	}

	// Current (v3) read.
	status, body := doJSON(t, ts, http.MethodGet, "/v2/secret/versioned/key", "", root)
	if status != http.StatusOK {
		t.Fatalf("read current: %d %s", status, body)
	}
	if !strings.Contains(string(body), "value-three") {
		t.Errorf("current read: %s does not contain value-three", body)
	}

	// Pinned version=1 read.
	status, body = doJSON(t, ts, http.MethodGet, "/v2/secret/versioned/key?version=1", "", root)
	if status != http.StatusOK {
		t.Fatalf("read v1: %d %s", status, body)
	}
	if !strings.Contains(string(body), "value-one") {
		t.Errorf("v1 read: %s does not contain value-one", body)
	}

	// Soft-delete v2 — note: query param is "versions" (plural).
	status, _ = doJSON(t, ts, http.MethodDelete, "/v2/secret/versioned/key?versions=2", "", root)
	if status != http.StatusNoContent {
		t.Errorf("delete v2: %d, want 204", status)
	}

	// Soft-deleted version still readable but response carries "deleted":true.
	status, body = doJSON(t, ts, http.MethodGet, "/v2/secret/versioned/key?version=2", "", root)
	if status != http.StatusOK {
		t.Errorf("read soft-deleted v2: %d, want 200 (with deleted flag)", status)
	}
	if !strings.Contains(string(body), `"deleted":true`) && !strings.Contains(string(body), `"deleted": true`) {
		t.Errorf("soft-deleted v2 body missing deleted flag: %s", body)
	}

	// Undelete v2.
	status, _ = doJSON(t, ts, http.MethodPost, "/v2/secret/undelete/versioned/key",
		`{"versions":[2]}`, root)
	if status != http.StatusNoContent {
		t.Errorf("undelete v2: %d, want 204", status)
	}

	// v2 must be back.
	status, body = doJSON(t, ts, http.MethodGet, "/v2/secret/versioned/key?version=2", "", root)
	if status != http.StatusOK {
		t.Errorf("read restored v2: %d %s, want 200", status, body)
	}
	if !strings.Contains(string(body), "value-two") {
		t.Errorf("restored v2: %s does not contain value-two", body)
	}
}

// TestKVv2CAS verifies the check-and-set (optimistic locking) mechanism.
func TestKVv2CAS(t *testing.T) {
	ts, _, root := newTestServer(t)

	// First write (no CAS) → version 1.
	status, _ := doJSON(t, ts, http.MethodPut, "/v2/secret/cas/key", "initial", root)
	if status != http.StatusOK {
		t.Fatalf("initial write: %d", status)
	}

	// CAS=0 must be rejected (key exists, current version is 1).
	status, _ = doJSON(t, ts, http.MethodPut, "/v2/secret/cas/key?cas=0", "bad", root)
	if status != http.StatusConflict {
		t.Errorf("cas=0 on existing key: %d, want 409", status)
	}

	// CAS=1 must succeed → version 2.
	status, body := doJSON(t, ts, http.MethodPut, "/v2/secret/cas/key?cas=1", "updated", root)
	if status != http.StatusOK {
		t.Errorf("cas=1: %d %s, want 200", status, body)
	}

	// Verify the update landed.
	status, body = doJSON(t, ts, http.MethodGet, "/v2/secret/cas/key", "", root)
	if status != http.StatusOK {
		t.Fatalf("read after cas: %d", status)
	}
	if !strings.Contains(string(body), "updated") {
		t.Errorf("cas update: %s does not contain 'updated'", body)
	}
}

// TestCubbyholeIsolation verifies that each token has its own private cubbyhole
// and cannot read another token's data.
func TestCubbyholeIsolation(t *testing.T) {
	ts, _, root := newTestServer(t)

	tokA := createTestToken(t, ts, root, "alice", []string{"root"}, "")
	tokB := createTestToken(t, ts, root, "bob", []string{"root"}, "")

	// Token A writes to its cubbyhole (body must be a JSON object).
	status, _ := doJSON(t, ts, http.MethodPut, "/v1/cubbyhole/secret",
		`{"value":"alice-secret"}`, tokA)
	if status != http.StatusNoContent {
		t.Fatalf("A write cubbyhole: %d", status)
	}

	// Token B cannot see Token A's cubbyhole — gets 404 (empty namespace).
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/cubbyhole/secret", "", tokB)
	if status != http.StatusNotFound {
		t.Errorf("B reads A cubbyhole: %d, want 404", status)
	}

	// Token A can read its own cubbyhole.
	status, body := doJSON(t, ts, http.MethodGet, "/v1/cubbyhole/secret", "", tokA)
	if status != http.StatusOK {
		t.Errorf("A reads own cubbyhole: %d, want 200", status)
	}
	if !strings.Contains(string(body), "alice-secret") {
		t.Errorf("A cubbyhole body %s does not contain alice-secret", body)
	}

	// Token B writes its own entry.
	status, _ = doJSON(t, ts, http.MethodPut, "/v1/cubbyhole/secret",
		`{"value":"bob-secret"}`, tokB)
	if status != http.StatusNoContent {
		t.Fatalf("B write cubbyhole: %d", status)
	}

	// Token A still sees its own value, not Bob's.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/cubbyhole/secret", "", tokA)
	if status != http.StatusOK {
		t.Fatalf("A reads own cubbyhole after B write: %d", status)
	}
	if strings.Contains(string(body), "bob-secret") {
		t.Errorf("A cubbyhole contaminated with bob's data: %s", body)
	}
	if !strings.Contains(string(body), "alice-secret") {
		t.Errorf("A cubbyhole lost alice's data: %s", body)
	}
}

// TestResponseWrappingOneTime verifies that a wrapping token can be unwrapped
// exactly once and is invalidated afterwards.
func TestResponseWrappingOneTime(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Wrap a payload.
	wrapBody := `{"data":{"secret_key":"super-secret","env":"prod"},"ttl":"5m"}`
	status, body := doJSON(t, ts, http.MethodPost, "/v1/sys/wrapping/wrap", wrapBody, root)
	if status != http.StatusOK {
		t.Fatalf("wrap: %d %s", status, body)
	}
	wrapTok := jsonField(t, body, "token")
	if wrapTok == "" {
		t.Fatalf("wrap token empty, body: %s", body)
	}

	// First unwrap: succeeds.
	unwrapBody := mustJSON(map[string]string{"token": wrapTok})
	status, body = doJSON(t, ts, http.MethodPost, "/v1/sys/wrapping/unwrap", unwrapBody, root)
	if status != http.StatusOK {
		t.Fatalf("first unwrap: %d %s", status, body)
	}
	if !strings.Contains(string(body), "super-secret") {
		t.Errorf("unwrap body %s does not contain original payload", body)
	}

	// Second unwrap: token already consumed → 404.
	status, _ = doJSON(t, ts, http.MethodPost, "/v1/sys/wrapping/unwrap", unwrapBody, root)
	if status != http.StatusNotFound {
		t.Errorf("second unwrap: %d, want 404 (already consumed)", status)
	}
}

// TestWrappingLookupAndRevoke verifies lookup metadata and explicit revocation.
func TestWrappingLookupAndRevoke(t *testing.T) {
	ts, _, root := newTestServer(t)

	status, body := doJSON(t, ts, http.MethodPost, "/v1/sys/wrapping/wrap",
		`{"data":{"x":"y"},"ttl":"10m"}`, root)
	if status != http.StatusOK {
		t.Fatalf("wrap: %d %s", status, body)
	}
	wrapTok := jsonField(t, body, "token")

	// Lookup must return creation metadata.
	lookupBody := mustJSON(map[string]string{"token": wrapTok})
	status, body = doJSON(t, ts, http.MethodPost, "/v1/sys/wrapping/lookup", lookupBody, root)
	if status != http.StatusOK {
		t.Fatalf("lookup: %d %s", status, body)
	}
	if !strings.Contains(string(body), "expires_at") {
		t.Errorf("lookup response missing expires_at: %s", body)
	}

	// Explicit revocation.
	status, _ = doJSON(t, ts, http.MethodDelete, "/v1/sys/wrapping/revoke", lookupBody, root)
	if status != http.StatusNoContent {
		t.Errorf("revoke: %d, want 204", status)
	}

	// After revocation, lookup must return 404.
	status, _ = doJSON(t, ts, http.MethodPost, "/v1/sys/wrapping/lookup", lookupBody, root)
	if status != http.StatusNotFound {
		t.Errorf("lookup after revoke: %d, want 404", status)
	}
}

// TestTokenRenewal verifies that a token's TTL can be extended via renewal
// before it expires.
func TestTokenRenewal(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Create a renewable token with 500ms TTL.
	b, _ := json.Marshal(map[string]any{
		"display_name": "renewable",
		"policies":     []string{"root"},
		"ttl":          "500ms",
		"renewable":    true,
		"max_ttl":      "1h",
	})
	status, respBody := doJSON(t, ts, http.MethodPost, "/v1/auth/token", string(b), root)
	if status != http.StatusCreated {
		t.Fatalf("create renewable token: %d %s", status, respBody)
	}
	tokID := jsonField(t, respBody, "id")

	// Token is valid immediately.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/auth/token/lookup-self", "", tokID)
	if status != http.StatusOK {
		t.Fatalf("pre-renewal lookup: %d, want 200", status)
	}

	// Renew with an additional 10s.
	renewBody := mustJSON(map[string]string{"increment": "10s"})
	status, body := doJSON(t, ts, http.MethodPost, "/v1/auth/token/"+tokID+"/renew", renewBody, root)
	if status != http.StatusOK {
		t.Fatalf("renew: %d %s, want 200", status, body)
	}

	// Wait past the original TTL.
	time.Sleep(600 * time.Millisecond)

	// Token should still be valid after renewal.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/auth/token/lookup-self", "", tokID)
	if status != http.StatusOK {
		t.Errorf("post-renewal lookup after original TTL: %d, want 200", status)
	}
}

// TestTokenAccessorOps verifies lookup-by-accessor and revoke-by-accessor.
func TestTokenAccessorOps(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Create a token and extract its accessor from the creation response.
	b, _ := json.Marshal(map[string]any{
		"display_name": "accessor-test",
		"policies":     []string{"root"},
	})
	status, body := doJSON(t, ts, http.MethodPost, "/v1/auth/token", string(b), root)
	if status != http.StatusCreated {
		t.Fatalf("create token: %d %s", status, body)
	}
	tokID := jsonField(t, body, "id")
	accessor := jsonField(t, body, "accessor")
	if accessor == "" {
		t.Fatalf("accessor empty, body: %s", body)
	}

	// Lookup by accessor.
	lookupBody := mustJSON(map[string]string{"accessor": accessor})
	status, body = doJSON(t, ts, http.MethodPost, "/v1/auth/token/lookup-accessor", lookupBody, root)
	if status != http.StatusOK {
		t.Fatalf("lookup-accessor: %d %s", status, body)
	}
	if jsonField(t, body, "id") != tokID {
		t.Errorf("lookup-accessor returned wrong token id: %s", body)
	}

	// Revoke by accessor.
	status, _ = doJSON(t, ts, http.MethodDelete, "/v1/auth/token/revoke-accessor", lookupBody, root)
	if status != http.StatusNoContent {
		t.Errorf("revoke-accessor: %d, want 204", status)
	}

	// Token must now be invalid.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/auth/token/lookup-self", "", tokID)
	if status != http.StatusUnauthorized {
		t.Errorf("revoked token: %d, want 401", status)
	}
}

// TestKVListHierarchy verifies that LIST returns logical folder structure.
func TestKVListHierarchy(t *testing.T) {
	ts, _, root := newTestServer(t)

	for _, path := range []string{
		"/v1/secret/myapp/db/password",
		"/v1/secret/myapp/db/username",
		"/v1/secret/myapp/api/key",
		"/v1/secret/myapp/cache/redis",
	} {
		status, _ := doJSON(t, ts, http.MethodPut, path, "val", root)
		if status != http.StatusNoContent {
			t.Fatalf("PUT %s: %d", path, status)
		}
	}

	// LIST /v1/secret/myapp/ — should return "db/", "api/", "cache/"
	req, _ := http.NewRequest("LIST", ts.URL+"/v1/secret/myapp/", nil)
	req.Header.Set("X-Tuck-Token", root)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("LIST myapp/: %d %s", resp.StatusCode, body)
	}
	for _, want := range []string{"db/", "api/", "cache/"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("LIST myapp/ missing %q; body: %s", want, body)
		}
	}

	// LIST /v1/secret/myapp/db/ — should return "password", "username"
	req, _ = http.NewRequest("LIST", ts.URL+"/v1/secret/myapp/db/", nil)
	req.Header.Set("X-Tuck-Token", root)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("LIST myapp/db/: %d %s", resp.StatusCode, body)
	}
	for _, want := range []string{"password", "username"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("LIST myapp/db/ missing %q; body: %s", want, body)
		}
	}
}

// TestChildTokenRevocationCascade verifies that revoking a parent token also
// invalidates all tokens it created (the child subtree).
func TestChildTokenRevocationCascade(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Create parent.
	parent := createTestToken(t, ts, root, "parent", []string{"root"}, "")

	// Parent creates a child.
	child := createTestToken(t, ts, parent, "child", []string{"root"}, "")

	// Child creates a grandchild.
	grandchild := createTestToken(t, ts, child, "grandchild", []string{"root"}, "")

	// All three are valid.
	for name, tok := range map[string]string{"parent": parent, "child": child, "grandchild": grandchild} {
		status, _ := doJSON(t, ts, http.MethodGet, "/v1/auth/token/lookup-self", "", tok)
		if status != http.StatusOK {
			t.Errorf("%s pre-revoke: %d, want 200", name, status)
		}
	}

	// Revoke the parent.
	status, _ := doJSON(t, ts, http.MethodDelete, "/v1/auth/token/"+parent, "", root)
	if status != http.StatusNoContent {
		t.Fatalf("revoke parent: %d, want 204", status)
	}

	// All three must now be invalid.
	for name, tok := range map[string]string{"parent": parent, "child": child, "grandchild": grandchild} {
		status, _ := doJSON(t, ts, http.MethodGet, "/v1/auth/token/lookup-self", "", tok)
		if status != http.StatusUnauthorized {
			t.Errorf("%s after parent revoke: %d, want 401", name, status)
		}
	}
}

// TestPKIFullFlow covers the PKI engine: generate a root CA, define a role,
// issue a certificate, verify it parses, then revoke it and check the CRL.
func TestPKIFullFlow(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Generate root CA.
	caPayload := mustJSON(map[string]any{
		"common_name": "tuck-test-ca",
		"ttl":         "87600h",
		"key_type":    "ec",
		"key_bits":    256,
	})
	status, body := doJSON(t, ts, http.MethodPost, "/v1/pki/generate/root", caPayload, root)
	if status != http.StatusOK {
		t.Fatalf("generate root CA: %d %s", status, body)
	}

	// GET /v1/pki/ca/pem — returns JSON {"certificate":"-----BEGIN CERTIFICATE-----\n..."}.
	status, caBody := doJSON(t, ts, http.MethodGet, "/v1/pki/ca/pem", "", "")
	if status != http.StatusOK {
		t.Fatalf("get CA PEM: %d %s", status, caBody)
	}
	caPEMStr := jsonField(t, caBody, "certificate")
	block, _ := pem.Decode([]byte(caPEMStr))
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("CA PEM is not a CERTIFICATE block; got: %s", caPEMStr[:min(len(caPEMStr), 100)])
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}
	if caCert.Subject.CommonName != "tuck-test-ca" {
		t.Errorf("CA CN = %q, want tuck-test-ca", caCert.Subject.CommonName)
	}

	// Create a PKI role.
	rolePayload := mustJSON(map[string]any{
		"allowed_domains":  []string{"example.com"},
		"allow_subdomains": true,
		"max_ttl":          "8760h",
	})
	status, body = doJSON(t, ts, http.MethodPut, "/v1/pki/roles/web", rolePayload, root)
	if status != http.StatusOK {
		t.Fatalf("create PKI role: %d %s", status, body)
	}

	// Issue a certificate.
	issuePayload := mustJSON(map[string]any{
		"common_name": "app.example.com",
		"ttl":         "24h",
	})
	status, body = doJSON(t, ts, http.MethodPost, "/v1/pki/issue/web", issuePayload, root)
	if status != http.StatusOK {
		t.Fatalf("issue cert: %d %s", status, body)
	}
	var issueResp struct {
		Certificate string `json:"certificate"`
		Serial      string `json:"serial"`
	}
	if err := json.Unmarshal(body, &issueResp); err != nil || issueResp.Certificate == "" {
		t.Fatalf("parse issue response: %v; body: %s", err, body)
	}

	// Parse the issued cert.
	certBlock, _ := pem.Decode([]byte(issueResp.Certificate))
	if certBlock == nil {
		t.Fatalf("issued cert is not PEM")
	}
	issuedCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("parse issued cert: %v", err)
	}
	if issuedCert.Subject.CommonName != "app.example.com" {
		t.Errorf("issued cert CN = %q, want app.example.com", issuedCert.Subject.CommonName)
	}

	// Revoke the certificate.
	status, body = doJSON(t, ts, http.MethodPost, "/v1/pki/revoke/"+issueResp.Serial, "", root)
	if status != http.StatusOK {
		t.Fatalf("revoke cert: %d %s", status, body)
	}

	// CRL must now be non-empty. Returns JSON {"crl":"-----BEGIN X509 CRL-----\n..."}.
	status, crlBody := doJSON(t, ts, http.MethodGet, "/v1/pki/crl/pem", "", "")
	if status != http.StatusOK {
		t.Fatalf("get CRL: %d %s", status, crlBody)
	}
	crlPEMStr := jsonField(t, crlBody, "crl")
	crlBlock, _ := pem.Decode([]byte(crlPEMStr))
	if crlBlock == nil {
		t.Fatalf("CRL is not PEM; got: %s", crlPEMStr[:min(len(crlPEMStr), 100)])
	}
	crl, err := x509.ParseRevocationList(crlBlock.Bytes)
	if err != nil {
		t.Fatalf("parse CRL: %v", err)
	}
	if len(crl.RevokedCertificateEntries) == 0 {
		t.Error("CRL has no revoked entries after revocation")
	}
}

// TestKVv2MetadataOps verifies that KV v2 metadata can be read and updated.
func TestKVv2MetadataOps(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Write a couple of versions.
	doJSON(t, ts, http.MethodPut, "/v2/secret/meta/key", "v1", root)
	doJSON(t, ts, http.MethodPut, "/v2/secret/meta/key", "v2", root)

	// GET metadata.
	status, body := doJSON(t, ts, http.MethodGet, "/v2/secret/metadata/meta/key", "", root)
	if status != http.StatusOK {
		t.Fatalf("get metadata: %d %s", status, body)
	}
	if !strings.Contains(string(body), "current_version") {
		t.Errorf("metadata missing current_version: %s", body)
	}

	// UPDATE metadata (set max_versions).
	status, _ = doJSON(t, ts, http.MethodPut, "/v2/secret/metadata/meta/key",
		`{"max_versions":5}`, root)
	if status != http.StatusNoContent {
		t.Errorf("update metadata: %d, want 204", status)
	}

	// Verify updated field.
	status, body = doJSON(t, ts, http.MethodGet, "/v2/secret/metadata/meta/key", "", root)
	if status != http.StatusOK {
		t.Fatalf("re-get metadata: %d", status)
	}
	if !strings.Contains(string(body), "5") {
		t.Errorf("max_versions not updated; body: %s", body)
	}
}

// TestKVv2Destroy permanently removes versions (cannot be undeleted).
func TestKVv2Destroy(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPut, "/v2/secret/destroy/key", "original", root)

	// Destroy version 1.
	status, body := doJSON(t, ts, http.MethodPost, "/v2/secret/destroy/destroy/key",
		`{"versions":[1]}`, root)
	if status != http.StatusNoContent {
		t.Fatalf("destroy: %d %s", status, body)
	}

	// Attempt to undelete (must fail — already destroyed).
	_, _ = doJSON(t, ts, http.MethodPost, "/v2/secret/undelete/destroy/key",
		`{"versions":[1]}`, root)
	// Undelete on destroyed version should either 422 or succeed silently but still 404 on read.
	// Either way, reading v1 must return 404/410.
	status, _ = doJSON(t, ts, http.MethodGet, "/v2/secret/destroy/key?version=1", "", root)
	if status != http.StatusNotFound && status != http.StatusGone {
		t.Errorf("read destroyed v1: %d, want 404 or 410", status)
	}
}

// TestHealthEndpoint verifies that GET /v1/health returns 200 without auth.
func TestHealthEndpoint(t *testing.T) {
	ts, _, _ := newTestServer(t)

	status, body := doJSON(t, ts, http.MethodGet, "/v1/health", "", "")
	if status != http.StatusOK {
		t.Fatalf("health: %d %s", status, body)
	}
	if !strings.Contains(string(body), "version") && !strings.Contains(string(body), "sealed") {
		t.Errorf("health body %s looks wrong", body)
	}
}

// TestSecretDeleteAndRecreate verifies KV v1 delete + recreate cycle.
func TestSecretDeleteAndRecreate(t *testing.T) {
	ts, _, root := newTestServer(t)

	status, _ := doJSON(t, ts, http.MethodPut, "/v1/secret/lifecycle/key", "original", root)
	if status != http.StatusNoContent {
		t.Fatalf("initial write: %d", status)
	}

	status, _ = doJSON(t, ts, http.MethodDelete, "/v1/secret/lifecycle/key", "", root)
	if status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}

	status, _ = doJSON(t, ts, http.MethodGet, "/v1/secret/lifecycle/key", "", root)
	if status != http.StatusNotFound {
		t.Errorf("read after delete: %d, want 404", status)
	}

	status, _ = doJSON(t, ts, http.MethodPut, "/v1/secret/lifecycle/key", "recreated", root)
	if status != http.StatusNoContent {
		t.Fatalf("recreate: %d", status)
	}

	status, body := doJSON(t, ts, http.MethodGet, "/v1/secret/lifecycle/key", "", root)
	if status != http.StatusOK {
		t.Fatalf("read recreated: %d", status)
	}
	if !strings.Contains(string(body), "recreated") {
		t.Errorf("recreated body: %s", body)
	}
}

// TestAppRoleTokenRenewable verifies that a token issued by AppRole login is
// renewable and can survive past its original TTL after a renew-self call.
func TestAppRoleTokenRenewable(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Create an AppRole with a very short token TTL.
	doJSON(t, ts, http.MethodPut, "/v1/auth/approle/role/renew-test",
		`{"policies":["root"],"token_ttl":"500ms"}`, root)

	status, body := doJSON(t, ts, http.MethodGet, "/v1/auth/approle/role/renew-test", "", root)
	if status != http.StatusOK {
		t.Fatalf("get role: %d %s", status, body)
	}
	roleID := jsonField(t, body, "role_id")

	status, body = doJSON(t, ts, http.MethodPost, "/v1/auth/approle/role/renew-test/secret-id", "", root)
	if status != http.StatusOK {
		t.Fatalf("gen secret-id: %d %s", status, body)
	}
	secretID := jsonField(t, body, "id")

	// Login via AppRole.
	loginBody := mustJSON(map[string]string{"role_id": roleID, "secret_id": secretID})
	status, body = doJSON(t, ts, http.MethodPost, "/v1/auth/approle/login", loginBody, "")
	if status != http.StatusOK {
		t.Fatalf("approle login: %d %s", status, body)
	}
	appTok := jsonField(t, body, "token")
	if appTok == "" {
		t.Fatalf("token empty, body: %s", body)
	}

	// lookup-self must report renewable=true.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/auth/token/lookup-self", "", appTok)
	if status != http.StatusOK {
		t.Fatalf("lookup-self: %d %s", status, body)
	}
	renewable := jsonField(t, body, "renewable")
	if renewable != "true" {
		t.Errorf("approle token renewable=%s, want true", renewable)
	}

	// Renew self while token is still valid.
	renewBody := mustJSON(map[string]string{"ttl": "10s"})
	status, body = doJSON(t, ts, http.MethodPost, "/v1/auth/token/renew-self", renewBody, appTok)
	if status != http.StatusOK {
		t.Fatalf("renew-self: %d %s, want 200", status, body)
	}

	// Wait past the original 500ms TTL.
	time.Sleep(600 * time.Millisecond)

	// Token must still be valid after renewal.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/auth/token/lookup-self", "", appTok)
	if status != http.StatusOK {
		t.Errorf("lookup-self after original TTL: %d, want 200 (token should be alive due to renewal)", status)
	}
}

// min is a helper for older Go compatibility.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
