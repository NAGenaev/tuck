package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// helpers -----------------------------------------------------------------

func githubRoleBody(repo, ref string, policies []string) string {
	b, _ := json.Marshal(map[string]any{
		"repository": repo,
		"ref":        ref,
		"policies":   policies,
	})
	return string(b)
}

func githubLoginBody(token, role string) string {
	b, _ := json.Marshal(map[string]string{"token": token, "role": role})
	return string(b)
}

// login edge cases --------------------------------------------------------

func TestGitHubLoginMissingRole(t *testing.T) {
	ts, _, _ := newTestServer(t)

	resp, err := http.Post(ts.URL+"/v1/auth/github/login", "application/json",
		strings.NewReader(githubLoginBody("any.jwt.token", "nonexistent")))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("login missing role: want 404, got %d", resp.StatusCode)
	}
}

func TestGitHubLoginInvalidToken(t *testing.T) {
	ts, _, tok := newTestServer(t)

	// Create a role first.
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut,
		ts.URL+"/v1/auth/github/role/prod",
		githubRoleBody("myorg/myrepo", "refs/heads/main", []string{"readonly"}),
		tok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Submit a syntactically invalid JWT — must be rejected.
	resp, err = http.Post(ts.URL+"/v1/auth/github/login", "application/json",
		strings.NewReader(githubLoginBody("not.a.valid.jwt", "prod")))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// Either 401 (invalid token) or 403 (claims mismatch) — both indicate rejection.
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("login invalid JWT: want 401 or 403, got %d", resp.StatusCode)
	}
}

func TestGitHubLoginMissingFields(t *testing.T) {
	ts, _, _ := newTestServer(t)

	// token present but role missing
	body := `{"token":"tok"}`
	resp, err := http.Post(ts.URL+"/v1/auth/github/login", "application/json",
		strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing role field: want 400, got %d", resp.StatusCode)
	}
}

func TestGitHubRoleGetMissing(t *testing.T) {
	ts, _, tok := newTestServer(t)

	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet,
		ts.URL+"/v1/auth/github/role/ghost", "", tok))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET missing role: want 404, got %d", resp.StatusCode)
	}
}
