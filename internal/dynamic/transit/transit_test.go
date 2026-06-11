package transit

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

// --- in-memory barrier ---

type memBarrier struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMem() *memBarrier { return &memBarrier{data: make(map[string][]byte)} }

func (m *memBarrier) Get(_ context.Context, key string) (*physical.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	c := make([]byte, len(v))
	copy(c, v)
	return &physical.Entry{Key: key, Value: c}, nil
}
func (m *memBarrier) Put(_ context.Context, e *physical.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := make([]byte, len(e.Value))
	copy(c, e.Value)
	m.data[e.Key] = c
	return nil
}
func (m *memBarrier) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}
func (m *memBarrier) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

// --- helpers ---

func mgr(t *testing.T) *Manager {
	t.Helper()
	return NewManager(newMem())
}

func mustCreateKey(t *testing.T, m *Manager, name, keyType string) {
	t.Helper()
	if err := m.CreateKey(context.Background(), name, keyType); err != nil {
		t.Fatalf("CreateKey(%q, %q): %v", name, keyType, err)
	}
}

// --- tests ---

func TestCreateKeyIdempotent(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()

	mustCreateKey(t, m, "k", "aes256-gcm96")
	// Second call must not error or overwrite.
	if err := m.CreateKey(ctx, "k", "aes256-gcm96"); err != nil {
		t.Fatal(err)
	}
	k, err := m.GetKey(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if k.LatestVersion != 1 {
		t.Fatalf("expected version 1, got %d", k.LatestVersion)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "enc", "aes256-gcm96")

	plain := []byte("hello, transit engine!")
	ct, err := m.Encrypt(ctx, "enc", plain)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ct, "vault:v1:") {
		t.Fatalf("unexpected ciphertext format: %q", ct)
	}

	got, err := m.Decrypt(ctx, "enc", ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("decrypt mismatch: got %q, want %q", got, plain)
	}
}

func TestEncryptNonDeterministic(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "aes256-gcm96")

	plain := []byte("same plaintext")
	ct1, _ := m.Encrypt(ctx, "k", plain)
	ct2, _ := m.Encrypt(ctx, "k", plain)
	if ct1 == ct2 {
		t.Fatal("two encryptions of the same plaintext should produce different ciphertexts")
	}
}

func TestRotateAndRewrap(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "aes256-gcm96")

	plain := []byte("secret data")
	oldCT, _ := m.Encrypt(ctx, "k", plain)

	if err := m.Rotate(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	k, _ := m.GetKey(ctx, "k")
	if k.LatestVersion != 2 {
		t.Fatalf("expected version 2 after rotate, got %d", k.LatestVersion)
	}

	// Old ciphertext (v1) still decryptable.
	dec, err := m.Decrypt(ctx, "k", oldCT)
	if err != nil {
		t.Fatalf("old ciphertext should still decrypt: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatal("decrypted old CT mismatch")
	}

	// Rewrap produces v2 ciphertext.
	newCT, err := m.Rewrap(ctx, "k", oldCT)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(newCT, "vault:v2:") {
		t.Fatalf("rewrapped ciphertext should be v2, got: %q", newCT)
	}
	dec2, err := m.Decrypt(ctx, "k", newCT)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dec2, plain) {
		t.Fatal("decrypt of rewrapped CT mismatch")
	}
}

func TestMinVersionBlocksDecrypt(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "aes256-gcm96")

	oldCT, _ := m.Encrypt(ctx, "k", []byte("old"))

	// Rotate, then set min_decryption_version=2.
	m.Rotate(ctx, "k")
	m.UpdateKey(ctx, "k", 2, false)

	// Old v1 ciphertext must be rejected.
	if _, err := m.Decrypt(ctx, "k", oldCT); !errors.Is(err, ErrKeyVersionTooOld) {
		t.Fatalf("expected ErrKeyVersionTooOld, got %v", err)
	}
	// New v2 ciphertext still works.
	newCT, _ := m.Encrypt(ctx, "k", []byte("new"))
	if _, err := m.Decrypt(ctx, "k", newCT); err != nil {
		t.Fatalf("v2 ciphertext should decrypt: %v", err)
	}
}

func TestDeleteKeyNotDeletableByDefault(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "aes256-gcm96")

	if err := m.DeleteKey(ctx, "k"); !errors.Is(err, ErrKeyNotDeletable) {
		t.Fatalf("expected ErrKeyNotDeletable, got %v", err)
	}
}

func TestDeleteKeyAfterMarkingDeletable(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "aes256-gcm96")
	m.UpdateKey(ctx, "k", 0, true)

	if err := m.DeleteKey(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetKey(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestListKeys(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()

	mustCreateKey(t, m, "alpha", "aes256-gcm96")
	mustCreateKey(t, m, "beta", "ed25519")

	names, err := m.ListKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(names), names)
	}
}

func TestSignVerifyECDSA(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "ecdsa", "ecdsa-p256")

	msg := []byte("payload to sign")
	sig, err := m.Sign(ctx, "ecdsa", msg, "sha2-256")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sig, "vault:v1:") {
		t.Fatalf("unexpected sig format: %q", sig)
	}

	ok, err := m.Verify(ctx, "ecdsa", msg, sig, "sha2-256")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected verification to succeed")
	}

	// Wrong message should fail.
	ok, _ = m.Verify(ctx, "ecdsa", []byte("tampered"), sig, "sha2-256")
	if ok {
		t.Fatal("expected verification of tampered message to fail")
	}
}

func TestSignVerifyEd25519(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "ed", "ed25519")

	msg := []byte("ed25519 message")
	sig, err := m.Sign(ctx, "ed", msg, "")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := m.Verify(ctx, "ed", msg, sig, "")
	if err != nil || !ok {
		t.Fatalf("ed25519 verification failed: err=%v ok=%v", err, ok)
	}
}

func TestSignVerifyRSA(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "rsa", "rsa-2048")

	msg := []byte("rsa pss message")
	sig, err := m.Sign(ctx, "rsa", msg, "sha2-256")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := m.Verify(ctx, "rsa", msg, sig, "sha2-256")
	if err != nil || !ok {
		t.Fatalf("rsa-pss verification failed: err=%v ok=%v", err, ok)
	}
}

func TestHMAC(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "aes256-gcm96")

	input := []byte("data to mac")
	mac1, err := m.HMAC(ctx, "k", input, "sha2-256")
	if err != nil {
		t.Fatal(err)
	}
	mac2, _ := m.HMAC(ctx, "k", input, "sha2-256")
	// Same key + same input = same HMAC.
	if mac1 != mac2 {
		t.Fatal("HMAC should be deterministic for same key and input")
	}

	// Different input = different HMAC.
	mac3, _ := m.HMAC(ctx, "k", []byte("different"), "sha2-256")
	if mac1 == mac3 {
		t.Fatal("HMAC of different inputs should differ")
	}
}

func TestEncryptionKeyRejectsSign(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "aes256-gcm96")

	if _, err := m.Sign(ctx, "k", []byte("x"), ""); !errors.Is(err, ErrNotSigningKey) {
		t.Fatalf("expected ErrNotSigningKey, got %v", err)
	}
}

func TestSigningKeyRejectsEncrypt(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "ed25519")

	if _, err := m.Encrypt(ctx, "k", []byte("x")); !errors.Is(err, ErrNotEncryptionKey) {
		t.Fatalf("expected ErrNotEncryptionKey, got %v", err)
	}
}

func TestInvalidCiphertext(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "aes256-gcm96")

	if _, err := m.Decrypt(ctx, "k", "garbage"); !errors.Is(err, ErrInvalidCiphertext) {
		t.Fatalf("expected ErrInvalidCiphertext, got %v", err)
	}
}

func TestSignRotateVerifyOldVersion(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()
	mustCreateKey(t, m, "k", "ecdsa-p256")

	msg := []byte("signed with v1")
	sig, _ := m.Sign(ctx, "k", msg, "sha2-256")

	// Rotate — now v2 is latest. Old signature (v1) must still verify.
	m.Rotate(ctx, "k")
	ok, err := m.Verify(ctx, "k", msg, sig, "sha2-256")
	if err != nil || !ok {
		t.Fatalf("v1 signature should still verify after rotate: err=%v ok=%v", err, ok)
	}
}
