package csi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	csispec "github.com/container-storage-interface/spec/lib/go/csi"

	"github.com/NAGenaev/tuck/internal/csi"
)

// mockMounter records calls and creates/removes directories without real mounts.
type mockMounter struct {
	mounted   map[string]bool
	mountErr  error
	unmountErr error
}

func newMock() *mockMounter { return &mockMounter{mounted: make(map[string]bool)} }

func (m *mockMounter) MountTmpfs(target string) error {
	if m.mountErr != nil {
		return m.mountErr
	}
	m.mounted[target] = true
	return nil
}

func (m *mockMounter) Unmount(target string) error {
	if m.unmountErr != nil {
		return m.unmountErr
	}
	delete(m.mounted, target)
	return nil
}

func TestNodePublishVolume(t *testing.T) {
	// Fake Tuck server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tuck-Token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": "s3cr3t"})
	}))
	defer ts.Close()

	target := t.TempDir()
	m := newMock()
	drv := csi.NewDriver("test-node", m)

	req := &csispec.NodePublishVolumeRequest{
		VolumeId:   "vol-1",
		TargetPath: target,
		VolumeContext: map[string]string{
			"tuck.io/addr":  ts.URL,
			"tuck.io/paths": "db/password",
		},
		Secrets: map[string]string{"token": "test-token"},
	}

	_, err := drv.NodePublishVolume(context.Background(), req)
	if err != nil {
		t.Fatalf("NodePublishVolume: %v", err)
	}

	// tmpfs should be mounted
	if !m.mounted[target] {
		t.Error("expected MountTmpfs to be called")
	}

	// Secret file should exist
	got, err := os.ReadFile(filepath.Join(target, "password"))
	if err != nil {
		t.Fatalf("read secret file: %v", err)
	}
	if string(got) != "s3cr3t" {
		t.Errorf("secret file content = %q, want %q", got, "s3cr3t")
	}
}

func TestNodeUnpublishVolume(t *testing.T) {
	target := t.TempDir()
	m := newMock()
	m.mounted[target] = true
	drv := csi.NewDriver("test-node", m)

	_, err := drv.NodeUnpublishVolume(context.Background(), &csispec.NodeUnpublishVolumeRequest{
		VolumeId:   "vol-1",
		TargetPath: target,
	})
	if err != nil {
		t.Fatalf("NodeUnpublishVolume: %v", err)
	}
	if m.mounted[target] {
		t.Error("expected Unmount to be called")
	}
}

func TestGetPluginInfo(t *testing.T) {
	drv := csi.NewDriver("node-1", newMock())
	resp, err := drv.GetPluginInfo(context.Background(), &csispec.GetPluginInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Name != csi.DriverName {
		t.Errorf("plugin name = %q, want %q", resp.Name, csi.DriverName)
	}
}

func TestMissingToken(t *testing.T) {
	drv := csi.NewDriver("node-1", newMock())
	_, err := drv.NodePublishVolume(context.Background(), &csispec.NodePublishVolumeRequest{
		TargetPath:    t.TempDir(),
		VolumeContext: map[string]string{"tuck.io/addr": "http://x", "tuck.io/paths": "a/b"},
		Secrets:       map[string]string{}, // no token
	})
	if err == nil {
		t.Error("expected error for missing token")
	}
}

// TestExpandKeys checks that tuck.io/expand-keys=true writes one file per
// JSON key when the secret value is a JSON object.
func TestExpandKeys(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tuck-Token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": `{"username":"admin","password":"s3cr3t"}`,
		})
	}))
	defer ts.Close()

	target := t.TempDir()
	m := newMock()
	drv := csi.NewDriver("node-1", m)

	_, err := drv.NodePublishVolume(context.Background(), &csispec.NodePublishVolumeRequest{
		VolumeId:   "vol-2",
		TargetPath: target,
		VolumeContext: map[string]string{
			"tuck.io/addr":        ts.URL,
			"tuck.io/paths":       "db/creds",
			"tuck.io/expand-keys": "true",
		},
		Secrets: map[string]string{"token": "tok"},
	})
	if err != nil {
		t.Fatalf("NodePublishVolume with expand-keys: %v", err)
	}

	for _, name := range []string{"username", "password"} {
		if _, err := os.Stat(filepath.Join(target, name)); err != nil {
			t.Errorf("expected file %q after key expansion: %v", name, err)
		}
	}
	got, _ := os.ReadFile(filepath.Join(target, "password"))
	if string(got) != "s3cr3t" {
		t.Errorf("password = %q, want %q", got, "s3cr3t")
	}
}

// TestCustomMode verifies that tuck.io/mode sets the file permission bits.
// Skipped on Windows where Unix permission bits are not enforced.
func TestCustomMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits not enforced on Windows")
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"value": "secret"})
	}))
	defer ts.Close()

	target := t.TempDir()
	m := newMock()
	drv := csi.NewDriver("node-1", m)

	_, err := drv.NodePublishVolume(context.Background(), &csispec.NodePublishVolumeRequest{
		VolumeId:   "vol-3",
		TargetPath: target,
		VolumeContext: map[string]string{
			"tuck.io/addr":  ts.URL,
			"tuck.io/paths": "app/key",
			"tuck.io/mode":  "0600",
		},
		Secrets: map[string]string{"token": "tok"},
	})
	if err != nil {
		t.Fatalf("NodePublishVolume with mode: %v", err)
	}

	info, err := os.Stat(filepath.Join(target, "key"))
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want %o", got, 0o600)
	}
}

// TestNodeGetVolumeStats verifies the stub returns without error.
func TestNodeGetVolumeStats(t *testing.T) {
	drv := csi.NewDriver("node-1", newMock())
	resp, err := drv.NodeGetVolumeStats(context.Background(), &csispec.NodeGetVolumeStatsRequest{})
	if err != nil {
		t.Fatalf("NodeGetVolumeStats: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}
