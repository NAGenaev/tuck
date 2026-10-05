package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
)

func loginK8sCluster(t *testing.T, base, cluster, saToken string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"token": saToken, "cluster": cluster})
	resp, err := http.Post(base+"/v1/auth/kubernetes/login", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// Two clusters can both have prod/api; each gets its own role, and a token
// minted by one cluster never resolves a role of the other.
func TestK8sMultiCluster(t *testing.T) {
	sa := &k8sauth.ReviewResult{Authenticated: true, Username: "system:serviceaccount:prod:api"}
	ts, c, rootTok := newTestServerWithK8s(t, &mockReviewer{result: sa})
	if err := c.AddK8sCluster("dc2", &mockReviewer{result: sa}); err != nil {
		t.Fatal(err)
	}
	// dc3 rejects every token (the SA does not exist there).
	if err := c.AddK8sCluster("dc3", &mockReviewer{result: &k8sauth.ReviewResult{}}); err != nil {
		t.Fatal(err)
	}

	put := func(path, body string, want int) {
		t.Helper()
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+path, body, rootTok))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("PUT %s: status=%d want %d", path, resp.StatusCode, want)
		}
	}
	// Role only in dc2.
	put("/v1/auth/kubernetes/clusters/dc2/role/prod/api", `{"policies":["p"]}`, http.StatusNoContent)
	// Unknown cluster and invalid name are rejected.
	put("/v1/auth/kubernetes/clusters/nope/role/prod/api", `{"policies":["p"]}`, http.StatusBadRequest)
	put("/v1/auth/kubernetes/clusters/BAD NAME/role/prod/api", `{"policies":["p"]}`, http.StatusBadRequest)

	if code, body := loginK8sCluster(t, ts.URL, "dc2", "jwt"); code != http.StatusOK {
		t.Fatalf("dc2 login: %d %s", code, body)
	}
	// Default cluster has the same ns/sa but no role of its own.
	if code, _ := loginK8sCluster(t, ts.URL, "", "jwt"); code != http.StatusForbidden {
		t.Fatalf("default-cluster login must not reuse dc2's role, got %d", code)
	}
	// dc3 does not recognise the token.
	if code, _ := loginK8sCluster(t, ts.URL, "dc3", "jwt"); code != http.StatusUnauthorized {
		t.Fatalf("dc3 login: got %d, want 401", code)
	}
	if code, _ := loginK8sCluster(t, ts.URL, "ghost", "jwt"); code != http.StatusBadRequest {
		t.Fatalf("unknown cluster: got %d, want 400", code)
	}

	// Role is readable under its cluster only.
	resp, _ := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/kubernetes/clusters/dc2/role/prod/api", "", rootTok))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get dc2 role: %d", resp.StatusCode)
	}
	resp, _ = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/kubernetes/role/prod/api", "", rootTok))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("default role must not exist: %d", resp.StatusCode)
	}

	resp, _ = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/auth/kubernetes/clusters", "", rootTok))
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(b), `"dc2"`) || !strings.Contains(string(b), `"dc3"`) {
		t.Fatalf("cluster list: %s", b)
	}
}
