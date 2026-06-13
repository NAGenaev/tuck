package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestSecretListGETCompat(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/explorer-test/foo", "bar", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status %d", resp.StatusCode)
	}

	// Legacy LIST still works (CLI).
	resp, err = http.DefaultClient.Do(authedReq(t, "LIST", ts.URL+"/v1/secret/", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("LIST status %d", resp.StatusCode)
	}

	// Browser-compatible GET ?list=true.
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/?list=true", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET list=true status %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, k := range body.Keys {
		if k == "explorer-test/" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected explorer-test/ in keys, got %v", body.Keys)
	}
}

func TestSecretListGETWithoutListParam(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/explorer-test/foo", "bar", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/explorer-test/foo", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET secret without list: status %d", resp.StatusCode)
	}
}

func TestPolicyListGETCompat(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/policy/?list=true", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET policy list status %d", resp.StatusCode)
	}
}
