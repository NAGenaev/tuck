package transit

import (
	"context"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

// FuzzParseVaultToken verifies that parseVaultToken never panics on arbitrary strings.
// It is the first line of defence for any ciphertext/signature supplied by a caller.
func FuzzParseVaultToken(f *testing.F) {
	f.Add("vault:v1:aGVsbG8=")
	f.Add("vault:v999:abc123")
	f.Add("vault:v1:")             // empty payload
	f.Add("vault:v0:x")           // version 0 — invalid
	f.Add("vault:vNaN:x")         // non-numeric version
	f.Add("vault:v-1:x")          // negative version
	f.Add("")                      // empty string
	f.Add("not-a-vault-token")     // wrong prefix
	f.Add("vault:v:")              // missing version number
	f.Add("vault:v1" + string(make([]byte, 512)) + ":x") // very long version field

	f.Fuzz(func(t *testing.T, s string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("parseVaultToken panicked on %q: %v", s, r)
			}
		}()
		_, _, _ = parseVaultToken(s)
	})
}

// FuzzDecryptAES verifies that Decrypt never panics on arbitrary ciphertext strings
// when the underlying key is AES-256-GCM.
func FuzzDecryptAES(f *testing.F) {
	f.Add("vault:v1:AAAA")
	f.Add("vault:v1:")
	f.Add("")
	f.Add("vault:v2147483647:garbage")
	f.Add(string([]byte{0x00, 0xff, 0xfe}))
	f.Add("vault:v1:" + string(make([]byte, 4096)))

	f.Fuzz(func(t *testing.T, ct string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Decrypt(AES) panicked on %q: %v", ct, r)
			}
		}()
		m := NewManager(physical.NewInMem())
		ctx := context.Background()
		_ = m.CreateKey(ctx, "k", "aes256-gcm96")
		_, _ = m.Decrypt(ctx, "k", ct)
	})
}

// FuzzVerifyECDSA verifies that Verify never panics on arbitrary signature strings
// for an ECDSA-P256 key.
func FuzzVerifyECDSA(f *testing.F) {
	f.Add("vault:v1:AAAA")
	f.Add("vault:v1:")
	f.Add("")
	f.Add(string([]byte{0xff, 0x00}))
	f.Add("vault:v1:" + string(make([]byte, 256)))

	f.Fuzz(func(t *testing.T, sig string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Verify(ECDSA) panicked on sig=%q: %v", sig, r)
			}
		}()
		m := NewManager(physical.NewInMem())
		ctx := context.Background()
		_ = m.CreateKey(ctx, "k", "ecdsa-p256")
		_, _ = m.Verify(ctx, "k", []byte("data"), sig, "sha2-256")
	})
}

// FuzzVerifyEd25519 verifies that Verify never panics on arbitrary signature strings
// for an Ed25519 key.
func FuzzVerifyEd25519(f *testing.F) {
	f.Add("vault:v1:AAAA")
	f.Add("")
	f.Add("vault:v1:" + string(make([]byte, 64)))

	f.Fuzz(func(t *testing.T, sig string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Verify(Ed25519) panicked on sig=%q: %v", sig, r)
			}
		}()
		m := NewManager(physical.NewInMem())
		ctx := context.Background()
		_ = m.CreateKey(ctx, "k", "ed25519")
		_, _ = m.Verify(ctx, "k", []byte("data"), sig, "")
	})
}

// FuzzEncryptDecryptRoundtrip verifies the invariant:
// Decrypt(key, Encrypt(key, p)) == p for all plaintexts p.
func FuzzEncryptDecryptRoundtrip(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xff, 0xfe, 0x00, 0x01})
	f.Add(make([]byte, 4096))

	f.Fuzz(func(t *testing.T, plaintext []byte) {
		m := NewManager(physical.NewInMem())
		ctx := context.Background()
		if err := m.CreateKey(ctx, "rt", "aes256-gcm96"); err != nil {
			t.Fatalf("CreateKey: %v", err)
		}
		ct, err := m.Encrypt(ctx, "rt", plaintext)
		if err != nil {
			t.Fatalf("Encrypt(%x): %v", plaintext, err)
		}
		got, err := m.Decrypt(ctx, "rt", ct)
		if err != nil {
			t.Fatalf("Decrypt after Encrypt(%x): %v", plaintext, err)
		}
		if string(got) != string(plaintext) {
			t.Errorf("roundtrip mismatch: got %x, want %x", got, plaintext)
		}
	})
}

// FuzzRewrap verifies that Rewrap never panics on arbitrary ciphertext strings.
func FuzzRewrap(f *testing.F) {
	f.Add("vault:v1:AAAA")
	f.Add("")
	f.Add("not-a-token")

	f.Fuzz(func(t *testing.T, ct string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Rewrap panicked on %q: %v", ct, r)
			}
		}()
		m := NewManager(physical.NewInMem())
		ctx := context.Background()
		_ = m.CreateKey(ctx, "k", "aes256-gcm96")
		_ = m.Rotate(ctx, "k")
		_, _ = m.Rewrap(ctx, "k", ct)
	})
}
