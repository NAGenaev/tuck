package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type wrapListResp struct {
	Scope    string `json:"scope"`
	Capsules []struct {
		Accessor           string `json:"accessor"`
		Size               int    `json:"size"`
		Namespace          string `json:"namespace"`
		CreatorAccessor    string `json:"creator_accessor"`
		CreatorFingerprint string `json:"creator_fingerprint"`
		CreationPath       string `json:"creation_path"`
		CreationTTL        int    `json:"creation_ttl"`
	} `json:"capsules"`
}

func wrapDo(t *testing.T, base, method, path, body, tok, ns string) (int, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(authedNsReq(t, method, base+path, body, tok, ns))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func mkToken(t *testing.T, base, root, policies, ns string) string {
	t.Helper()
	s, b := wrapDo(t, base, "POST", "/v1/auth/token", `{"policies":`+policies+`,"ttl":"1h"}`, root, ns)
	if s != 201 {
		t.Fatal(s, b)
	}
	return jsonField(t, []byte(b), "id")
}

func wrapIt(t *testing.T, base, tok, ns, body string) string {
	t.Helper()
	s, b := wrapDo(t, base, "POST", "/v1/sys/wrapping/wrap", body, tok, ns)
	if s != 200 {
		t.Fatal(s, b)
	}
	return jsonField(t, []byte(b), "token")
}

func listWraps(t *testing.T, base, tok, ns string) wrapListResp {
	t.Helper()
	s, b := wrapDo(t, base, "GET", "/v1/sys/wrapping/list", "", tok, ns)
	if s != 200 {
		t.Fatal(s, b)
	}
	var out wrapListResp
	if err := json.Unmarshal([]byte(b), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWrappingListScopingAndRevoke(t *testing.T) {
	ts, _, root := newTestServer(t)
	if s, b := wrapDo(t, ts.URL, "PUT", "/v1/policy/plain", `{"rules":[{"path":"secret/**","capabilities":["read"]}]}`, root, ""); s != 204 {
		t.Fatal(s, b)
	}
	alice := mkToken(t, ts.URL, root, `["plain"]`, "")
	bob := mkToken(t, ts.URL, root, `["plain"]`, "")

	tokA := wrapIt(t, ts.URL, alice, "", `{"data":{"secret":"alice-secret-value"},"ttl":"10m","creation_path":"secret/db"}`)
	tokB := wrapIt(t, ts.URL, bob, "", `{"data":{"x":"bob-secret"},"ttl":"5m"}`)
	tokR := wrapIt(t, ts.URL, root, "", `{"data":{"r":1},"ttl":"1m"}`)

	// Each plain caller sees only their own capsule; root sees all three.
	la := listWraps(t, ts.URL, alice, "")
	if la.Scope != "own" || len(la.Capsules) != 1 || la.Capsules[0].CreationPath != "secret/db" || la.Capsules[0].CreationTTL != 600 {
		t.Fatalf("alice list wrong: %+v", la)
	}
	if lb := listWraps(t, ts.URL, bob, ""); len(lb.Capsules) != 1 || lb.Capsules[0].Accessor == la.Capsules[0].Accessor {
		t.Fatalf("bob must see only his own: %+v", lb)
	}
	lr := listWraps(t, ts.URL, root, "")
	if lr.Scope != "all" || len(lr.Capsules) != 3 {
		t.Fatalf("root must see all: %+v", lr)
	}

	// Nothing secret in the raw output, ever.
	_, raw := wrapDo(t, ts.URL, "GET", "/v1/sys/wrapping/list", "", root, "")
	for _, leak := range []string{tokA, tokB, tokR, strings.TrimPrefix(tokA, "tuck_wrap_"), "alice-secret-value", "bob-secret", alice, bob, root} {
		if strings.Contains(raw, leak) {
			t.Fatalf("list leaks %q", leak)
		}
	}
	if la.Capsules[0].CreatorFingerprint == "" || la.Capsules[0].CreatorAccessor == "" {
		t.Fatal("creator identity should be recorded")
	}

	// Bob cannot revoke the capsule of Alice (looks like a missing one) ...
	accA := la.Capsules[0].Accessor
	if s, _ := wrapDo(t, ts.URL, "DELETE", "/v1/sys/wrapping/revoke-accessor/"+accA, "", bob, ""); s != 404 {
		t.Fatalf("foreign revoke: want 404, got %d", s)
	}
	if s, b := wrapDo(t, ts.URL, "POST", "/v1/sys/wrapping/lookup", `{"token":"`+tokA+`"}`, alice, ""); s != 200 {
		t.Fatalf("capsule must survive: %d %s", s, b)
	}
	// ... the token is not an accessor ...
	if s, _ := wrapDo(t, ts.URL, "DELETE", "/v1/sys/wrapping/revoke-accessor/"+tokA, "", alice, ""); s != 404 {
		t.Fatalf("token as accessor: want 404, got %d", s)
	}
	// ... the owner can ...
	if s, b := wrapDo(t, ts.URL, "DELETE", "/v1/sys/wrapping/revoke-accessor/"+accA, "", alice, ""); s != 204 {
		t.Fatalf("owner revoke: %d %s", s, b)
	}
	if s, _ := wrapDo(t, ts.URL, "POST", "/v1/sys/wrapping/unwrap", `{"token":"`+tokA+`"}`, alice, ""); s != 404 {
		t.Fatalf("revoked capsule must be gone, got %d", s)
	}
	// ... and root can revoke anyone's.
	accB := listWraps(t, ts.URL, bob, "").Capsules[0].Accessor
	if s, b := wrapDo(t, ts.URL, "DELETE", "/v1/sys/wrapping/revoke-accessor/"+accB, "", root, ""); s != 204 {
		t.Fatalf("root revoke: %d %s", s, b)
	}
	if s, _ := wrapDo(t, ts.URL, "DELETE", "/v1/sys/wrapping/revoke-accessor/"+accB, "", root, ""); s != 404 {
		t.Fatalf("double revoke: want 404, got %d", s)
	}
	// Unwrapped capsules disappear from the list.
	if s, b := wrapDo(t, ts.URL, "POST", "/v1/sys/wrapping/unwrap", `{"token":"`+tokR+`"}`, root, ""); s != 200 {
		t.Fatal(s, b)
	}
	if lr := listWraps(t, ts.URL, root, ""); len(lr.Capsules) != 0 {
		t.Fatalf("list should be empty: %+v", lr)
	}

	// Stats: 3 wrapped, 1 unwrapped, 2 revoked. Plain callers have no access.
	if s, _ := wrapDo(t, ts.URL, "GET", "/v1/sys/wrapping/stats", "", alice, ""); s != 403 {
		t.Fatalf("stats must need ACL: got %d", s)
	}
	s, b := wrapDo(t, ts.URL, "GET", "/v1/sys/wrapping/stats", "", root, "")
	if s != 200 {
		t.Fatal(s, b)
	}
	var st map[string]any
	_ = json.Unmarshal([]byte(b), &st)
	if st["wrapped"] != float64(3) || st["unwrapped"] != float64(1) || st["revoked"] != float64(2) || st["expired"] != float64(0) || st["active"] != float64(0) {
		t.Fatalf("stats wrong: %s", b)
	}
	if st["since"] == nil || st["since"] == "" {
		t.Fatal("since missing")
	}
}

func TestWrappingListNamespaceIsolation(t *testing.T) {
	ts, _, root := newTestServer(t)
	for _, n := range []string{"team-a", "team-b"} {
		if s, b := wrapDo(t, ts.URL, "POST", "/v1/sys/namespaces", `{"name":"`+n+`"}`, root, ""); s != 201 {
			t.Fatal(s, b)
		}
	}
	// Root-policy admin bound to team-a.
	rootA := mkToken(t, ts.URL, root, `["root"]`, "team-a")
	wrapIt(t, ts.URL, rootA, "team-a", `{"data":{"a":1}}`)
	wrapIt(t, ts.URL, root, "team-b", `{"data":{"b":1}}`)
	wrapIt(t, ts.URL, root, "", `{"data":{"r":1}}`)

	la := listWraps(t, ts.URL, rootA, "")
	if len(la.Capsules) != 1 || la.Capsules[0].Namespace != "team-a" {
		t.Fatalf("namespace-bound admin must see only its namespace: %+v", la)
	}
	// Cross-namespace attempts by the bound token are refused outright.
	if s, _ := wrapDo(t, ts.URL, "GET", "/v1/sys/wrapping/list", "", rootA, "team-b"); s != 403 {
		t.Fatalf("cross-namespace list: want 403, got %d", s)
	}
	if lb := listWraps(t, ts.URL, root, "team-b"); len(lb.Capsules) != 1 || lb.Capsules[0].Namespace != "team-b" {
		t.Fatalf("root narrowed to team-b: %+v", lb)
	}
	all := listWraps(t, ts.URL, root, "")
	if len(all.Capsules) != 3 {
		t.Fatalf("root without header sees all namespaces: %+v", all)
	}
	// A namespace admin cannot revoke a capsule of another namespace.
	var accB string
	for _, c := range all.Capsules {
		if c.Namespace == "team-b" {
			accB = c.Accessor
		}
	}
	if s, _ := wrapDo(t, ts.URL, "DELETE", "/v1/sys/wrapping/revoke-accessor/"+accB, "", rootA, ""); s != 404 {
		t.Fatalf("cross-namespace revoke: want 404, got %d", s)
	}
}

func TestWrappingListRequiresToken(t *testing.T) {
	ts, _, _ := newTestServer(t)
	for _, p := range []string{"GET /v1/sys/wrapping/list", "GET /v1/sys/wrapping/stats", "DELETE /v1/sys/wrapping/revoke-accessor/x"} {
		parts := strings.SplitN(p, " ", 2)
		req, _ := http.NewRequest(parts[0], ts.URL+parts[1], nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("%s: want 401, got %d", p, resp.StatusCode)
		}
	}
}
