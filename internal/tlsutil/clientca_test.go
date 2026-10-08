package tlsutil

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

// WriteTestCA writes the leaf certificate of a test server as a PEM CA bundle.
func writePEM(t *testing.T, der []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadCAPool(t *testing.T) {
	cert, err := SelfSigned()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x509.ParseCertificate(cert.Certificate[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCAPool(writePEM(t, cert.Certificate[0])); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}

	if _, err := LoadCAPool(filepath.Join(t.TempDir(), "missing.crt")); err == nil {
		t.Error("missing file must be an error")
	}

	junk := filepath.Join(t.TempDir(), "junk.crt")
	if err := os.WriteFile(junk, []byte("not a pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCAPool(junk); err == nil {
		t.Error("non-PEM file must be an error")
	}
}
