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
	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

// mockReviewer is a test double for k8s.Reviewer.
type mockReviewer struct {
	result *k8sauth.ReviewResult
	err    error
}

func (m *mockReviewer) Review(_ string) (*k8sauth.ReviewResult, error) {
	return m.result, m.err
}

func newTestServerWithK8s(t *testing.T, reviewer k8sauth.Reviewer) (*httptest.Server, *core.Core, string) {
	t.Helper()
	c := core.NewWithK8s(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")), reviewer)
	result, err := c.Start(context.Background())
	if err != nil {
		t.Fatalf("core start: %v", err)
	}
	ts := httptest.NewServer(New(c).Handler())
	t.Cleanup(ts.Close)
	return ts, c, result.RootToken.ID
}

func loginK8s(t *testing.T, ts *httptest.Server, saToken string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"token": saToken})
	resp, err := http.Post(ts.URL+"/v1/auth/kubernetes/login", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// TestK8sLoginSuccess verifies the full happy path: reviewer authenticates
// the SA, a role is bound, Tuck token is returned.
func TestK8sLoginSuccess(t *testing.T) {
	reviewer := &mockReviewer{result: &k8sauth.ReviewResult{
		Authenticated: true,
		Username:      "system:serviceaccount:default:myapp",
	}}
	ts, _, rootTok := newTestServerWithK8s(t, reviewer)

	// bind role: default/myapp → policy "read-all"
	roleBody := `{"policies":["read-all"],"ttl":"1h"}`
	resp, _ := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/auth/kubernetes/role/default/myapp", roleBody, rootTok))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bind role: status=%d", resp.StatusCode)
	}

	resp = loginK8s(t, ts, "fake-sa-token")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status=%d body=%s", resp.StatusCode, body)
	}
	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil || result["token"] == "" {
		t.Fatalf("login: parse response: %v body=%s", err, body)
	}
	if !strings.HasPrefix(result["token"], "tuck_") {
		t.Errorf("token %q doesn't look like a tuck token", result["token"])
	}
}

// TestK8sLoginTokenHasBoundPolicies verifies the returned Tuck token carries
// exactly the policies from the bound role.
func TestK8sLoginTokenHasBoundPolicies(t *testing.T) {
	reviewer := &mockReviewer{result: &k8sauth.ReviewResult{
		Authenticated: true,
		Username:      "system:serviceaccount:prod:api-server",
	}}
	ts, c, rootTok := newTestServerWithK8s(t, reviewer)

	// create a named policy
	http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/policy/prod-read", //nolint
		`{"rules":[{"path":"secret/prod/*","capabilities":["read"]}]}`, rootTok))

	// bind role with that policy
	http.DefaultClient.Do(authedReq(t, http.MethodPut, //nolint
		ts.URL+"/v1/auth/kubernetes/role/prod/api-server",
		`{"policies":["prod-read"]}`, rootTok))

	resp := loginK8s(t, ts, "sa-jwt")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: %s", body)
	}
	var result map[string]string
	json.Unmarshal(body, &result) //nolint

	// look up the issued token directly via core and verify policies
	tok, err := c.LookupToken(context.Background(), result["token"])
	if err != nil {
		t.Fatalf("lookup issued token: %v", err)
	}
	if len(tok.Policies) != 1 || tok.Policies[0] != "prod-read" {
		t.Errorf("token policies = %v, want [prod-read]", tok.Policies)
	}
}

// TestK8sLoginNotAuthenticated verifies 401 when the reviewer rejects the token.
func TestK8sLoginNotAuthenticated(t *testing.T) {
	reviewer := &mockReviewer{result: &k8sauth.ReviewResult{Authenticated: false}}
	ts, _, _ := newTestServerWithK8s(t, reviewer)

	resp := loginK8s(t, ts, "bad-token")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", resp.StatusCode)
	}
}

// TestK8sLoginNoRole verifies 403 when the SA is authenticated but has no bound role.
func TestK8sLoginNoRole(t *testing.T) {
	reviewer := &mockReviewer{result: &k8sauth.ReviewResult{
		Authenticated: true,
		Username:      "system:serviceaccount:default:unknown-sa",
	}}
	ts, _, _ := newTestServerWithK8s(t, reviewer)

	resp := loginK8s(t, ts, "valid-sa-token")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", resp.StatusCode)
	}
}

// TestK8sLoginDisabled verifies 501 when no reviewer is configured.
func TestK8sLoginDisabled(t *testing.T) {
	ts, _, _ := newTestServerWithK8s(t, nil)

	resp := loginK8s(t, ts, "any-token")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status=%d, want 501", resp.StatusCode)
	}
}

// TestK8sLoginMissingToken verifies 400 on malformed request body.
func TestK8sLoginMissingToken(t *testing.T) {
	ts, _, _ := newTestServerWithK8s(t, nil)

	resp, _ := http.Post(ts.URL+"/v1/auth/kubernetes/login", "application/json",
		strings.NewReader(`{"not_token":"x"}`))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", resp.StatusCode)
	}
}

// TestK8sRoleManagement verifies CRUD of k8s role bindings.
func TestK8sRoleManagement(t *testing.T) {
	ts, _, rootTok := newTestServerWithK8s(t, nil)

	// create
	resp, _ := http.DefaultClient.Do(authedReq(t, http.MethodPut,
		ts.URL+"/v1/auth/kubernetes/role/staging/worker",
		`{"policies":["worker-policy"],"ttl":"12h"}`, rootTok))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put role: status=%d", resp.StatusCode)
	}

	// get
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet,
		ts.URL+"/v1/auth/kubernetes/role/staging/worker", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get role: status=%d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "worker-policy") {
		t.Errorf("get role body=%s, want worker-policy", body)
	}
	if !strings.Contains(string(body), "12h") {
		t.Errorf("get role body=%s, want ttl 12h", body)
	}

	// delete
	resp, _ = http.DefaultClient.Do(authedReq(t, http.MethodDelete,
		ts.URL+"/v1/auth/kubernetes/role/staging/worker", "", rootTok))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete role: status=%d", resp.StatusCode)
	}

	// get after delete → 404
	resp, _ = http.DefaultClient.Do(authedReq(t, http.MethodGet,
		ts.URL+"/v1/auth/kubernetes/role/staging/worker", "", rootTok))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted role: status=%d, want 404", resp.StatusCode)
	}
}

// TestK8sRoleRequiresAuth verifies the role management endpoints require a token.
func TestK8sRoleRequiresAuth(t *testing.T) {
	ts, _, _ := newTestServerWithK8s(t, nil)

	resp, _ := http.Get(ts.URL + "/v1/auth/kubernetes/role/default/myapp")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET role: status=%d, want 401", resp.StatusCode)
	}
}
