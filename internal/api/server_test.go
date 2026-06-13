package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

func newTestServer(tb testing.TB) (*httptest.Server, *core.Core, string) {
	tb.Helper()
	c := core.New(physical.NewInMem(), seal.NewDev(filepath.Join(tb.TempDir(), "rootkey")))
	result, err := c.Start(context.Background())
	if err != nil {
		tb.Fatalf("core start: %v", err)
	}
	ts := httptest.NewServer(New(c).Handler())
	tb.Cleanup(ts.Close)
	return ts, c, result.RootToken.ID
}

func authedReq(tb testing.TB, method, url, body, tokenID string) *http.Request {
	tb.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		tb.Fatal(err)
	}
	req.Header.Set("X-Tuck-Token", tokenID)
	return req
}

func TestKVRoundTrip(t *testing.T) {
	ts, _, tok := newTestServer(t)

	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/db/password", "hunter2", tok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", resp.StatusCode)
	}

	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/db/password", "", tok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"value":"hunter2"`) {
		t.Fatalf("GET body = %s, want value hunter2", body)
	}

	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/nope", "", tok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET missing status = %d, want 404", resp.StatusCode)
	}
}

func TestUnauthorized(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/v1/secret/anything")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", resp.StatusCode)
	}
}

func TestSealedReturns503(t *testing.T) {
	ts, c, tok := newTestServer(t)
	c.Seal()
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/db/password", "", tok))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("sealed GET status = %d, want 503", resp.StatusCode)
	}
}

func TestHealth(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"sealed":false`) {
		t.Fatalf("health body = %s, want sealed:false", body)
	}
}

func TestTokenManagement(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	body := `{"display_name":"test-app","policies":[]}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/auth/token", body, rootTok))
	if err != nil {
		t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create token status = %d, want 201; body: %s", resp.StatusCode, respBody)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil || created.ID == "" {
		t.Fatalf("parse created token: %v; body: %s", err, respBody)
	}

	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/token/"+created.ID, "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lookup token status = %d, want 200", resp.StatusCode)
	}

	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodDelete, ts.URL+"/v1/auth/token/"+created.ID, "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke token status = %d, want 204", resp.StatusCode)
	}
}

func TestPolicyManagement(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	body := `{"rules":[{"path":"secret/db/*","capabilities":["read","write"]}]}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/policy/db-rw", body, rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put policy status = %d, want 204", resp.StatusCode)
	}

	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/policy/db-rw", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get policy status = %d, want 200; body: %s", resp.StatusCode, respBody)
	}
	if !strings.Contains(string(respBody), `"read"`) {
		t.Fatalf("get policy body = %s, want read capability", respBody)
	}

	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodDelete, ts.URL+"/v1/policy/db-rw", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete policy status = %d, want 204", resp.StatusCode)
	}
}

func TestACLEnforcement(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	// create a read-only policy for secret/prod/*
	policyBody := `{"rules":[{"path":"secret/prod/*","capabilities":["read"]}]}`
	resp, _ := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/policy/prod-ro", policyBody, rootTok))
	resp.Body.Close()

	// create a token with that policy
	tokenBody := `{"display_name":"prod-reader","policies":["prod-ro"]}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/auth/token", tokenBody, rootTok))
	if err != nil {
		t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var limited struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &limited); err != nil {
		t.Fatalf("parse limited token: %v", err)
	}

	// seed a secret with root token
	resp, _ = http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/prod/api-key", "s3cr3t", rootTok))
	resp.Body.Close()

	// limited token can read secret/prod/*
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/prod/api-key", "", limited.ID))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("limited read status = %d, want 200", resp.StatusCode)
	}

	// limited token cannot write
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/prod/new", "val", limited.ID))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("limited write status = %d, want 403", resp.StatusCode)
	}

	// limited token cannot read secret/staging/*
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/staging/key", "", limited.ID))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("out-of-scope read status = %d, want 403", resp.StatusCode)
	}
}

// authedNsReq is like authedReq but also sets X-Tuck-Namespace.
func authedNsReq(tb testing.TB, method, url, body, tokenID, ns string) *http.Request {
	tb.Helper()
	req := authedReq(tb, method, url, body, tokenID)
	if ns != "" {
		req.Header.Set("X-Tuck-Namespace", ns)
	}
	return req
}

// TestNamespaceCRUD tests namespace lifecycle and secret isolation.
func TestNamespaceCRUD(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	// Create namespace
	b, _ := json.Marshal(map[string]string{"name": "dev"})
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/sys/namespaces", string(b), rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create namespace status = %d, want 201", resp.StatusCode)
	}

	// Get namespace
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/namespaces/dev", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get namespace status = %d, body = %s", resp.StatusCode, body)
	}

	// Write secret in root namespace
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/shared", "root-value", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Write secret in dev namespace
	resp, err = http.DefaultClient.Do(authedNsReq(t, http.MethodPut, ts.URL+"/v1/secret/shared", "dev-value", rootTok, "dev"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Read from root — should get "root-value"
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/shared", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("root read status = %d", resp.StatusCode)
	}
	var rootResult map[string]any
	_ = json.Unmarshal(body, &rootResult)
	if v, _ := rootResult["value"].(string); v != "root-value" {
		t.Errorf("root secret = %q, want %q", v, "root-value")
	}

	// Read from dev namespace — should get "dev-value"
	resp, err = http.DefaultClient.Do(authedNsReq(t, http.MethodGet, ts.URL+"/v1/secret/shared", "", rootTok, "dev"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dev read status = %d, body = %s", resp.StatusCode, body)
	}
	var devResult map[string]any
	_ = json.Unmarshal(body, &devResult)
	if v, _ := devResult["value"].(string); v != "dev-value" {
		t.Errorf("dev secret = %q, want %q", v, "dev-value")
	}

	// List namespaces
	resp, err = http.DefaultClient.Do(authedReq(t, "LIST", ts.URL+"/v1/sys/namespaces/", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list namespaces status = %d, body = %s", resp.StatusCode, body)
	}
	var listResult map[string]any
	_ = json.Unmarshal(body, &listResult)
	keys, _ := listResult["keys"].([]any)
	if len(keys) != 1 || keys[0] != "dev" {
		t.Errorf("list namespaces = %v, want [dev]", keys)
	}

	// Delete namespace
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodDelete, ts.URL+"/v1/sys/namespaces/dev", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete namespace status = %d", resp.StatusCode)
	}

	// Get after delete → 404
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/namespaces/dev", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted namespace status = %d, want 404", resp.StatusCode)
	}
}

func TestTokenRoleRoundTrip(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	// Create role
	roleBody := `{"policies":["root"],"ttl":"1h","renewable":true,"max_uses":5}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/auth/token/roles/ci", roleBody, rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put role status = %d, want 204", resp.StatusCode)
	}

	// Get role
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/token/roles/ci", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get role status = %d, body = %s", resp.StatusCode, body)
	}
	var role map[string]any
	_ = json.Unmarshal(body, &role)
	if role["name"] != "ci" {
		t.Errorf("role name = %v, want ci", role["name"])
	}
	if role["renewable"] != true {
		t.Errorf("role renewable = %v, want true", role["renewable"])
	}

	// List roles
	resp, err = http.DefaultClient.Do(authedReq(t, "LIST", ts.URL+"/v1/auth/token/roles/", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list roles status = %d", resp.StatusCode)
	}
	var listRes map[string]any
	_ = json.Unmarshal(body, &listRes)
	keys, _ := listRes["keys"].([]any)
	if len(keys) != 1 || keys[0] != "ci" {
		t.Errorf("list roles = %v, want [ci]", keys)
	}

	// Create token from role
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/auth/token/roles/ci/create", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create-from-role status = %d, body = %s", resp.StatusCode, body)
	}
	var tok map[string]any
	_ = json.Unmarshal(body, &tok)
	if tok["id"] == "" {
		t.Error("created token has no id")
	}
	if tok["renewable"] != true {
		t.Errorf("token renewable = %v, want true", tok["renewable"])
	}
	if tok["max_uses"].(float64) != 5 {
		t.Errorf("token max_uses = %v, want 5", tok["max_uses"])
	}

	// Delete role
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodDelete, ts.URL+"/v1/auth/token/roles/ci", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete role status = %d", resp.StatusCode)
	}

	// Create from deleted role → 404
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/auth/token/roles/ci/create", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("create from deleted role status = %d, want 404", resp.StatusCode)
	}
}
