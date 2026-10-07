package operator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// ---- Mock KubeClient ----

type mockKube struct {
	mu       sync.Mutex
	items    []TuckSecret
	rv       string
	events   []WatchEvent
	applied  []*KubeSecret
	statuses []TuckSecret

	nsAnnotations map[string]map[string]string
	nsErr         error

	// applyChanged controls what ApplySecret reports for "changed", default
	// true (unset) so existing tests that don't care keep passing.
	applyUnchanged bool
	restarted      []RestartTarget
	restartErr     error
}

func (m *mockKube) NamespaceAnnotations(_ context.Context, ns string) (map[string]string, error) {
	if m.nsErr != nil {
		return nil, m.nsErr
	}
	return m.nsAnnotations[ns], nil
}

func (m *mockKube) List(_ context.Context, _ string) (*TuckSecretList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := &TuckSecretList{Items: append([]TuckSecret(nil), m.items...)}
	list.Metadata.ResourceVersion = m.rv
	return list, nil
}

func (m *mockKube) Watch(_ context.Context, _, _ string) (<-chan WatchEvent, error) {
	ch := make(chan WatchEvent, len(m.events)+1)
	for _, ev := range m.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func (m *mockKube) ApplySecret(_ context.Context, s *KubeSecret) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.applied = append(m.applied, s)
	return !m.applyUnchanged, nil
}

func (m *mockKube) RestartWorkload(_ context.Context, _ string, target RestartTarget, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.restartErr != nil {
		return m.restartErr
	}
	m.restarted = append(m.restarted, target)
	return nil
}

func (m *mockKube) UpdateStatus(_ context.Context, ts *TuckSecret) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses = append(m.statuses, *ts)
	return nil
}

func (m *mockKube) DeleteSecret(_ context.Context, _, _ string) error { return nil }

func (m *mockKube) AddFinalizer(_ context.Context, _ *TuckSecret, _ string) error { return nil }

func (m *mockKube) RemoveFinalizer(_ context.Context, _ *TuckSecret, _ string) error { return nil }

// ---- Mock TuckClient ----

type mockTuck struct {
	mu         sync.Mutex
	secrets    map[string][]byte
	fetched    []string
	kvVersions []string
}

func (m *mockTuck) GetSecret(_ context.Context, path, kvVersion string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetched = append(m.fetched, path)
	m.kvVersions = append(m.kvVersions, kvVersion)
	v, ok := m.secrets[path]
	if !ok {
		return nil, nil
	}
	return v, nil
}

// ---- Helpers ----

func newTS(ns, name, tuckPath, secretName, secretKey string) TuckSecret {
	return TuckSecret{
		APIVersion: "tuck.io/v1alpha1",
		Kind:       "TuckSecret",
		Metadata:   ObjectMeta{Name: name, Namespace: ns, ResourceVersion: "1"},
		Spec: TuckSecretSpec{
			TuckPath:   tuckPath,
			SecretName: secretName,
			SecretKey:  secretKey,
		},
	}
}

// ---- Tests ----

func TestReconcile_BasicSync(t *testing.T) {
	ts := newTS("prod", "db-pass", "db/password", "db-creds", "password")
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"db/password": []byte("s3cr3t")}}

	ctrl := New(kube, tuck, "prod")
	ctx := context.Background()

	if err := ctrl.reconcile(ctx, ts); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}
	if len(kube.applied) != 1 {
		t.Fatalf("expected 1 applied secret, got %d", len(kube.applied))
	}
	s := kube.applied[0]
	if s.Metadata.Name != "db-creds" {
		t.Errorf("expected secret name db-creds, got %q", s.Metadata.Name)
	}
	if s.Metadata.Namespace != "prod" {
		t.Errorf("expected namespace prod, got %q", s.Metadata.Namespace)
	}
	if got := string(s.Data["password"]); got != "s3cr3t" {
		t.Errorf("expected value s3cr3t, got %q", got)
	}
	if s.Metadata.Labels["managed-by"] != "tuck-operator" {
		t.Errorf("expected managed-by label, got %v", s.Metadata.Labels)
	}
}

func TestReconcile_KVVersionDefaultsToV1(t *testing.T) {
	ts := newTS("prod", "db-pass", "db/password", "db-creds", "password")
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"db/password": []byte("s3cr3t")}}

	ctrl := New(kube, tuck, "prod")
	if err := ctrl.reconcile(context.Background(), ts); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}
	if len(tuck.kvVersions) != 1 || tuck.kvVersions[0] != "1" {
		t.Fatalf("expected kvVersion \"1\" (default), got %v", tuck.kvVersions)
	}
}

func TestReconcile_KVVersionV2Passthrough(t *testing.T) {
	ts := newTS("prod", "db-pass", "db", "db-creds", "password")
	ts.Spec.KVVersion = "2"
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"db": []byte(`{"dsn":"..."}`)}}

	ctrl := New(kube, tuck, "prod")
	if err := ctrl.reconcile(context.Background(), ts); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}
	if len(tuck.kvVersions) != 1 || tuck.kvVersions[0] != "2" {
		t.Fatalf("expected kvVersion \"2\" to pass through, got %v", tuck.kvVersions)
	}
}

func TestReconcile_RestartTargets_TriggersOnChange(t *testing.T) {
	ts := newTS("prod", "db-pass", "db/password", "db-creds", "password")
	ts.Spec.RestartTargets = []RestartTarget{{Kind: "Deployment", Name: "api"}, {Kind: "StatefulSet", Name: "worker"}}
	kube := &mockKube{} // applyUnchanged=false (zero value) => ApplySecret reports changed=true
	tuck := &mockTuck{secrets: map[string][]byte{"db/password": []byte("s3cr3t")}}

	ctrl := New(kube, tuck, "prod")
	ctrl.EnableRestartTargets(true)
	if err := ctrl.reconcile(context.Background(), ts); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}
	if len(kube.restarted) != 2 {
		t.Fatalf("expected 2 restart targets bumped, got %v", kube.restarted)
	}
}

func TestReconcile_RestartTargets_SkippedWhenValueUnchanged(t *testing.T) {
	ts := newTS("prod", "db-pass", "db/password", "db-creds", "password")
	ts.Spec.RestartTargets = []RestartTarget{{Kind: "Deployment", Name: "api"}}
	kube := &mockKube{applyUnchanged: true} // a periodic refresh that found the same value
	tuck := &mockTuck{secrets: map[string][]byte{"db/password": []byte("s3cr3t")}}

	ctrl := New(kube, tuck, "prod")
	ctrl.EnableRestartTargets(true)
	if err := ctrl.reconcile(context.Background(), ts); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}
	if len(kube.restarted) != 0 {
		t.Fatalf("expected no restarts for an unchanged value, got %v", kube.restarted)
	}
}

func TestReconcile_RestartTargets_DisabledByDefault(t *testing.T) {
	ts := newTS("prod", "db-pass", "db/password", "db-creds", "password")
	ts.Spec.RestartTargets = []RestartTarget{{Kind: "Deployment", Name: "api"}}
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"db/password": []byte("s3cr3t")}}

	ctrl := New(kube, tuck, "prod") // EnableRestartTargets never called
	if err := ctrl.reconcile(context.Background(), ts); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}
	if len(kube.restarted) != 0 {
		t.Fatalf("expected restarts to stay off by default, got %v", kube.restarted)
	}
}

func TestReconcile_RestartTargets_BestEffortOnError(t *testing.T) {
	ts := newTS("prod", "db-pass", "db/password", "db-creds", "password")
	ts.Spec.RestartTargets = []RestartTarget{{Kind: "Deployment", Name: "api"}}
	kube := &mockKube{restartErr: errors.New("forbidden")}
	tuck := &mockTuck{secrets: map[string][]byte{"db/password": []byte("s3cr3t")}}

	ctrl := New(kube, tuck, "prod")
	ctrl.EnableRestartTargets(true)
	// A restart-patch failure must not fail the reconcile -- the Secret sync
	// already succeeded and is the thing status.Synced reports on.
	if err := ctrl.reconcile(context.Background(), ts); err != nil {
		t.Fatalf("reconcile should succeed despite a restart error, got: %v", err)
	}
	if len(kube.applied) != 1 {
		t.Fatalf("expected the secret to still be applied, got %d", len(kube.applied))
	}
}

func TestReconcile_MissingSpec(t *testing.T) {
	ts := TuckSecret{Metadata: ObjectMeta{Name: "x", Namespace: "ns"}}
	ctrl := New(&mockKube{}, &mockTuck{}, "")
	if err := ctrl.reconcile(context.Background(), ts); err == nil {
		t.Fatal("expected error for missing spec fields")
	}
}

func TestRunOnce_ListAndSync(t *testing.T) {
	ts := newTS("ns", "app-secret", "app/key", "app-k8s-secret", "key")
	kube := &mockKube{
		items: []TuckSecret{ts},
		rv:    "42",
		// Empty events: watch channel is closed immediately so runOnce returns.
	}
	tuck := &mockTuck{secrets: map[string][]byte{"app/key": []byte("val")}}
	ctrl := New(kube, tuck, "ns")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// runOnce should return after watch channel closes (not block).
	if err := ctrl.runOnce(ctx); err != nil && ctx.Err() == nil {
		t.Fatalf("runOnce error: %v", err)
	}
	if len(kube.applied) == 0 {
		t.Error("expected at least one applied secret")
	}
}

func TestHandleEvent_Added(t *testing.T) {
	ts := newTS("ns", "ev-secret", "path/to/secret", "k8s-secret", "data")
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"path/to/secret": []byte("hello")}}
	ctrl := New(kube, tuck, "ns")

	ev := WatchEvent{Type: "ADDED", Object: ts}
	if err := ctrl.handleEvent(context.Background(), ev); err != nil {
		t.Fatalf("handleEvent ADDED error: %v", err)
	}
	if len(kube.applied) != 1 {
		t.Fatalf("expected 1 applied secret after ADDED, got %d", len(kube.applied))
	}
}

// TestHandleEvent_ModifiedEchoWithoutSpecChange_SkipsReconcile guards
// against a self-triggering feedback loop: reconcile's status write bumps
// resourceVersion, which the watch echoes back as another MODIFIED event
// for the very same, unchanged spec. Without a spec-change check that echo
// would trigger another reconcile, another status write, another echo —
// an unbounded MODIFIED/UpdateStatus loop with no backoff. Only a genuine
// spec change (or first sighting of a resource) should reconcile.
func TestHandleEvent_ModifiedEchoWithoutSpecChange_SkipsReconcile(t *testing.T) {
	ts := newTS("ns", "ev-secret", "path/to/secret", "k8s-secret", "data")
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"path/to/secret": []byte("hello")}}
	ctrl := New(kube, tuck, "ns")

	// First MODIFIED (equivalent to ADDED for an untracked resource) does a
	// real reconcile.
	if err := ctrl.handleEvent(context.Background(), WatchEvent{Type: "MODIFIED", Object: ts}); err != nil {
		t.Fatalf("first handleEvent error: %v", err)
	}
	if len(kube.applied) != 1 || len(kube.statuses) != 1 {
		t.Fatalf("expected 1 applied secret and 1 status write after first MODIFIED, got applied=%d statuses=%d",
			len(kube.applied), len(kube.statuses))
	}

	// Echo: same spec, only resourceVersion bumped (as a status write would
	// do) — must not trigger another reconcile.
	echo := ts
	echo.Metadata.ResourceVersion = "2"
	if err := ctrl.handleEvent(context.Background(), WatchEvent{Type: "MODIFIED", Object: echo}); err != nil {
		t.Fatalf("echo handleEvent error: %v", err)
	}
	if len(kube.applied) != 1 || len(kube.statuses) != 1 {
		t.Fatalf("echo MODIFIED must not reconcile again, got applied=%d statuses=%d", len(kube.applied), len(kube.statuses))
	}

	// A genuine spec change must still reconcile.
	changed := ts
	changed.Metadata.ResourceVersion = "3"
	changed.Spec.SecretKey = "other-key"
	if err := ctrl.handleEvent(context.Background(), WatchEvent{Type: "MODIFIED", Object: changed}); err != nil {
		t.Fatalf("changed handleEvent error: %v", err)
	}
	if len(kube.applied) != 2 || len(kube.statuses) != 2 {
		t.Fatalf("spec change must reconcile, got applied=%d statuses=%d", len(kube.applied), len(kube.statuses))
	}
}

func TestHandleEvent_Deleted_DoesNotApply(t *testing.T) {
	ts := newTS("ns", "ev-secret", "path/to/secret", "k8s-secret", "data")
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"path/to/secret": []byte("hello")}}
	ctrl := New(kube, tuck, "ns")
	ctrl.addTracked(ts)

	ev := WatchEvent{Type: "DELETED", Object: ts}
	if err := ctrl.handleEvent(context.Background(), ev); err != nil {
		t.Fatalf("handleEvent DELETED error: %v", err)
	}
	if len(kube.applied) != 0 {
		t.Error("DELETED event should NOT apply a k8s secret")
	}

	ctrl.mu.Lock()
	_, still := ctrl.tracked[resourceKey(ts)]
	ctrl.mu.Unlock()
	if still {
		t.Error("resource should have been removed from tracked map after DELETED")
	}
}

func TestHandleEvent_ErrorReturnsError(t *testing.T) {
	ctrl := New(&mockKube{}, &mockTuck{}, "")
	ev := WatchEvent{Type: "ERROR"}
	if err := ctrl.handleEvent(context.Background(), ev); err == nil {
		t.Error("expected error for ERROR watch event")
	}
}

func TestRefreshDuration_Default(t *testing.T) {
	spec := TuckSecretSpec{}
	if d := spec.RefreshDuration(); d != 5*time.Minute {
		t.Errorf("expected 5m default, got %v", d)
	}
}

func TestRefreshDuration_Custom(t *testing.T) {
	spec := TuckSecretSpec{RefreshInterval: "1h"}
	if d := spec.RefreshDuration(); d != time.Hour {
		t.Errorf("expected 1h, got %v", d)
	}
}

func TestRefreshDuration_Invalid(t *testing.T) {
	spec := TuckSecretSpec{RefreshInterval: "notaduration"}
	if d := spec.RefreshDuration(); d != 5*time.Minute {
		t.Errorf("expected 5m fallback for invalid duration, got %v", d)
	}
}

func TestRunDueRefreshes(t *testing.T) {
	ts := newTS("ns", "r", "p", "s", "k")
	kube := &mockKube{}
	tuck := &mockTuck{secrets: map[string][]byte{"p": []byte("v")}}
	ctrl := New(kube, tuck, "ns")

	// Manually set nextRefresh to the past so the resource is immediately due.
	ctrl.mu.Lock()
	ctrl.tracked[resourceKey(ts)] = trackedResource{
		ts:          ts,
		nextRefresh: time.Now().Add(-1 * time.Second),
	}
	ctrl.mu.Unlock()

	ctrl.runDueRefreshes(context.Background())

	if len(kube.applied) == 0 {
		t.Error("expected at least one ApplySecret call during refresh")
	}
}
