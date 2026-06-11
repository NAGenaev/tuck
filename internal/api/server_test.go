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

func newTestServer(t *testing.T) (*httptest.Server, *core.Core, string) {
	t.Helper()
	c := core.New(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	result, err := c.Start(context.Background())
	if err != nil {
		t.Fatalf("core start: %v", err)
	}
	ts := httptest.NewServer(New(c).Handler())
	t.Cleanup(ts.Close)
	return ts, c, result.RootToken.ID
}

func authedReq(t *testing.T, method, url, body, tokenID string) *http.Request {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatal(err)
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
