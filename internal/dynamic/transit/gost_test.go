package transit

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestKuznyechikMGMKeyLifecycle(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "g", "kuznyechik-mgm")

	plain := []byte("secret payload")
	ct, err := m.Encrypt(ctx, "g", plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Decrypt(ctx, "g", ct)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("decrypt: %q %v", got, err)
	}

	// Rotation keeps old ciphertext readable; rewrap moves it to v2.
	if err := m.Rotate(ctx, "g"); err != nil {
		t.Fatal(err)
	}
	if got, err := m.Decrypt(ctx, "g", ct); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("decrypt old version: %q %v", got, err)
	}
	rw, err := m.Rewrap(ctx, "g", ct)
	if err != nil || rw == ct {
		t.Fatalf("rewrap: %q %v", rw, err)
	}
	if got, err := m.Decrypt(ctx, "g", rw); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("decrypt rewrapped: %q %v", got, err)
	}

	// Tampering is detected; the key does not sign.
	bad := ct[:len(ct)-2] + "AA"
	if bad == ct {
		bad = ct[:len(ct)-2] + "BB"
	}
	if _, err := m.Decrypt(ctx, "g", bad); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
	if _, err := m.Sign(ctx, "g", []byte("x"), ""); !errors.Is(err, ErrNotSigningKey) {
		t.Fatalf("expected ErrNotSigningKey, got %v", err)
	}
}

func TestKuznyechikCiphertextNotInterchangeableWithAES(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "g", "kuznyechik-mgm")
	mustCreateKey(t, m, "a", "aes256-gcm96")
	ct, _ := m.Encrypt(ctx, "g", []byte("x"))
	if _, err := m.Decrypt(ctx, "a", ct); err == nil {
		t.Fatal("ciphertext must not decrypt under a different key")
	}
}

func TestHMACStreebog(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "aes256-gcm96")
	in := []byte("data")

	m256, err := m.HMAC(ctx, "k", in, "streebog-256")
	if err != nil {
		t.Fatal(err)
	}
	m512, _ := m.HMAC(ctx, "k", in, "streebog-512")
	sha, _ := m.HMAC(ctx, "k", in, "sha2-256")
	again, _ := m.HMAC(ctx, "k", in, "streebog-256")
	if m256 != again {
		t.Fatal("HMAC-Streebog must be deterministic")
	}
	if m256 == m512 || m256 == sha {
		t.Fatal("different hash algorithms must give different MACs")
	}
	if len(m256) >= len(m512) {
		t.Fatalf("streebog-512 MAC should be longer: %d vs %d", len(m256), len(m512))
	}
}
