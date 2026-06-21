package core

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/policy"
	"github.com/NAGenaev/tuck/internal/seal"
	"github.com/NAGenaev/tuck/internal/token"
)

// newTestCore returns an initialized, unsealed Core and the root token ID.
func newTestCore(t *testing.T) (*Core, string) {
	t.Helper()
	c := New(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	result, err := c.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return c, result.RootToken.ID
}

// --- Token lifecycle ---

func TestCreateAndLookupToken(t *testing.T) {
	c, root := newTestCore(t)
	ctx := context.Background()

	tok, err := c.CreateToken(ctx, "test", []string{"root"}, time.Hour)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if tok.ID == "" {
		t.Fatal("token ID is empty")
	}
	if tok.Accessor == "" {
		t.Fatal("token accessor is empty")
	}

	got, err := c.LookupToken(ctx, tok.ID)
	if err != nil {
		t.Fatalf("LookupToken: %v", err)
	}
	if got.ID != tok.ID {
		t.Errorf("LookupToken ID = %q, want %q", got.ID, tok.ID)
	}

	// Root token also lookupable.
	_, err = c.LookupToken(ctx, root)
	if err != nil {
		t.Errorf("LookupToken(root): %v", err)
	}
}

func TestLookupSelf(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	tok, _ := c.CreateToken(ctx, "self", []string{"root"}, time.Hour)
	got, err := c.LookupSelf(ctx, tok.ID)
	if err != nil {
		t.Fatalf("LookupSelf: %v", err)
	}
	if got.ID != tok.ID {
		t.Errorf("LookupSelf ID = %q, want %q", got.ID, tok.ID)
	}
}

func TestLookupNonExistentToken(t *testing.T) {
	c, _ := newTestCore(t)
	_, err := c.LookupToken(context.Background(), "tuck_doesnotexist")
	if err == nil {
		t.Error("expected error for non-existent token, got nil")
	}
}

func TestRevokeToken(t *testing.T) {
	c, root := newTestCore(t)
	ctx := context.Background()

	tok, _ := c.CreateToken(ctx, "rev", []string{"root"}, time.Hour)

	if err := c.RevokeToken(ctx, tok.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	// Token must be gone.
	_, err := c.LookupToken(ctx, tok.ID)
	if err == nil {
		t.Error("LookupToken after revoke: expected error, got nil")
	}

	// Root token must still be alive.
	if _, err := c.LookupToken(ctx, root); err != nil {
		t.Errorf("root token gone after unrelated revoke: %v", err)
	}
}

func TestRevokeTokenTree_Cascade(t *testing.T) {
	c, root := newTestCore(t)
	ctx := context.Background()

	// parent → child → grandchild
	parent, _ := c.CreateToken(ctx, "parent", []string{"root"}, time.Hour,
		WithParent(root))
	child, _ := c.CreateToken(ctx, "child", []string{"root"}, time.Hour,
		WithParent(parent.ID))
	grandchild, _ := c.CreateToken(ctx, "grandchild", []string{"root"}, time.Hour,
		WithParent(child.ID))

	if err := c.RevokeTokenTree(ctx, parent.ID); err != nil {
		t.Fatalf("RevokeTokenTree: %v", err)
	}

	for name, id := range map[string]string{
		"parent":      parent.ID,
		"child":       child.ID,
		"grandchild":  grandchild.ID,
	} {
		if _, err := c.LookupToken(ctx, id); err == nil {
			t.Errorf("%s still alive after cascade revoke", name)
		}
	}
}

func TestOrphanTokenSurvivesParentRevoke(t *testing.T) {
	c, root := newTestCore(t)
	ctx := context.Background()

	parent, _ := c.CreateToken(ctx, "parent", []string{"root"}, time.Hour,
		WithParent(root))
	orphan, _ := c.CreateToken(ctx, "orphan", []string{"root"}, time.Hour,
		WithOrphan())

	if err := c.RevokeTokenTree(ctx, parent.ID); err != nil {
		t.Fatalf("RevokeTokenTree: %v", err)
	}

	// Orphan has no parent link, must survive.
	if _, err := c.LookupToken(ctx, orphan.ID); err != nil {
		t.Errorf("orphan token unexpectedly gone after unrelated revoke: %v", err)
	}
}

// --- Accessor operations ---

func TestLookupAndRevokeByAccessor(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	tok, _ := c.CreateToken(ctx, "acc", []string{"root"}, time.Hour)
	accessor := tok.Accessor

	got, err := c.LookupTokenByAccessor(ctx, accessor)
	if err != nil {
		t.Fatalf("LookupTokenByAccessor: %v", err)
	}
	if got.ID != tok.ID {
		t.Errorf("accessor lookup returned %q, want %q", got.ID, tok.ID)
	}

	if err := c.RevokeTokenByAccessor(ctx, accessor); err != nil {
		t.Fatalf("RevokeTokenByAccessor: %v", err)
	}
	if _, err := c.LookupToken(ctx, tok.ID); err == nil {
		t.Error("token still alive after revoke-by-accessor")
	}
}

// --- Renewal ---

func TestRenewToken_Renewable(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	tok, _ := c.CreateToken(ctx, "r", []string{"root"}, 100*time.Millisecond,
		WithRenewable(0))

	renewed, err := c.RenewToken(ctx, tok.ID, 10*time.Second)
	if err != nil {
		t.Fatalf("RenewToken: %v", err)
	}
	if !renewed.ExpiresAt.After(time.Now().Add(5 * time.Second)) {
		t.Errorf("renewed ExpiresAt %v is not far enough in the future", renewed.ExpiresAt)
	}

	// Wait past original TTL — token should still exist.
	time.Sleep(150 * time.Millisecond)
	if _, err := c.LookupToken(ctx, tok.ID); err != nil {
		t.Errorf("token gone after renewal + original TTL elapsed: %v", err)
	}
}

func TestRenewToken_NotRenewable(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	tok, _ := c.CreateToken(ctx, "nr", []string{"root"}, time.Hour) // no WithRenewable

	_, err := c.RenewToken(ctx, tok.ID, time.Hour)
	if err == nil {
		t.Fatal("RenewToken on non-renewable token: expected error, got nil")
	}
}

func TestRenewToken_MaxTTLCap(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	maxTTL := 500 * time.Millisecond
	tok, _ := c.CreateToken(ctx, "cap", []string{"root"}, 100*time.Millisecond,
		WithRenewable(maxTTL))

	renewed, err := c.RenewToken(ctx, tok.ID, 10*time.Second) // request >> maxTTL
	if err != nil {
		t.Fatalf("RenewToken: %v", err)
	}
	cap := tok.CreatedAt.Add(maxTTL)
	// Allow 50ms clock slack.
	if renewed.ExpiresAt.After(cap.Add(50 * time.Millisecond)) {
		t.Errorf("renewed ExpiresAt %v exceeds MaxTTL cap %v", renewed.ExpiresAt, cap)
	}
}

func TestRenewSelf(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	tok, _ := c.CreateToken(ctx, "rs", []string{"root"}, 100*time.Millisecond,
		WithRenewable(0))

	if _, err := c.RenewSelf(ctx, tok.ID, 10*time.Second); err != nil {
		t.Fatalf("RenewSelf: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	if _, err := c.LookupToken(ctx, tok.ID); err != nil {
		t.Errorf("token gone after RenewSelf: %v", err)
	}
}

func TestRenewExpiredToken(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	tok, _ := c.CreateToken(ctx, "exp", []string{"root"}, 50*time.Millisecond,
		WithRenewable(0))

	time.Sleep(100 * time.Millisecond) // let it expire

	_, err := c.RenewToken(ctx, tok.ID, time.Hour)
	if err == nil {
		t.Fatal("RenewToken on expired token: expected error, got nil")
	}
}

// --- MaxUses / TrackUse ---

func TestMaxUses_TokenRevokedAfterExhaustion(t *testing.T) {
	c, root := newTestCore(t)
	ctx := context.Background()

	tok, _ := c.CreateToken(ctx, "mu", []string{"root"}, time.Hour,
		WithMaxUses(2))

	// Two uses should succeed.
	for i := range 2 {
		if err := c.TrackUse(ctx, tok.ID); err != nil {
			t.Fatalf("TrackUse #%d: %v", i+1, err)
		}
	}

	// Third use must fail because the token is auto-revoked.
	if err := c.TrackUse(ctx, tok.ID); err == nil {
		t.Error("TrackUse after max_uses exhaustion: expected error, got nil")
	}

	// Token must be gone.
	if _, err := c.LookupToken(ctx, tok.ID); err == nil {
		t.Error("token still alive after max_uses exhaustion")
	}

	// Root unaffected.
	if _, err := c.LookupToken(ctx, root); err != nil {
		t.Errorf("root token gone after unrelated exhaustion: %v", err)
	}
}

// --- Authenticate ---

func TestAuthenticate_ValidToken(t *testing.T) {
	c, root := newTestCore(t)

	tok, err := c.Authenticate(context.Background(), root)
	if err != nil {
		t.Fatalf("Authenticate(root): %v", err)
	}
	if tok.ID != root {
		t.Errorf("Authenticate returned ID %q, want %q", tok.ID, root)
	}
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	c, _ := newTestCore(t)

	_, err := c.Authenticate(context.Background(), "tuck_bogus")
	if err == nil {
		t.Error("Authenticate(bogus): expected error, got nil")
	}
}

func TestAuthenticate_ExpiredToken(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	tok, _ := c.CreateToken(ctx, "exp", []string{"root"}, 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	_, err := c.Authenticate(ctx, tok.ID)
	if err == nil {
		t.Error("Authenticate(expired): expected error, got nil")
	}
}

// --- ACL / EnforceAccess ---

func TestEnforceAccess_RootAlwaysAllowed(t *testing.T) {
	c, root := newTestCore(t)
	ctx := context.Background()

	for _, path := range []string{"secret/anything", "auth/token", "sys/rotate"} {
		for _, cap := range []policy.Capability{policy.CapRead, policy.CapWrite, policy.CapDelete, policy.CapList} {
			if err := c.EnforceAccess(ctx, root, path, cap); err != nil {
				t.Errorf("root denied %v on %s: %v", cap, path, err)
			}
		}
	}
}

func TestEnforceAccess_PolicyAllowsAndDenies(t *testing.T) {
	c, root := newTestCore(t)
	ctx := context.Background()

	p := &policy.Policy{
		Name: "kv-readonly",
		Rules: []policy.Rule{
			{Path: "secret/data/*", Capabilities: policy.CapRead | policy.CapList},
		},
	}
	if err := c.PutPolicy(ctx, "", p); err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}

	tok, _ := c.CreateToken(ctx, "ro", []string{"kv-readonly"}, time.Hour)

	// Allowed reads.
	if err := c.EnforceAccess(ctx, tok.ID, "secret/data/foo", policy.CapRead); err != nil {
		t.Errorf("read on allowed path: %v", err)
	}
	if err := c.EnforceAccess(ctx, tok.ID, "secret/data/bar", policy.CapList); err != nil {
		t.Errorf("list on allowed path: %v", err)
	}

	// Denied write.
	if err := c.EnforceAccess(ctx, tok.ID, "secret/data/foo", policy.CapWrite); err == nil {
		t.Error("write on read-only path: expected denied, got nil")
	}

	// Denied path.
	if err := c.EnforceAccess(ctx, tok.ID, "secret/other/key", policy.CapRead); err == nil {
		t.Error("read on forbidden path: expected denied, got nil")
	}

	// Root always allowed regardless.
	if err := c.EnforceAccess(ctx, root, "secret/data/foo", policy.CapWrite); err != nil {
		t.Errorf("root denied write: %v", err)
	}
}

func TestEnforceAccess_RevokedTokenDenied(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	tok, _ := c.CreateToken(ctx, "rev", []string{"root"}, time.Hour)
	_ = c.RevokeToken(ctx, tok.ID)

	if err := c.EnforceAccess(ctx, tok.ID, "secret/anything", policy.CapRead); err == nil {
		t.Error("revoked token granted access: expected denied")
	}
}

// --- Policy CRUD ---

func TestPolicyCRUD(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	p := &policy.Policy{
		Name:  "mypol",
		Rules: []policy.Rule{{Path: "secret/*", Capabilities: policy.CapRead}},
	}

	if err := c.PutPolicy(ctx, "", p); err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}

	got, err := c.GetPolicy(ctx, "", "mypol")
	if err != nil {
		t.Fatalf("GetPolicy: %v", err)
	}
	if got.Name != "mypol" {
		t.Errorf("GetPolicy name = %q, want mypol", got.Name)
	}

	names, err := c.ListPolicies(ctx, "")
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	found := false
	for _, n := range names {
		if n == "mypol" {
			found = true
		}
	}
	if !found {
		t.Errorf("ListPolicies missing mypol; got %v", names)
	}

	if err := c.DeletePolicy(ctx, "", "mypol"); err != nil {
		t.Fatalf("DeletePolicy: %v", err)
	}
	if _, err := c.GetPolicy(ctx, "", "mypol"); err == nil {
		t.Error("GetPolicy after delete: expected error, got nil")
	}
}

// --- Secret CRUD ---

func TestSecretCRUD(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	if err := c.PutSecret(ctx, "", "myapp/db-pass", []byte("s3cr3t")); err != nil {
		t.Fatalf("PutSecret: %v", err)
	}

	val, ok, err := c.GetSecret(ctx, "", "myapp/db-pass")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if !ok {
		t.Fatal("GetSecret: secret not found")
	}
	if string(val) != "s3cr3t" {
		t.Errorf("GetSecret = %q, want s3cr3t", val)
	}

	// List should include the key.
	keys, err := c.ListSecrets(ctx, "", "myapp/")
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	found := false
	for _, k := range keys {
		if k == "db-pass" {
			found = true
		}
	}
	if !found {
		t.Errorf("ListSecrets missing db-pass; got %v", keys)
	}

	// Delete.
	if err := c.DeleteSecret(ctx, "", "myapp/db-pass"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	_, ok, err = c.GetSecret(ctx, "", "myapp/db-pass")
	if err != nil {
		t.Fatalf("GetSecret after delete: %v", err)
	}
	if ok {
		t.Error("GetSecret after delete: expected not found, got ok=true")
	}
}

func TestSecretOverwrite(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	_ = c.PutSecret(ctx, "", "key", []byte("v1"))
	_ = c.PutSecret(ctx, "", "key", []byte("v2"))

	val, _, _ := c.GetSecret(ctx, "", "key")
	if string(val) != "v2" {
		t.Errorf("overwrite: got %q, want v2", val)
	}
}

// --- Token roles ---

func TestTokenRole_CreateAndUse(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := context.Background()

	role := &token.Role{
		Name:      "myrole",
		Policies:  []string{"root"},
		TTL:       time.Hour,
		Renewable: true,
		MaxTTL:    24 * time.Hour,
	}
	if err := c.PutTokenRole(ctx, role); err != nil {
		t.Fatalf("PutTokenRole: %v", err)
	}

	tok, err := c.CreateTokenFromRole(ctx, "myrole", "my-app")
	if err != nil {
		t.Fatalf("CreateTokenFromRole: %v", err)
	}
	if !tok.Renewable {
		t.Error("token from renewable role must be renewable")
	}

	names, _ := c.ListTokenRoles(ctx)
	found := false
	for _, n := range names {
		if n == "myrole" {
			found = true
		}
	}
	if !found {
		t.Errorf("ListTokenRoles missing myrole; got %v", names)
	}

	if err := c.DeleteTokenRole(ctx, "myrole"); err != nil {
		t.Fatalf("DeleteTokenRole: %v", err)
	}
	if _, err := c.GetTokenRole(ctx, "myrole"); err == nil {
		t.Error("GetTokenRole after delete: expected error, got nil")
	}
	// Token issued from role remains valid.
	if _, err := c.LookupToken(ctx, tok.ID); err != nil {
		t.Errorf("token from deleted role should still be valid: %v", err)
	}
}

// --- SealStatus ---

func TestSealStatus_Unsealed(t *testing.T) {
	c, _ := newTestCore(t)
	s := c.SealStatus()
	if s.Sealed {
		t.Error("SealStatus: core should be unsealed after Start")
	}
}

func TestSealed_FalseAfterStart(t *testing.T) {
	c, _ := newTestCore(t)
	if c.Sealed() {
		t.Error("Sealed() = true after Start, want false")
	}
}
