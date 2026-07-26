package csi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	csispec "github.com/container-storage-interface/spec/lib/go/csi"
)

// localMockMounter mirrors driver_test.go's mockMounter (package csi_test) —
// duplicated here since whitebox tests in package csi can't import the
// external csi_test package's helpers.
type localMockMounter struct {
	mounted map[string]bool
}

func newLocalMock() *localMockMounter { return &localMockMounter{mounted: make(map[string]bool)} }

func (m *localMockMounter) MountTmpfs(target string) error {
	m.mounted[target] = true
	return nil
}

func (m *localMockMounter) Unmount(target string) error {
	delete(m.mounted, target)
	return nil
}

func valueServer(t *testing.T, value *atomic.Value) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"value": value.Load().(string)})
	}))
}

func publishWithRefresh(t *testing.T, drv *Driver, addr, target, refreshInterval string) {
	t.Helper()
	req := &csispec.NodePublishVolumeRequest{
		VolumeId:   "vol-1",
		TargetPath: target,
		VolumeContext: map[string]string{
			ctxAddr:  addr,
			ctxPaths: "db/password",
		},
		Secrets: map[string]string{secretKeyToken: "test-token"},
	}
	if refreshInterval != "" {
		req.VolumeContext[ctxRefreshInterval] = refreshInterval
	}
	if _, err := drv.NodePublishVolume(context.Background(), req); err != nil {
		t.Fatalf("NodePublishVolume: %v", err)
	}
}

func TestPublish_NoRefreshAttribute_NotTracked(t *testing.T) {
	var v atomic.Value
	v.Store("v1")
	ts := valueServer(t, &v)
	defer ts.Close()

	drv := NewDriver("node", newLocalMock())
	publishWithRefresh(t, drv, ts.URL, t.TempDir(), "")

	if len(drv.mounts) != 0 {
		t.Fatalf("expected 0 tracked mounts, got %d", len(drv.mounts))
	}
}

func TestPublish_WithRefreshInterval_Tracked(t *testing.T) {
	var v atomic.Value
	v.Store("v1")
	ts := valueServer(t, &v)
	defer ts.Close()

	drv := NewDriver("node", newLocalMock())
	target := t.TempDir()
	publishWithRefresh(t, drv, ts.URL, target, "5m")

	if len(drv.mounts) != 1 {
		t.Fatalf("expected 1 tracked mount, got %d", len(drv.mounts))
	}
	st, ok := drv.mounts[target]
	if !ok {
		t.Fatalf("mount for target %q not tracked", target)
	}
	if st.addr != ts.URL || st.token != "test-token" || st.interval != 5*time.Minute {
		t.Errorf("unexpected mountState: %+v", st)
	}
}

func TestPublish_InvalidRefreshInterval_Rejected(t *testing.T) {
	var v atomic.Value
	v.Store("v1")
	ts := valueServer(t, &v)
	defer ts.Close()

	m := newLocalMock()
	drv := NewDriver("node", m)
	target := t.TempDir()

	req := &csispec.NodePublishVolumeRequest{
		VolumeId:   "vol-1",
		TargetPath: target,
		VolumeContext: map[string]string{
			ctxAddr:            ts.URL,
			ctxPaths:           "db/password",
			ctxRefreshInterval: "not-a-duration",
		},
		Secrets: map[string]string{secretKeyToken: "test-token"},
	}
	_, err := drv.NodePublishVolume(context.Background(), req)
	if err == nil {
		t.Fatal("expected an error for an invalid refresh-interval, got nil")
	}
	if len(drv.mounts) != 0 {
		t.Errorf("expected no tracked mounts after a rejected publish, got %d", len(drv.mounts))
	}
	if m.mounted[target] {
		t.Error("expected tmpfs to be unmounted after a rejected publish")
	}
}

func TestRunDueRefreshesAt_UpdatesFileContent(t *testing.T) {
	var v atomic.Value
	v.Store("v1")
	ts := valueServer(t, &v)
	defer ts.Close()

	drv := NewDriver("node", newLocalMock())
	target := t.TempDir()
	publishWithRefresh(t, drv, ts.URL, target, "1m")

	v.Store("v2")
	drv.runDueRefreshesAt(context.Background(), time.Now().Add(time.Hour))

	got, err := os.ReadFile(filepath.Join(target, "password"))
	if err != nil {
		t.Fatalf("read secret file: %v", err)
	}
	if string(got) != "v2" {
		t.Errorf("secret file content = %q, want %q", got, "v2")
	}
}

func TestRunDueRefreshesAt_NotYetDue_NoChange(t *testing.T) {
	var reqCount atomic.Int32
	var v atomic.Value
	v.Store("v1")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"value": v.Load().(string)})
	}))
	defer ts.Close()

	drv := NewDriver("node", newLocalMock())
	target := t.TempDir()
	publishWithRefresh(t, drv, ts.URL, target, "1h")

	v.Store("v2")
	drv.runDueRefreshesAt(context.Background(), time.Now())

	got, err := os.ReadFile(filepath.Join(target, "password"))
	if err != nil {
		t.Fatalf("read secret file: %v", err)
	}
	if string(got) != "v1" {
		t.Errorf("secret file content changed before it was due: got %q, want %q", got, "v1")
	}
	if n := reqCount.Load(); n != 1 {
		t.Errorf("expected exactly 1 request (the initial publish), got %d", n)
	}
}

func TestRunDueRefreshesAt_FailedFetch_LeavesOldFileInPlace(t *testing.T) {
	var failing atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": "v1"})
	}))
	defer ts.Close()

	drv := NewDriver("node", newLocalMock())
	target := t.TempDir()
	publishWithRefresh(t, drv, ts.URL, target, "1m")

	failing.Store(true)
	drv.runDueRefreshesAt(context.Background(), time.Now().Add(time.Hour))

	got, err := os.ReadFile(filepath.Join(target, "password"))
	if err != nil {
		t.Fatalf("read secret file: %v", err)
	}
	if string(got) != "v1" {
		t.Errorf("secret file content = %q after a failed refresh, want unchanged %q", got, "v1")
	}
	if len(drv.mounts) != 1 {
		t.Errorf("expected mount to remain tracked after a failed refresh, got %d tracked", len(drv.mounts))
	}
}

func TestRunDueRefreshesAt_RetriesNextTickAfterFailure(t *testing.T) {
	var failing atomic.Bool
	var v atomic.Value
	v.Store("v1")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": v.Load().(string)})
	}))
	defer ts.Close()

	drv := NewDriver("node", newLocalMock())
	target := t.TempDir()
	publishWithRefresh(t, drv, ts.URL, target, "1m")

	failing.Store(true)
	drv.runDueRefreshesAt(context.Background(), time.Now().Add(time.Hour))

	// nextRefresh must still be in the past (unchanged) after the failure,
	// so a later pass retries it without needing a fresh "due" timestamp.
	failing.Store(false)
	v.Store("v2")
	drv.runDueRefreshesAt(context.Background(), time.Now().Add(time.Hour))

	got, err := os.ReadFile(filepath.Join(target, "password"))
	if err != nil {
		t.Fatalf("read secret file: %v", err)
	}
	if string(got) != "v2" {
		t.Errorf("secret file content = %q after retry, want %q", got, "v2")
	}
}

func TestUnpublish_StopsTracking(t *testing.T) {
	var v atomic.Value
	v.Store("v1")
	ts := valueServer(t, &v)
	defer ts.Close()

	drv := NewDriver("node", newLocalMock())
	target := t.TempDir()
	publishWithRefresh(t, drv, ts.URL, target, "1m")

	_, err := drv.NodeUnpublishVolume(context.Background(), &csispec.NodeUnpublishVolumeRequest{TargetPath: target})
	if err != nil {
		t.Fatalf("NodeUnpublishVolume: %v", err)
	}
	if len(drv.mounts) != 0 {
		t.Fatalf("expected 0 tracked mounts after unpublish, got %d", len(drv.mounts))
	}

	// A refresh pass afterward must be a no-op — no panic, no request.
	drv.runDueRefreshesAt(context.Background(), time.Now().Add(time.Hour))
}

func TestConcurrentMounts_OneFailsOtherSucceeds(t *testing.T) {
	var failingA atomic.Bool
	tsA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failingA.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": "a1"})
	}))
	defer tsA.Close()

	var vB atomic.Value
	vB.Store("b1")
	tsB := valueServer(t, &vB)
	defer tsB.Close()

	drv := NewDriver("node", newLocalMock())
	targetA, targetB := t.TempDir(), t.TempDir()
	publishWithRefresh(t, drv, tsA.URL, targetA, "1m")
	publishWithRefresh(t, drv, tsB.URL, targetB, "1m")

	failingA.Store(true)
	vB.Store("b2")
	drv.runDueRefreshesAt(context.Background(), time.Now().Add(time.Hour))

	gotA, err := os.ReadFile(filepath.Join(targetA, "password"))
	if err != nil {
		t.Fatalf("read secret file A: %v", err)
	}
	if string(gotA) != "a1" {
		t.Errorf("mount A content = %q, want unchanged %q (its refresh should have failed)", gotA, "a1")
	}

	gotB, err := os.ReadFile(filepath.Join(targetB, "password"))
	if err != nil {
		t.Fatalf("read secret file B: %v", err)
	}
	if string(gotB) != "b2" {
		t.Errorf("mount B content = %q, want %q (its refresh should have succeeded)", gotB, "b2")
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "secret")

	if err := writeFileAtomic(dest, []byte("v1"), 0o400); err != nil {
		t.Fatalf("writeFileAtomic (create): %v", err)
	}
	if err := writeFileAtomic(dest, []byte("v2"), 0o400); err != nil {
		t.Fatalf("writeFileAtomic (overwrite): %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "v2" {
		t.Errorf("content = %q, want %q", got, "v2")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 file left in %s, found %d: %v", dir, len(entries), entries)
	}

	if err := writeFileAtomic(filepath.Join(dir, "missing-subdir", "secret"), []byte("x"), 0o400); err == nil {
		t.Error("expected an error writing into a nonexistent directory, got nil")
	}
}
