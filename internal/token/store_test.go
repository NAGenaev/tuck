package token

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/physical"
)

// newTestStore creates a fully initialised token store backed by an
// in-memory backend.  It also returns the underlying InMem backend so
// callers can inspect raw storage keys.
func newTestStore(t *testing.T) (*Store, *physical.InMem) {
	t.Helper()
	mem := physical.NewInMem()
	b := barrier.New(mem)

	rootKey := make([]byte, 32)
	if _, err := rand.Read(rootKey); err != nil {
		t.Fatalf("gen root key: %v", err)
	}
	if err := b.Initialize(context.Background(), rootKey); err != nil {
		t.Fatalf("barrier init: %v", err)
	}
	if err := b.Unseal(context.Background(), rootKey); err != nil {
		t.Fatalf("barrier unseal: %v", err)
	}
	return NewStore(b), mem
}

// TestTokenIDNotInStorageKeys is the SEC-1 regression test:
// the raw bearer token ID must NOT appear as a bbolt (physical) key.
func TestTokenIDNotInStorageKeys(t *testing.T) {
	store, mem := newTestStore(t)
	ctx := context.Background()

	tok, err := Generate("sec1-test", []string{"root"}, 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	if err := store.Put(ctx, tok); err != nil {
		t.Fatalf("put token: %v", err)
	}

	// Inspect every key in the physical backend.
	keys, _ := mem.List(ctx, "")
	for _, k := range keys {
		if strings.Contains(k, tok.ID) {
			t.Errorf("SEC-1 VIOLATION: raw token ID found in storage key %q", k)
		}
	}
}

// TestTokenRoundTrip verifies that Get/Put/Delete/List work correctly after
// the storage key was changed to SHA-256(id).
func TestTokenRoundTrip(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	tok, err := Generate("rt-test", []string{"read"}, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	tok.Accessor = "tuck_acc_testacc"

	if err := store.Put(ctx, tok); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := store.Get(ctx, tok.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != tok.ID {
		t.Errorf("id mismatch: got %q, want %q", got.ID, tok.ID)
	}

	ids, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == tok.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("List() did not return token ID %q; got %v", tok.ID, ids)
	}

	if err := store.Delete(ctx, tok.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Get(ctx, tok.ID); err != ErrNotFound {
		t.Errorf("after delete: expected ErrNotFound, got %v", err)
	}
}

// TestGetByAccessorAfterHashing verifies accessor lookup still works.
func TestGetByAccessorAfterHashing(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	tok, _ := Generate("acc-test", []string{"root"}, 0)
	tok.Accessor = "tuck_acc_myaccessor"
	_ = store.Put(ctx, tok)

	got, err := store.GetByAccessor(ctx, tok.Accessor)
	if err != nil {
		t.Fatalf("GetByAccessor: %v", err)
	}
	if got.ID != tok.ID {
		t.Errorf("accessor lookup returned wrong token: got %q, want %q", got.ID, tok.ID)
	}
}
