package tlsutil

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePair(t *testing.T, dir, cn string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kd, _ := x509.MarshalECPrivateKey(key)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "tls.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	must(os.WriteFile(filepath.Join(dir, "tls.key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd}), 0o600))
}

func commonName(t *testing.T, r *Reloader) string {
	t.Helper()
	c, err := r.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	x, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return x.Subject.CommonName
}

func TestReloaderPicksUpRenewedCertificate(t *testing.T) {
	dir := t.TempDir()
	writePair(t, dir, "old")
	r, err := NewReloader(filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key"))
	if err != nil {
		t.Fatal(err)
	}
	if got := commonName(t, r); got != "old" {
		t.Fatalf("cn = %q", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Watch(ctx, 10*time.Millisecond, t.Logf)

	time.Sleep(20 * time.Millisecond)
	writePair(t, dir, "new")
	deadline := time.Now().Add(3 * time.Second)
	for commonName(t, r) != "new" {
		if time.Now().After(deadline) {
			t.Fatal("renewed certificate not picked up")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReloaderKeepsCertOnBrokenFiles(t *testing.T) {
	dir := t.TempDir()
	writePair(t, dir, "good")
	r, err := NewReloader(filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "tls.crt"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.reload(); err == nil {
		t.Fatal("expected reload error")
	}
	if got := commonName(t, r); got != "good" {
		t.Fatalf("cn = %q, want the previous certificate", got)
	}
}
