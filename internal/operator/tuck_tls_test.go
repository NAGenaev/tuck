package operator

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func tlsTuck(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/kubernetes/login":
			_, _ = w.Write([]byte(`{"token":"t"}`))
		default:
			_, _ = w.Write([]byte(`{"value":"s3cret"}`))
		}
	}))
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.crt")
	b := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(ca, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return srv, ca
}

func saFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(p, []byte("jwt"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTuckClient_CAFileTrustsPrivateCA(t *testing.T) {
	srv, ca := tlsTuck(t)
	c, err := NewTuckClient(srv.URL, saFile(t)).WithCAFile(ca)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.GetSecret(context.Background(), "team/db", "1")
	if err != nil {
		t.Fatalf("GetSecret over TLS with pinned CA: %v", err)
	}
	if string(got) != "s3cret" {
		t.Errorf("value = %q", got)
	}
}

func TestTuckClient_WithoutCARejectsPrivateCA(t *testing.T) {
	srv, _ := tlsTuck(t)
	c := NewTuckClient(srv.URL, saFile(t))
	if _, err := c.GetSecret(context.Background(), "team/db", "1"); err == nil {
		t.Fatal("a certificate from an unknown CA must be rejected")
	}
}

func TestTuckClient_WithCAFileBadBundle(t *testing.T) {
	if _, err := NewTuckClient("https://x", saFile(t)).WithCAFile(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("missing CA file must be an error")
	}
}
