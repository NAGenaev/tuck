package barrier

import (
	"bytes"
	"context"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

func forEachSuite(t *testing.T, fn func(t *testing.T, name string)) {
	t.Helper()
	for _, name := range SuiteNames() {
		t.Run(name, func(t *testing.T) { fn(t, name) })
	}
}

func TestSuiteLifecycleAndPersistence(t *testing.T) {
	forEachSuite(t, func(t *testing.T, name string) {
		ctx := context.Background()
		be := physical.NewInMem()
		root := randKey(t)

		b := New(be)
		if err := b.InitializeWithSuite(ctx, root, name); err != nil {
			t.Fatal(err)
		}
		if err := b.Unseal(ctx, root); err != nil {
			t.Fatal(err)
		}
		if b.CipherSuite() != name {
			t.Fatalf("suite = %q, want %q", b.CipherSuite(), name)
		}
		want := []byte("s3cr3t-value")
		if err := b.Put(ctx, &physical.Entry{Key: "secret/a", Value: want}); err != nil {
			t.Fatal(err)
		}
		raw, _ := be.Get(ctx, "secret/a")
		if bytes.Contains(raw.Value, want) {
			t.Fatal("plaintext visible at rest")
		}

		// A fresh Barrier over the same backend must pick the suite up from the store.
		b2 := New(be)
		if err := b2.Unseal(ctx, root); err != nil {
			t.Fatal(err)
		}
		if b2.CipherSuite() != name {
			t.Fatalf("reopened suite = %q, want %q", b2.CipherSuite(), name)
		}
		got, err := b2.Get(ctx, "secret/a")
		if err != nil || got == nil || !bytes.Equal(got.Value, want) {
			t.Fatalf("reopened get = %v, %v", got, err)
		}

		// Wrong root key never unseals.
		if err := New(be).Unseal(ctx, randKey(t)); err != ErrUnsealFailed {
			t.Fatalf("wrong key: %v", err)
		}

		// Rekey keeps data readable under the new root key only.
		newRoot := randKey(t)
		if err := b2.Rekey(ctx, newRoot); err != nil {
			t.Fatal(err)
		}
		b3 := New(be)
		if err := b3.Unseal(ctx, root); err != ErrUnsealFailed {
			t.Fatalf("old root key after rekey: %v", err)
		}
		if err := b3.Unseal(ctx, newRoot); err != nil {
			t.Fatal(err)
		}
		if got, err := b3.Get(ctx, "secret/a"); err != nil || !bytes.Equal(got.Value, want) {
			t.Fatalf("after rekey: %v, %v", got, err)
		}
	})
}

func TestSuiteBindsCiphertextToStorageKey(t *testing.T) {
	forEachSuite(t, func(t *testing.T, name string) {
		ctx := context.Background()
		be := physical.NewInMem()
		root := randKey(t)
		b := New(be)
		if err := b.InitializeWithSuite(ctx, root, name); err != nil {
			t.Fatal(err)
		}
		if err := b.Unseal(ctx, root); err != nil {
			t.Fatal(err)
		}
		_ = b.Put(ctx, &physical.Entry{Key: "secret/low", Value: []byte("public")})
		_ = b.Put(ctx, &physical.Entry{Key: "secret/high", Value: []byte("top-secret")})

		// An attacker with storage access copies one ciphertext over another.
		high, _ := be.Get(ctx, "secret/high")
		_ = be.Put(ctx, &physical.Entry{Key: "secret/low", Value: high.Value})
		if _, err := b.Get(ctx, "secret/low"); err == nil {
			t.Fatal("swapped ciphertext must fail authentication")
		}
	})
}

func TestSuiteTamperDetected(t *testing.T) {
	forEachSuite(t, func(t *testing.T, name string) {
		ctx := context.Background()
		be := physical.NewInMem()
		root := randKey(t)
		b := New(be)
		_ = b.InitializeWithSuite(ctx, root, name)
		_ = b.Unseal(ctx, root)
		_ = b.Put(ctx, &physical.Entry{Key: "secret/x", Value: []byte("value")})
		raw, _ := be.Get(ctx, "secret/x")
		raw.Value[len(raw.Value)-1] ^= 1
		_ = be.Put(ctx, raw)
		if _, err := b.Get(ctx, "secret/x"); err == nil {
			t.Fatal("tampered ciphertext must be rejected")
		}
	})
}

func TestUnknownSuiteRejected(t *testing.T) {
	err := New(physical.NewInMem()).InitializeWithSuite(context.Background(), make([]byte, keySize), "rot13")
	if err == nil {
		t.Fatal("unknown suite must be rejected")
	}
}

// A store written before cipher suites existed has no metadata record and
// uses AES-256-GCM without associated data; it must keep working unchanged.
func TestLegacyStoreWithoutMetadata(t *testing.T) {
	ctx := context.Background()
	be := physical.NewInMem()
	root := randKey(t)
	aes, _ := LookupSuite("")

	dek := randKey(t)
	wrapped, _ := aes.seal(root, dek, nil)
	_ = be.Put(ctx, &physical.Entry{Key: keyringPath, Value: wrapped})
	ct, _ := aes.seal(dek, []byte("old-secret"), nil)
	_ = be.Put(ctx, &physical.Entry{Key: "secret/legacy", Value: ct})

	b := New(be)
	if err := b.Unseal(ctx, root); err != nil {
		t.Fatalf("legacy unseal: %v", err)
	}
	got, err := b.Get(ctx, "secret/legacy")
	if err != nil || string(got.Value) != "old-secret" {
		t.Fatalf("legacy get: %v, %v", got, err)
	}
	// New writes into a legacy store stay in the legacy format.
	if err := b.Put(ctx, &physical.Entry{Key: "secret/new", Value: []byte("n")}); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Get(ctx, "secret/new"); err != nil || string(got.Value) != "n" {
		t.Fatalf("legacy put/get: %v, %v", got, err)
	}
}

func TestRotateDataKey(t *testing.T) {
	forEachSuite(t, func(t *testing.T, name string) {
		ctx := context.Background()
		be := physical.NewInMem()
		root := randKey(t)
		b := New(be)
		if err := b.InitializeWithSuite(ctx, root, name); err != nil {
			t.Fatal(err)
		}
		if err := b.Unseal(ctx, root); err != nil {
			t.Fatal(err)
		}
		_ = b.Put(ctx, &physical.Entry{Key: "secret/old", Value: []byte("old")})
		if b.KeyVersion() != 1 {
			t.Fatalf("initial version %d", b.KeyVersion())
		}

		// Rotation without reencryption: old entry still readable, new writes use v2.
		v, n, err := b.RotateDataKey(ctx, false)
		if err != nil || v != 2 || n != 0 {
			t.Fatalf("rotate: %d %d %v", v, n, err)
		}
		_ = b.Put(ctx, &physical.Entry{Key: "secret/new", Value: []byte("new")})
		for k, want := range map[string]string{"secret/old": "old", "secret/new": "new"} {
			if got, err := b.Get(ctx, k); err != nil || string(got.Value) != want {
				t.Fatalf("get %s: %v %v", k, got, err)
			}
		}
		// Survives reseal/unseal with both keys in the keyring.
		b.Seal()
		if err := b.Unseal(ctx, root); err != nil {
			t.Fatal(err)
		}
		if got, err := b.Get(ctx, "secret/old"); err != nil || string(got.Value) != "old" {
			t.Fatalf("after unseal: %v %v", got, err)
		}

		// Rotation with reencryption drops the old key.
		v, n, err = b.RotateDataKey(ctx, true)
		if err != nil || v != 3 || n != 2 {
			t.Fatalf("rotate+reencrypt: %d %d %v", v, n, err)
		}
		if len(b.keys) != 1 {
			t.Fatalf("old keys retained: %d", len(b.keys))
		}
		b2 := New(be)
		if err := b2.Unseal(ctx, root); err != nil {
			t.Fatal(err)
		}
		for k, want := range map[string]string{"secret/old": "old", "secret/new": "new"} {
			if got, err := b2.Get(ctx, k); err != nil || string(got.Value) != want {
				t.Fatalf("get %s after reencrypt: %v %v", k, got, err)
			}
		}
		// Root-key rotation still works after DEK rotation.
		nr := randKey(t)
		if err := b2.Rekey(ctx, nr); err != nil {
			t.Fatal(err)
		}
		b3 := New(be)
		if err := b3.Unseal(ctx, nr); err != nil {
			t.Fatal(err)
		}
		// And a second data-key rotation after the rekey wraps with the new root.
		if _, _, err := b2.RotateDataKey(ctx, true); err != nil {
			t.Fatal(err)
		}
		b4 := New(be)
		if err := b4.Unseal(ctx, nr); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRotateDataKeyUnsupportedOnLegacyStore(t *testing.T) {
	ctx := context.Background()
	be := physical.NewInMem()
	root := randKey(t)
	aes, _ := LookupSuite("")
	dek := randKey(t)
	wrapped, _ := aes.seal(root, dek, nil)
	_ = be.Put(ctx, &physical.Entry{Key: keyringPath, Value: wrapped})
	b := New(be)
	if err := b.Unseal(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.RotateDataKey(ctx, false); err != ErrNotVersioned {
		t.Fatalf("got %v", err)
	}
}
