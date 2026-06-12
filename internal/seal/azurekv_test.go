package seal_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/NAGenaev/tuck/internal/seal"
)

// fakeAzureKVCrypter is an in-memory stub that XOR-encodes plaintext.
type fakeAzureKVCrypter struct{}

func (f *fakeAzureKVCrypter) Encrypt(_ context.Context, _ string, plaintext []byte) ([]byte, error) {
	ct := make([]byte, len(plaintext))
	for i, b := range plaintext {
		ct[i] = b ^ 0xEF
	}
	return ct, nil
}

func (f *fakeAzureKVCrypter) Decrypt(_ context.Context, _ string, ciphertext []byte) ([]byte, error) {
	return f.Encrypt(context.Background(), "", ciphertext)
}

func newTestAzureKV(t *testing.T) *seal.AzureKVSeal {
	t.Helper()
	path := filepath.Join(t.TempDir(), "azurekv-wrapped.key")
	return seal.NewAzureKV(
		"https://test-vault.vault.azure.net",
		"tuck-seal",
		"",
		path,
		seal.WithAzureKVCrypter(&fakeAzureKVCrypter{}),
	)
}

// TestAzureKVSeal_TypeIsAzureKV verifies Type() returns the correct string.
func TestAzureKVSeal_TypeIsAzureKV(t *testing.T) {
	s := newTestAzureKV(t)
	if s.Type() != "azurekv" {
		t.Errorf("Type() = %q, want \"azurekv\"", s.Type())
	}
}

// TestAzureKVSeal_InitRootKeySize verifies Init returns a 32-byte root key.
func TestAzureKVSeal_InitRootKeySize(t *testing.T) {
	s := newTestAzureKV(t)
	result, err := s.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(result.RootKey) != 32 {
		t.Errorf("RootKey length = %d, want 32", len(result.RootKey))
	}
	if result.Shares != nil {
		t.Error("AzureKVSeal.Init() should not return Shares")
	}
}

// TestAzureKVSeal_InitUnsealRoundTrip verifies that Init and Unseal recover the same key.
func TestAzureKVSeal_InitUnsealRoundTrip(t *testing.T) {
	s := newTestAzureKV(t)

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

// TestAzureKVSeal_UnsealMissingFile verifies Unseal errors when the wrapped key file is absent.
func TestAzureKVSeal_UnsealMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.key")
	s := seal.NewAzureKV("https://vault.azure.net", "key", "", path,
		seal.WithAzureKVCrypter(&fakeAzureKVCrypter{}))
	if _, err := s.Unseal(); err == nil {
		t.Error("expected error for missing wrapped key file, got nil")
	}
}
