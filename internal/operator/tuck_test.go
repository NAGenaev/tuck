package operator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestGetSecret_KVVersionSelectsURL verifies GetSecret hits /v1/secret/ for
// kvVersion "1" (or "") and /v2/secret/ for "2" — the exact prefix mismatch
// that silently 404ed every KV v2 secret.
func TestGetSecret_KVVersionSelectsURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/kubernetes/login":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
		default:
			gotPath = r.URL.Path
			_ = json.NewEncoder(w).Encode(map[string]string{"path": "db", "value": "ok"})
		}
	}))
	defer srv.Close()

	tokenFile, err := os.CreateTemp(t.TempDir(), "sa-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tokenFile.WriteString("fake-jwt"); err != nil {
		t.Fatal(err)
	}
	_ = tokenFile.Close()

	c := NewTuckClient(srv.URL, tokenFile.Name())

	if _, err := c.GetSecret(context.Background(), "db", "1"); err != nil {
		t.Fatalf("GetSecret v1: %v", err)
	}
	if !strings.HasPrefix(gotPath, "/v1/secret/") {
		t.Errorf("kvVersion=1: expected /v1/secret/ prefix, got %q", gotPath)
	}

	if _, err := c.GetSecret(context.Background(), "db", "2"); err != nil {
		t.Fatalf("GetSecret v2: %v", err)
	}
	if !strings.HasPrefix(gotPath, "/v2/secret/") {
		t.Errorf("kvVersion=2: expected /v2/secret/ prefix, got %q", gotPath)
	}

	if _, err := c.GetSecret(context.Background(), "db", ""); err != nil {
		t.Fatalf("GetSecret default: %v", err)
	}
	if !strings.HasPrefix(gotPath, "/v1/secret/") {
		t.Errorf("kvVersion=\"\": expected /v1/secret/ default prefix, got %q", gotPath)
	}
}

func TestGetSecret_SendsTuckNamespace(t *testing.T) {
	var gotNS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/kubernetes/login" {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
			return
		}
		gotNS = r.Header.Get("X-Tuck-Namespace")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": "ok"})
	}))
	defer srv.Close()
	f := t.TempDir() + "/sa"
	if err := os.WriteFile(f, []byte("jwt"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := NewTuckClient(srv.URL, f)
	if _, err := c.GetSecret(withTuckNamespace(context.Background(), "team-a"), "db", "1"); err != nil || gotNS != "team-a" {
		t.Fatalf("header %q err %v", gotNS, err)
	}
	if _, err := c.GetSecret(context.Background(), "db", "1"); err != nil || gotNS != "" {
		t.Fatalf("unbound read must not send a namespace: %q %v", gotNS, err)
	}
}
