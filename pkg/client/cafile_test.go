package client

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWithCAFile(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sealed":false}`))
	}))
	defer srv.Close()

	ca := filepath.Join(t.TempDir(), "ca.crt")
	b := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(ca, b, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := New(srv.URL, "", WithCAFile(ca)).SealStatus(context.Background()); err != nil {
		t.Fatalf("pinned CA must verify the server: %v", err)
	}
	if _, err := New(srv.URL, "").SealStatus(context.Background()); err == nil {
		t.Error("unknown CA must be rejected without WithCAFile")
	}
	if _, err := New(srv.URL, "", WithCAFile(filepath.Join(t.TempDir(), "nope"))).SealStatus(context.Background()); err == nil {
		t.Error("unreadable CA file must fail closed")
	}
}
