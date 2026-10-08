package main

import (
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestClientSendsNamespaceHeaderOnEveryCall(t *testing.T) {
	var got []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.Header.Get("X-Tuck-Namespace"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	c := newClient(ts.URL, "tok", "team-a", false)
	for _, call := range []func() (*http.Response, error){
		func() (*http.Response, error) { return c.do("GET", "/x", nil) },
		func() (*http.Response, error) { return c.doRaw("PUT", "/v1/secret/a", http.NoBody) },
		func() (*http.Response, error) { return c.doRaw("LIST", "/v1/secret/", nil) },
	} {
		resp, err := call()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	for _, g := range got {
		if g[len(g)-6:] != "team-a" {
			t.Errorf("request %q lost the namespace header", g)
		}
	}
	if len(got) != 3 {
		t.Fatalf("got %d requests, want 3", len(got))
	}
}

func TestK8sRolePath(t *testing.T) {
	if p := k8sRolePath("", "ns", "sa"); p != "/v1/auth/kubernetes/role/ns/sa" {
		t.Errorf("default cluster path = %q", p)
	}
	if p := k8sRolePath("prod", "ns", "sa"); p != "/v1/auth/kubernetes/clusters/prod/role/ns/sa" {
		t.Errorf("named cluster path = %q", p)
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" a, b ,,c ")
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("splitCSV = %v", got)
	}
}

func TestReadJSONArgStripsBOMFromStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("\xef\xbb\xbf[{\"path\":\"secret/*\"}]"); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()

	if got := readJSONArg("-"); got != `[{"path":"secret/*"}]` {
		t.Fatalf("got %q", got)
	}
	if got := readJSONArg("[]"); got != "[]" {
		t.Fatalf("literal arg changed: %q", got)
	}
}

func TestTrustCAFile(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer ts.Close()
	// Without the CA the self-signed server is rejected.
	c := newClient(ts.URL, "t", "", false)
	if resp, err := c.do("GET", "/x", nil); err == nil {
		_ = resp.Body.Close()
		t.Fatal("untrusted certificate accepted")
	}
	// With the server's certificate as CA it is accepted.
	dir := t.TempDir()
	pemPath := filepath.Join(dir, "ca.pem")
	pemData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ts.Certificate().Raw})
	if err := os.WriteFile(pemPath, pemData, 0o600); err != nil {
		t.Fatal(err)
	}
	c = newClient(ts.URL, "t", "", false)
	if err := c.trustCAFile(pemPath); err != nil {
		t.Fatal(err)
	}
	resp, err := c.do("GET", "/x", nil)
	if err != nil {
		t.Fatalf("trusted CA rejected: %v", err)
	}
	_ = resp.Body.Close()
	// A bad file is an error, never a silent fallback.
	bad := filepath.Join(dir, "bad.pem")
	_ = os.WriteFile(bad, []byte("nonsense"), 0o600)
	if err := newClient(ts.URL, "t", "", false).trustCAFile(bad); err == nil {
		t.Error("garbage CA file accepted")
	}
	if err := newClient(ts.URL, "t", "", false).trustCAFile(filepath.Join(dir, "missing.pem")); err == nil {
		t.Error("missing CA file accepted")
	}
}
