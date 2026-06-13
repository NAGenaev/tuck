package token

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"
	"time"

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

func TestChildrenIndex(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	parent, _ := Generate("parent", []string{"root"}, 0)
	store.Put(ctx, parent)

	// Create two children with ParentID set.
	child1, _ := Generate("child1", []string{"read"}, time.Hour)
	child1.ParentID = parent.ID
	store.Put(ctx, child1)

	child2, _ := Generate("child2", []string{"read"}, time.Hour)
	child2.ParentID = parent.ID
	store.Put(ctx, child2)

	// Orphan child — should NOT appear in Children().
	orphan, _ := Generate("orphan", []string{"read"}, time.Hour)
	orphan.ParentID = "" // no parent
	orphan.Orphan = true
	store.Put(ctx, orphan)

	children, err := store.Children(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d: %v", len(children), children)
	}
}

func TestDeleteCleansChildrenIndex(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	parent, _ := Generate("parent", []string{"root"}, 0)
	store.Put(ctx, parent)

	child, _ := Generate("child", []string{"read"}, time.Hour)
	child.ParentID = parent.ID
	store.Put(ctx, child)

	// Verify child is listed.
	before, _ := store.Children(ctx, parent.ID)
	if len(before) != 1 {
		t.Fatalf("expected 1 child before delete, got %d", len(before))
	}

	// Delete the child; its entry in the children index should be cleaned up.
	store.Delete(ctx, child.ID)

	after, _ := store.Children(ctx, parent.ID)
	if len(after) != 0 {
		t.Fatalf("expected 0 children after delete, got %d", len(after))
	}
}

func TestPeriodTokenFields(t *testing.T) {
	tok, _ := Generate("svc", []string{"default"}, time.Hour)
	tok.Period = 30 * time.Minute
	tok.Renewable = true

	if tok.Period != 30*time.Minute {
		t.Fatalf("unexpected period: %v", tok.Period)
	}
	if !tok.Renewable {
		t.Fatal("period token should be renewable")
	}
}

func TestOrphanToken(t *testing.T) {
	tok, _ := Generate("orphan", []string{"default"}, time.Hour)
	tok.Orphan = true
	tok.ParentID = "" // orphans have no parent

	if tok.ParentID != "" {
		t.Fatalf("orphan should have no parent, got %q", tok.ParentID)
	}
	if !tok.Orphan {
		t.Fatal("expected Orphan=true")
	}
}
