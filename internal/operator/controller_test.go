package operator

import (
	"context"
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

func (m *mockKube) ApplySecret(_ context.Context, s *KubeSecret) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.applied = append(m.applied, s)
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
	mu      sync.Mutex
	secrets map[string][]byte
}

func (m *mockTuck) GetSecret(_ context.Context, path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
