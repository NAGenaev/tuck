package barrier

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

func randKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		t.Fatal(err)
	}
	return k
}

// TestBarrierLifecycle walks the full Milestone 0 promise:
// init -> unseal -> write -> read -> (encrypted at rest) -> seal -> read fails.
func TestBarrierLifecycle(t *testing.T) {
	ctx := context.Background()
	be := physical.NewInMem()
	b := New(be)
	rootKey := randKey(t)

	if err := b.Initialize(ctx, rootKey); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := b.Unseal(ctx, rootKey); err != nil {
		t.Fatalf("unseal: %v", err)
	}
	if b.IsSealed() {
		t.Fatal("barrier should be unsealed after Unseal")
	}

	want := []byte("hunter2")
	if err := b.Put(ctx, &physical.Entry{Key: "secret/db", Value: want}); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := b.Get(ctx, "secret/db")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || !bytes.Equal(got.Value, want) {
		t.Fatalf("get = %v, want %q", got, want)
	}

	// Encrypted at rest: the raw backend value must not contain the plaintext.
	raw, _ := be.Get(ctx, "secret/db")
	if raw == nil || bytes.Contains(raw.Value, want) {
		t.Fatal("value is not encrypted at rest")
	}

	// Seal -> reads and writes fail.
	b.Seal()
	if !b.IsSealed() {
		t.Fatal("barrier should be sealed after Seal")
	}
	if _, err := b.Get(ctx, "secret/db"); err != ErrSealed {
		t.Fatalf("get after seal = %v, want ErrSealed", err)
	}
	if err := b.Put(ctx, &physical.Entry{Key: "secret/x", Value: []byte("y")}); err != ErrSealed {
		t.Fatalf("put after seal = %v, want ErrSealed", err)
	}
}

func TestUnsealWrongKey(t *testing.T) {
	ctx := context.Background()
	b := New(physical.NewInMem())
	if err := b.Initialize(ctx, randKey(t)); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := b.Unseal(ctx, randKey(t)); err != ErrUnsealFailed {
		t.Fatalf("unseal wrong key = %v, want ErrUnsealFailed", err)
	}
}

func TestDoubleInitFails(t *testing.T) {
	ctx := context.Background()
	b := New(physical.NewInMem())
	rootKey := randKey(t)
	if err := b.Initialize(ctx, rootKey); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := b.Initialize(ctx, rootKey); err != ErrAlreadyInit {
		t.Fatalf("second init = %v, want ErrAlreadyInit", err)
	}
}
