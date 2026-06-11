package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

func newTestServer(t *testing.T) (*httptest.Server, *core.Core) {
	t.Helper()
	c := core.New(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	if _, err := c.Start(context.Background()); err != nil {
		t.Fatalf("core start: %v", err)
	}
	ts := httptest.NewServer(New(c).Handler())
	t.Cleanup(ts.Close)
	return ts, c
}

func TestKVRoundTrip(t *testing.T) {
	ts, _ := newTestServer(t)

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/v1/secret/db/password", strings.NewReader("hunter2"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/v1/secret/db/password")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"value":"hunter2"`) {
		t.Fatalf("GET body = %s, want value hunter2", body)
	}

	resp, err = http.Get(ts.URL + "/v1/secret/nope")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET missing status = %d, want 404", resp.StatusCode)
	}
}

func TestSealedReturns503(t *testing.T) {
	ts, c := newTestServer(t)
	c.Seal()
	resp, err := http.Get(ts.URL + "/v1/secret/db/password")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("sealed GET status = %d, want 503", resp.StatusCode)
	}
}

func TestHealth(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"sealed":false`) {
		t.Fatalf("health body = %s, want sealed:false", body)
	}
}
