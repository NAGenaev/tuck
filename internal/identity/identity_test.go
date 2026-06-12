package identity_test

import (
	"context"
	"testing"

	"github.com/NAGenaev/tuck/internal/identity"
	"github.com/NAGenaev/tuck/internal/physical"
)

func newStore() *identity.Store {
	return identity.NewStore(physical.NewInMem())
}

func TestEntityCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore()

	e, err := s.CreateEntity(ctx, "alice", []string{"read"}, map[string]string{"dept": "eng"})
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "alice" {
		t.Errorf("name = %q, want alice", e.Name)
	}

	got, err := s.GetEntityByID(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "alice" {
		t.Errorf("GetByID name = %q", got.Name)
	}

	got2, err := s.GetEntityByName(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if got2.ID != e.ID {
		t.Errorf("GetByName ID mismatch")
	}

	ids, err := s.ListEntityIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Errorf("list len = %d, want 1", len(ids))
	}

	if err := s.DeleteEntity(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetEntityByID(ctx, e.ID); err == nil {
		t.Error("expected ErrEntityNotFound after delete")
	}
}

func TestAliasCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore()

	e, _ := s.CreateEntity(ctx, "bob", nil, nil)

	a, err := s.CreateAlias(ctx, e.ID, "auth_approle", "my-role", nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAliasByID(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EntityID != e.ID {
		t.Errorf("alias entity_id mismatch")
	}

	got2, err := s.GetAliasByMount(ctx, "auth_approle", "my-role")
	if err != nil {
		t.Fatal(err)
	}
	if got2.ID != a.ID {
		t.Errorf("GetAliasByMount ID mismatch")
	}

	if err := s.DeleteAlias(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAliasByID(ctx, a.ID); err == nil {
		t.Error("expected ErrAliasNotFound after delete")
	}
}

func TestEnsureAlias(t *testing.T) {
	ctx := context.Background()
	s := newStore()

	// First call: creates entity + alias.
	e1, err := s.EnsureAlias(ctx, "auth_ldap", "carol")
	if err != nil {
		t.Fatal(err)
	}
	if e1 == nil {
		t.Fatal("nil entity")
	}

	// Second call: same entity returned.
	e2, err := s.EnsureAlias(ctx, "auth_ldap", "carol")
	if err != nil {
		t.Fatal(err)
	}
	if e1.ID != e2.ID {
		t.Errorf("EnsureAlias returned different entity on second call: %q vs %q", e1.ID, e2.ID)
	}
}

func TestGroupCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore()

	g, err := s.CreateGroup(ctx, "engineering", []string{"dev"}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetGroupByID(ctx, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "engineering" {
		t.Errorf("name = %q", got.Name)
	}

	got2, err := s.GetGroupByName(ctx, "engineering")
	if err != nil {
		t.Fatal(err)
	}
	if got2.ID != g.ID {
		t.Errorf("GetGroupByName ID mismatch")
	}

	if err := s.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetGroupByID(ctx, g.ID); err == nil {
		t.Error("expected ErrGroupNotFound after delete")
	}
}

func TestResolveEntityPolicies(t *testing.T) {
	ctx := context.Background()
	s := newStore()

	// Entity with direct policy.
	e, _ := s.CreateEntity(ctx, "dave", []string{"entity-pol"}, nil)

	// Group that entity belongs to.
	g, _ := s.CreateGroup(ctx, "team-a", []string{"group-pol"}, []string{e.ID}, nil, nil)
	_ = g

	// Parent group that includes team-a.
	_, _ = s.CreateGroup(ctx, "all-staff", []string{"staff-pol"}, nil, []string{g.ID}, nil)

	policies := s.ResolveEntityPolicies(ctx, e.ID)
	pset := make(map[string]bool)
	for _, p := range policies {
		pset[p] = true
	}
	for _, want := range []string{"entity-pol", "group-pol", "staff-pol"} {
		if !pset[want] {
			t.Errorf("policy %q not in resolved set %v", want, policies)
		}
	}
}

func TestDisabledEntityReturnsNoPolicies(t *testing.T) {
	ctx := context.Background()
	s := newStore()

	e, _ := s.CreateEntity(ctx, "eve", []string{"secret-pol"}, nil)
	e.Disabled = true
	_ = s.PutEntity(ctx, e)

	policies := s.ResolveEntityPolicies(ctx, e.ID)
	if len(policies) != 0 {
		t.Errorf("expected no policies for disabled entity, got %v", policies)
	}
}

func TestGroupAliasCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore()

	g, _ := s.CreateGroup(ctx, "devs", []string{"dev-pol"}, nil, nil, nil)

	ga, err := s.CreateGroupAlias(ctx, g.ID, "auth_ldap", "cn=devs,ou=groups,dc=example,dc=com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ga.GroupID != g.ID {
		t.Errorf("group_id mismatch")
	}

	got, err := s.GetGroupAliasByID(ctx, ga.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != ga.Name {
		t.Errorf("name mismatch: %q vs %q", got.Name, ga.Name)
	}

	ids, err := s.ListGroupAliasIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Errorf("list len = %d, want 1", len(ids))
	}

	if err := s.DeleteGroupAlias(ctx, ga.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetGroupAliasByID(ctx, ga.ID); err == nil {
		t.Error("expected ErrGroupAliasNotFound after delete")
	}
}

func TestResolveExternalGroupPolicies(t *testing.T) {
	ctx := context.Background()
	s := newStore()

	// Create Tuck group hierarchy: parent → child.
	child, _ := s.CreateGroup(ctx, "ldap-devs", []string{"dev-pol"}, nil, nil, nil)
	parent, _ := s.CreateGroup(ctx, "ldap-all", []string{"all-pol"}, nil, []string{child.ID}, nil)
	_ = parent

	// Map external LDAP group DN → child Tuck group.
	_, _ = s.CreateGroupAlias(ctx, child.ID, "auth_ldap", "cn=devs,ou=groups,dc=example,dc=com", nil)

	// Resolve: external group belongs to child → picks up child + parent policies.
	policies := s.ResolveExternalGroupPolicies(ctx, "auth_ldap", []string{"cn=devs,ou=groups,dc=example,dc=com"})
	pset := make(map[string]bool)
	for _, p := range policies {
		pset[p] = true
	}
	for _, want := range []string{"dev-pol", "all-pol"} {
		if !pset[want] {
			t.Errorf("policy %q not in resolved set %v", want, policies)
		}
	}

	// Unmapped external group → no policies.
	none := s.ResolveExternalGroupPolicies(ctx, "auth_ldap", []string{"cn=unknown,ou=groups,dc=example,dc=com"})
	if len(none) != 0 {
		t.Errorf("expected no policies for unmapped group, got %v", none)
	}
}
