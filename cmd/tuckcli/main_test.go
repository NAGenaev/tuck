package main

import (
	"io"
	"net/http"
	"net/http/httptest"
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
