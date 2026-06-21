package barrier

import (
	"context"
	"crypto/rand"
	"io"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

// FuzzDecryptBytes verifies that decrypt never panics on arbitrary (key, ciphertext) pairs.
// Only key slices of exactly keySize bytes are tested — other lengths are skipped
// because AES-256 requires a 32-byte key; the interesting fuzzing is over the ciphertext.
func FuzzDecryptBytes(f *testing.F) {
	// Seed with a real key and a valid ciphertext.
	key := make([]byte, keySize)
	_, _ = io.ReadFull(rand.Reader, key)
	if ct, err := encrypt(key, []byte("hello world")); err == nil {
		f.Add(key, ct)
	}
	// Boundary seeds: too-short ciphertext, empty, exact nonce size, garbage.
	f.Add(key, []byte{})
	f.Add(key, []byte{0x00})
	f.Add(key, make([]byte, 12))  // exactly GCM nonce size — no payload
	f.Add(key, make([]byte, 11))  // one byte short of nonce
	f.Add(key, make([]byte, 100)) // nonce + garbage payload

	f.Fuzz(func(t *testing.T, key, ct []byte) {
		if len(key) != keySize {
			return // skip invalid key lengths — not the attack surface we're testing
		}
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("decrypt panicked on key=%x ct=%x: %v", key, ct, r)
			}
		}()
		_, _ = decrypt(key, ct)
	})
}

// FuzzEncryptDecryptRoundtrip verifies the invariant: decrypt(k, encrypt(k, p)) == p
// for all plaintexts p and randomly generated keys.
func FuzzEncryptDecryptRoundtrip(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xff, 0xfe, 0x00, 0x01})
	f.Add(make([]byte, 4096))

	f.Fuzz(func(t *testing.T, plaintext []byte) {
		key := make([]byte, keySize)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			t.Fatalf("generate key: %v", err)
		}
		ct, err := encrypt(key, plaintext)
		if err != nil {
			t.Fatalf("encrypt(%x): %v", plaintext, err)
		}
		got, err := decrypt(key, ct)
		if err != nil {
			t.Fatalf("decrypt after encrypt(%x): %v", plaintext, err)
		}
		if string(got) != string(plaintext) {
			t.Errorf("roundtrip: got %x, want %x", got, plaintext)
		}
	})
}

// FuzzBarrierUnseal verifies that Barrier.Unseal never panics when called with
// arbitrary root key bytes — including wrong-length and all-zero keys.
func FuzzBarrierUnseal(f *testing.F) {
	f.Add(make([]byte, keySize))   // all-zero valid-length key
	f.Add(make([]byte, keySize-1)) // one byte short
	f.Add(make([]byte, keySize+1)) // one byte over
	f.Add([]byte{})
	f.Add(make([]byte, 64))

	f.Fuzz(func(t *testing.T, rootKey []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Unseal panicked on key=%x: %v", rootKey, r)
			}
		}()
		ctx := context.Background()
		be := physical.NewInMem()
		b := New(be)

		// Initialize with a known-good key so the keyring exists.
		validKey := make([]byte, keySize)
		_, _ = io.ReadFull(rand.Reader, validKey)
		_ = b.Initialize(ctx, validKey)

		// Unseal with fuzz-supplied key — must never panic (may return error).
		_ = b.Unseal(ctx, rootKey)
	})
}
