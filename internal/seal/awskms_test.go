package seal_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/NAGenaev/tuck/internal/seal"
)

// fakeAWSKMSCrypter is an in-memory stub that XOR-encodes plaintext.
// XOR with a fixed mask is self-inverse, so the same function encrypts and decrypts.
type fakeAWSKMSCrypter struct{}

func (f *fakeAWSKMSCrypter) Encrypt(_ context.Context, _ string, plaintext []byte) ([]byte, error) {
	ct := make([]byte, len(plaintext))
	for i, b := range plaintext {
		ct[i] = b ^ 0xAB
	}
	return ct, nil
}

func (f *fakeAWSKMSCrypter) Decrypt(_ context.Context, _ string, ciphertext []byte) ([]byte, error) {
	// XOR is self-inverse
	return f.Encrypt(context.Background(), "", ciphertext)
}

func newTestAWSKMS(t *testing.T) *seal.AWSKMSSeal {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aws-wrapped.key")
	return seal.NewAWSKMS("alias/tuck-seal", "us-east-1", path,
		seal.WithAWSKMSCrypter(&fakeAWSKMSCrypter{}))
}

// TestAWSKMSSeal_TypeIsAWSKMS verifies Type() returns the correct string.
func TestAWSKMSSeal_TypeIsAWSKMS(t *testing.T) {
	s := newTestAWSKMS(t)
	if s.Type() != "awskms" {
		t.Errorf("Type() = %q, want \"awskms\"", s.Type())
	}
}

// TestAWSKMSSeal_InitRootKeySize verifies that Init returns a 32-byte root key.
func TestAWSKMSSeal_InitRootKeySize(t *testing.T) {
	s := newTestAWSKMS(t)
	result, err := s.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(result.RootKey) != 32 {
		t.Errorf("RootKey length = %d, want 32", len(result.RootKey))
	}
	if result.Shares != nil {
		t.Error("AWSKMSSeal.Init() should not return Shares")
	}
}

// TestAWSKMSSeal_InitUnsealRoundTrip verifies that Init and Unseal recover the same key.
func TestAWSKMSSeal_InitUnsealRoundTrip(t *testing.T) {
	s := newTestAWSKMS(t)

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

// TestAWSKMSSeal_UnsealMissingFile verifies Unseal errors when the wrapped key file is absent.
func TestAWSKMSSeal_UnsealMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.key")
	s := seal.NewAWSKMS("alias/test", "us-east-1", path,
		seal.WithAWSKMSCrypter(&fakeAWSKMSCrypter{}))
	if _, err := s.Unseal(); err == nil {
		t.Error("expected error for missing wrapped key file, got nil")
	}
}
