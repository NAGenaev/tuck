package csi

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPClientCAFile(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	ca := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}

	get := func(hc *http.Client) error {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		resp, err := hc.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		return err
	}

	hc, err := httpClient(false, ca)
	if err != nil {
		t.Fatal(err)
	}
	if err := get(hc); err != nil {
		t.Errorf("pinned CA must verify: %v", err)
	}
	hc, _ = httpClient(false, "")
	if err := get(hc); err == nil {
		t.Error("unknown CA must be rejected without a bundle")
	}
	if _, err := httpClient(false, filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("missing bundle must be an error")
	}
}
