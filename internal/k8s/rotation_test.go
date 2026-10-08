package k8s

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestReviewer_RereadsTokenFileEveryCall(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"status":{"authenticated":true,"user":{"username":"system:serviceaccount:ns:sa"}}}`))
	}))
	defer srv.Close()

	tokenFile := filepath.Join(t.TempDir(), "reviewer.token")
	write := func(v string) {
		t.Helper()
		if err := os.WriteFile(tokenFile, []byte(v+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("old-token")

	c, err := NewClientFromFiles(srv.URL, tokenFile, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Review("sa-jwt"); err != nil {
		t.Fatal(err)
	}

	write("rotated-token")
	if _, err := c.Review("sa-jwt"); err != nil {
		t.Fatal(err)
	}

	if len(seen) != 2 || seen[0] != "Bearer old-token" || seen[1] != "Bearer rotated-token" {
		t.Fatalf("Authorization headers = %q, want old then rotated token without restart", seen)
	}
}

func TestReviewer_MissingTokenFileIsAnError(t *testing.T) {
	c, err := NewClientFromFiles("http://127.0.0.1:1", filepath.Join(t.TempDir(), "gone"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Review("jwt"); err == nil {
		t.Fatal("expected an error when the reviewer token file is unreadable")
	}
}
