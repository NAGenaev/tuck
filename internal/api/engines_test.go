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

func TestExperimentalEngineGate(t *testing.T) {
	c := core.New(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	res, err := c.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	srv := New(c)
	srv.RestrictEngines([]string{"totp"})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	get := func(path string) (int, string) {
		t.Helper()
		resp, err := http.DefaultClient.Do(authedReq(t, "GET", ts.URL+path, "", res.RootToken.ID))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	for _, p := range []string{"/v1/pki/ca/pem", "/v1/ssh/ca/public-key", "/v1/aws/config", "/v1/gcp/config", "/v1/azure/config"} {
		if s, b := get(p); s != http.StatusNotFound || !strings.Contains(b, "disabled") {
			t.Errorf("%s: want 404 disabled, got %d %s", p, s, b)
		}
	}
	if s, b := get("/v1/totp/keys/"); strings.Contains(b, "disabled") {
		t.Errorf("explicitly enabled engine must pass the gate: %d %s", s, b)
	}
	if s, _ := get("/v1/health"); s != http.StatusOK {
		t.Errorf("core endpoints must be unaffected, got %d", s)
	}
}

func TestExperimentalEnginesOpenWithoutRestriction(t *testing.T) {
	ts, _, root := newTestServer(t)
	resp, err := http.DefaultClient.Do(authedReq(t, "GET", ts.URL+"/v1/aws/config", "", root))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(b), "disabled (experimental)") {
		t.Errorf("gate must be off unless RestrictEngines is called: %s", b)
	}
}
