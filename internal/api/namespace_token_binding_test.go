package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// A token issued inside a namespace must be bound to it: its policies are
// resolved there and it cannot be pointed at another namespace via the
// client-supplied X-Tuck-Namespace header.
func TestNamespaceBoundTokenCannotCrossNamespaces(t *testing.T) {
	ts, _, root := newTestServer(t)
	do := func(method, path, body, tok, ns string) (int, string) {
		t.Helper()
		resp, err := http.DefaultClient.Do(authedNsReq(t, method, ts.URL+path, body, tok, ns))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	for _, n := range []string{"team-a", "team-b"} {
		if s, b := do("POST", "/v1/sys/namespaces", `{"name":"`+n+`"}`, root, ""); s != 201 {
			t.Fatal(s, b)
		}
	}
	if s, b := do("PUT", "/v1/secret/prod/pass", `{"value":"A-SECRET"}`, root, "team-a"); s != 204 {
		t.Fatal(s, b)
	}
	if s, b := do("PUT", "/v1/secret/prod/pass", `{"value":"B-SECRET"}`, root, "team-b"); s != 204 {
		t.Fatal(s, b)
	}
	if s, b := do("PUT", "/v1/policy/rd", `{"rules":[{"path":"secret/**","capabilities":["read"]}]}`, root, "team-a"); s != 204 {
		t.Fatal(s, b)
	}
	s, b := do("POST", "/v1/auth/token", `{"policies":["rd"],"ttl":"10m"}`, root, "team-a")
	if s != 201 {
		t.Fatal(s, b)
	}
	ta := jsonField(t, []byte(b), "id")

	if s, b := do("GET", "/v1/secret/prod/pass", "", ta, "team-a"); s != 200 {
		t.Fatalf("own namespace: %d %s", s, b)
	}
	// Header omitted: the token's own namespace applies, never the root one.
	if s, b := do("GET", "/v1/secret/prod/pass", "", ta, ""); s != 200 || !strings.Contains(b, "A-SECRET") {
		t.Fatalf("no header must default to the bound namespace: %d %s", s, b)
	}
	if s, b := do("GET", "/v1/secret/prod/pass", "", ta, "team-b"); s != 403 || strings.Contains(b, "B-SECRET") {
		t.Fatalf("cross-namespace read must be refused: %d %s", s, b)
	}
}

