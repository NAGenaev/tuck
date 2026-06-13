package github_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/auth/github"
	"github.com/NAGenaev/tuck/internal/physical"
)

func newStore(t *testing.T) *github.Store {
	t.Helper()
	return github.NewStore(physical.NewInMem())
}

func TestStorePutGetRole(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	r := &github.Role{
		Name:       "ci",
		Repository: "myorg/myrepo",
		Ref:        "refs/heads/main",
		Policies:   []string{"ci-read"},
		TTL:        time.Hour,
	}
	if err := s.PutRole(ctx, r); err != nil {
		t.Fatalf("PutRole: %v", err)
	}

	got, err := s.GetRole(ctx, "ci")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if got.Name != r.Name || got.Repository != r.Repository || got.Ref != r.Ref {
		t.Errorf("GetRole mismatch: got %+v", got)
	}
	if len(got.Policies) != 1 || got.Policies[0] != "ci-read" {
		t.Errorf("policies mismatch: %v", got.Policies)
	}
}

func TestStoreGetMissing(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	_, err := s.GetRole(ctx, "nonexistent")
	if !errors.Is(err, github.ErrRoleNotFound) {
		t.Fatalf("want ErrRoleNotFound, got %v", err)
	}
}

func TestStoreDeleteRole(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	r := &github.Role{Name: "todelete", Policies: []string{"x"}}
	if err := s.PutRole(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRole(ctx, "todelete"); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
	_, err := s.GetRole(ctx, "todelete")
	if !errors.Is(err, github.ErrRoleNotFound) {
		t.Fatalf("after delete: want ErrRoleNotFound, got %v", err)
	}
}

func TestStoreListRoles(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	names := []string{"alpha", "beta", "gamma"}
	for _, n := range names {
		if err := s.PutRole(ctx, &github.Role{Name: n, Policies: []string{"p"}}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListRoles(ctx)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	gotSet := make(map[string]bool, len(got))
	for _, n := range got {
		gotSet[n] = true
	}
	for _, n := range names {
		if !gotSet[n] {
			t.Errorf("ListRoles missing %q; got %v", n, got)
		}
	}
}

func TestStorePutUpdatesRole(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	r := &github.Role{Name: "myrole", Ref: "refs/heads/main", Policies: []string{"old"}}
	if err := s.PutRole(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.Policies = []string{"new"}
	r.Ref = "refs/heads/dev"
	if err := s.PutRole(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRole(ctx, "myrole")
	if err != nil {
		t.Fatal(err)
	}
	if got.Ref != "refs/heads/dev" || len(got.Policies) != 1 || got.Policies[0] != "new" {
		t.Errorf("update not persisted: %+v", got)
	}
}
