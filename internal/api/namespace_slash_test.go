package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestNamespaceSlashIndependence verifies that allowing "/" as an
// organizational naming convention in namespace names (e.g. "a/b") does NOT
// create any parent/child relationship: "a" and "a/b" must behave as two
// completely unrelated, fully-isolated namespaces. This is a tenant-isolation
// boundary — it must never regress.
func TestNamespaceSlashIndependence(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	mustCreateNS := func(name string) {
		t.Helper()
		b, _ := json.Marshal(map[string]string{"name": name})
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/sys/namespaces", string(b), rootTok))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create namespace %q status = %d, body = %s", name, resp.StatusCode, body)
		}
	}

	// Create "a" first, then the differently-named "a/b" — creating the
	// latter must NOT require "a" to already exist (no real parent
	// requirement), but here we create both to exercise independence in both
	// directions.
	mustCreateNS("a")
	mustCreateNS("a/b")

	// GET-by-path works for both the plain and the slash-containing name
	// (exercises the {name...} route added for multi-segment names).
	for _, name := range []string{"a", "a/b"} {
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/namespaces/"+name, "", rootTok))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get namespace %q status = %d, body = %s", name, resp.StatusCode, body)
		}
	}

	putSecret := func(ns, value string) {
		t.Helper()
		resp, err := http.DefaultClient.Do(authedNsReq(t, http.MethodPut, ts.URL+"/v1/secret/shared", value, rootTok, ns))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("put secret in %q status = %d", ns, resp.StatusCode)
		}
	}
	getSecret := func(ns string) (int, string) {
		t.Helper()
		resp, err := http.DefaultClient.Do(authedNsReq(t, http.MethodGet, ts.URL+"/v1/secret/shared", "", rootTok, ns))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return resp.StatusCode, ""
		}
		var result map[string]any
		_ = json.Unmarshal(body, &result)
		v, _ := result["value"].(string)
		return resp.StatusCode, v
	}

	// Write distinct secrets at the SAME logical path in each namespace.
	putSecret("a", "value-in-a")
	putSecret("a/b", "value-in-a-slash-b")

	if _, v := getSecret("a"); v != "value-in-a" {
		t.Errorf(`secret in "a" = %q, want "value-in-a" (must not see "a/b"'s secret)`, v)
	}
	if _, v := getSecret("a/b"); v != "value-in-a-slash-b" {
		t.Errorf(`secret in "a/b" = %q, want "value-in-a-slash-b" (must not see "a"'s secret)`, v)
	}

	// Listing secrets in "a" must not surface anything belonging to "a/b".
	resp, err := http.DefaultClient.Do(authedNsReq(t, "LIST", ts.URL+"/v1/secret/", "", rootTok, "a"))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list secrets in \"a\" status = %d, body = %s", resp.StatusCode, body)
	}
	var listResult map[string]any
	_ = json.Unmarshal(body, &listResult)
	keys, _ := listResult["keys"].([]any)
	if len(keys) != 1 || keys[0] != "shared" {
		t.Errorf(`list secrets in "a" = %v, want ["shared"] only (no leakage from "a/b")`, keys)
	}

	// Stats endpoint works for both, independently (exercises the /stats
	// suffix folded into getNamespace for the {name...} route).
	for _, name := range []string{"a", "a/b"} {
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/namespaces/"+name+"/stats", "", rootTok))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("stats for %q status = %d, body = %s", name, resp.StatusCode, body)
		}
	}

	// Deleting "a" must not affect "a/b" in any way.
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodDelete, ts.URL+"/v1/sys/namespaces/a", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete namespace \"a\" status = %d", resp.StatusCode)
	}

	// "a" is gone...
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/namespaces/a", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted namespace \"a\" status = %d, want 404", resp.StatusCode)
	}
	// ...but "a/b" and its secret are completely untouched.
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/namespaces/a/b", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf(`namespace "a/b" status after deleting "a" = %d, want 200 (no cascading delete)`, resp.StatusCode)
	}
	if status, v := getSecret("a/b"); status != http.StatusOK || v != "value-in-a-slash-b" {
		t.Fatalf(`secret in "a/b" after deleting "a": status=%d value=%q, want 200/"value-in-a-slash-b"`, status, v)
	}

	// Now delete "a/b" and confirm it does not resurrect or affect a fresh "a".
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodDelete, ts.URL+"/v1/sys/namespaces/a/b", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete namespace \"a/b\" status = %d", resp.StatusCode)
	}
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/namespaces/a/b", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted namespace \"a/b\" status = %d, want 404", resp.StatusCode)
	}
}

// TestNamespaceSlashCreateDoesNotRequireParent confirms "team-a/project-x" can
// be created even though plain "team-a" was never created as its own
// namespace — there is no parent entity, only a naming convention.
func TestNamespaceSlashCreateDoesNotRequireParent(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	b, _ := json.Marshal(map[string]string{"name": "team-a/project-x"})
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/sys/namespaces", string(b), rootTok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create namespace status = %d, body = %s", resp.StatusCode, body)
	}

	// Plain "team-a" was never created and must not exist.
	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/namespaces/team-a", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf(`"team-a" status = %d, want 404 (no implicit parent namespace)`, resp.StatusCode)
	}
}
