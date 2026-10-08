package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

// failingBackend lets N writes through after it is armed, then fails every write.
type failingBackend struct {
	physical.Backend
	armed bool
	left  int
}

func (f *failingBackend) Put(ctx context.Context, e *physical.Entry) error {
	if f.armed {
		if f.left <= 0 {
			return errors.New("disk full")
		}
		f.left--
	}
	return f.Backend.Put(ctx, e)
}

type nopReviewer struct{}

func (nopReviewer) Review(string) (*k8sauth.ReviewResult, error) { return &k8sauth.ReviewResult{}, nil }

// A failure in the last step (the Kubernetes role) must leave nothing behind:
// no namespace, no policies, no LDAP role.
func TestOnboardTeamRollsBackOnLateFailure(t *testing.T) {
	fb := &failingBackend{Backend: physical.NewInMem()}
	c := New(fb, seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	if _, err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.AddK8sCluster("c1", nopReviewer{}); err != nil {
		t.Fatal(err)
	}
	spec := TeamSpec{
		Name:   "rb",
		Groups: map[string][]string{AccessReader: {"rb-readers"}},
		K8s:    []TeamK8sBinding{{Cluster: "c1", Namespace: "app", ServiceAccount: "sa", Access: AccessReader}},
	}

	// Writes in order: namespace, policy, LDAP role, then the K8s role fails.
	fb.armed, fb.left = true, 3
	_, err := c.OnboardTeam(context.Background(), spec)
	fb.armed = false
	if err == nil {
		t.Fatal("onboarding should have failed")
	}
	ctx := context.Background()
	if _, gerr := c.GetNamespace(ctx, "rb"); gerr == nil {
		t.Error("namespace left behind")
	}
	if roles, _ := c.ListLDAPRoles(ctx); len(roles) != 0 {
		t.Errorf("LDAP role left behind: %v", roles)
	}
	if _, gerr := c.GetPolicy(ctx, "rb", "team-reader"); gerr == nil {
		t.Error("policy left behind")
	}
	if _, gerr := c.GetK8sRole(ctx, "c1", "app", "sa"); gerr == nil {
		t.Error("k8s role left behind")
	}

	// The same spec succeeds once the storage works again.
	if _, err := c.OnboardTeam(ctx, spec); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
}
