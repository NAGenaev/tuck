package token_test

import (
	"context"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/token"
)

func TestTokenRoleStore(t *testing.T) {
	ctx := context.Background()
	s := token.NewRoleStore(physical.NewInMem())

	// Put
	r := &token.Role{
		Name:      "ci",
		Policies:  []string{"ci-read"},
		TTL:       4 * time.Hour,
		MaxTTL:    24 * time.Hour,
		Renewable: true,
	}
	if err := s.Put(ctx, r); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get
	got, err := s.Get(ctx, "ci")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "ci" || got.TTL != 4*time.Hour || !got.Renewable {
		t.Errorf("unexpected role: %+v", got)
	}

	// List
	if err := s.Put(ctx, &token.Role{Name: "admin"}); err != nil {
		t.Fatalf("Put admin: %v", err)
	}
	names, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("List = %v, want 2 entries", names)
	}

	// Delete
	if err := s.Delete(ctx, "ci"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Get(ctx, "ci")
	if err == nil {
		t.Error("expected ErrRoleNotFound after delete")
	}
}
