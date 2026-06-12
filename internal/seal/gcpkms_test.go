package seal_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/NAGenaev/tuck/internal/seal"
)

// fakeGCPKMSCrypter is an in-memory stub that XOR-encodes plaintext.
type fakeGCPKMSCrypter struct{}

func (f *fakeGCPKMSCrypter) Encrypt(_ context.Context, _ string, plaintext []byte) ([]byte, error) {
	ct := make([]byte, len(plaintext))
	for i, b := range plaintext {
		ct[i] = b ^ 0xCD
	}
	return ct, nil
}

func (f *fakeGCPKMSCrypter) Decrypt(_ context.Context, _ string, ciphertext []byte) ([]byte, error) {
	return f.Encrypt(context.Background(), "", ciphertext)
}

func (f *fakeGCPKMSCrypter) Close() error { return nil }

func newTestGCPKMS(t *testing.T) *seal.GCPKMSSeal {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gcp-wrapped.key")
	keyName := "projects/my-project/locations/global/keyRings/tuck/cryptoKeys/seal"
	return seal.NewGCPKMS(keyName, path, seal.WithGCPKMSCrypter(&fakeGCPKMSCrypter{}))
}

// TestGCPKMSSeal_TypeIsGCPKMS verifies Type() returns the correct string.
func TestGCPKMSSeal_TypeIsGCPKMS(t *testing.T) {
	s := newTestGCPKMS(t)
	if s.Type() != "gcpkms" {
		t.Errorf("Type() = %q, want \"gcpkms\"", s.Type())
	}
}

// TestGCPKMSSeal_InitRootKeySize verifies that Init returns a 32-byte root key.
func TestGCPKMSSeal_InitRootKeySize(t *testing.T) {
	s := newTestGCPKMS(t)
	result, err := s.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(result.RootKey) != 32 {
		t.Errorf("RootKey length = %d, want 32", len(result.RootKey))
	}
	if result.Shares != nil {
		t.Error("GCPKMSSeal.Init() should not return Shares")
	}
}

// TestGCPKMSSeal_InitUnsealRoundTrip verifies that Init and Unseal recover the same key.
func TestGCPKMSSeal_InitUnsealRoundTrip(t *testing.T) {
	s := newTestGCPKMS(t)

	result, err := s.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	recovered, err := s.Unseal()
	if err != nil {
		t.Fatalf("Unseal: %v", err)
	}

	if len(recovered) != len(result.RootKey) {
		t.Fatalf("recovered key length = %d, want %d", len(recovered), len(result.RootKey))
	}
	for i, b := range result.RootKey {
		if recovered[i] != b {
			t.Errorf("recovered key byte[%d] = %02x, want %02x", i, recovered[i], b)
			break
		}
	}
}

// TestGCPKMSSeal_UnsealMissingFile verifies Unseal errors when the wrapped key file is absent.
func TestGCPKMSSeal_UnsealMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.key")
	keyName := "projects/my-project/locations/global/keyRings/tuck/cryptoKeys/seal"
	s := seal.NewGCPKMS(keyName, path, seal.WithGCPKMSCrypter(&fakeGCPKMSCrypter{}))
	if _, err := s.Unseal(); err == nil {
		t.Error("expected error for missing wrapped key file, got nil")
	}
}
