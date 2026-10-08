package csi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	csispec "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type k8sMounter struct {
	mounts  int
	mounted map[string]bool
}

func newK8sMounter() *k8sMounter { return &k8sMounter{mounted: map[string]bool{}} }

func (m *k8sMounter) MountTmpfs(t string) error { m.mounts++; m.mounted[t] = true; return nil }
func (m *k8sMounter) Unmount(t string) error    { delete(m.mounted, t); return nil }
func (m *k8sMounter) IsMounted(t string) (bool, error) {
	return m.mounted[t], nil
}

// fakeTuck accepts K8s logins for the SA tokens in validSA and then serves
// secrets only to Tuck tokens it minted and has not revoked.
type fakeTuck struct {
	mu      sync.Mutex
	validSA map[string]bool
	live    map[string]bool
	logins  int
	value   string
	srv     *httptest.Server
}

func newFakeTuck(t *testing.T, validSA ...string) *fakeTuck {
	f := &fakeTuck{validSA: map[string]bool{}, live: map[string]bool{}, value: "v1"}
	for _, s := range validSA {
		f.validSA[s] = true
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/v1/auth/kubernetes/login" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !f.validSA[body["token"]] {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			f.logins++
			tok := fmt.Sprintf("tuck-%s-%d", body["token"], f.logins)
			f.live[tok] = true
			_ = json.NewEncoder(w).Encode(map[string]string{"token": tok})
			return
		}
		if !f.live[r.Header.Get("X-Tuck-Token")] {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": f.value})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTuck) revokeAll() {
	f.mu.Lock()
	f.live = map[string]bool{}
	f.mu.Unlock()
}

func (f *fakeTuck) setValue(v string) {
	f.mu.Lock()
	f.value = v
	f.mu.Unlock()
}

func saTokens(tok string, exp time.Time) string {
	b, _ := json.Marshal(map[string]any{"": map[string]string{
		"token": tok, "expirationTimestamp": exp.UTC().Format(time.RFC3339),
	}})
	return string(b)
}

func k8sReq(addr, target, sa string, exp time.Time, extra map[string]string) *csispec.NodePublishVolumeRequest {
	vc := map[string]string{
		ctxAddr:           addr,
		ctxPaths:          "db/password",
		ctxAuth:           authKubernetes,
		volumeKeySATokens: saTokens(sa, exp),
	}
	for k, v := range extra {
		vc[k] = v
	}
	return &csispec.NodePublishVolumeRequest{VolumeId: "v", TargetPath: target, VolumeContext: vc}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestKubernetesAuth_PublishLogsInAndTracks(t *testing.T) {
	f := newFakeTuck(t, "sa-1")
	target := t.TempDir()
	drv := NewDriver("n", newK8sMounter())

	if _, err := drv.NodePublishVolume(context.Background(), k8sReq(f.srv.URL, target, "sa-1", time.Now().Add(time.Hour), nil)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "password")); got != "v1" {
		t.Errorf("file = %q", got)
	}
	st, ok := drv.mounts[target]
	if !ok || !st.k8s || st.token == "" || st.sa.token != "sa-1" {
		t.Fatalf("expected tracked k8s mount with token, got %+v ok=%v", st, ok)
	}
}

func TestKubernetesAuth_RepublishRotatesSAOnly(t *testing.T) {
	f := newFakeTuck(t, "sa-1", "sa-2")
	target := t.TempDir()
	m := newK8sMounter()
	drv := NewDriver("n", m)
	ctx := context.Background()

	if _, err := drv.NodePublishVolume(ctx, k8sReq(f.srv.URL, target, "sa-1", time.Now().Add(time.Hour), nil)); err != nil {
		t.Fatal(err)
	}
	f.setValue("v2") // a republish must not rewrite files
	if _, err := drv.NodePublishVolume(ctx, k8sReq(f.srv.URL, target, "sa-2", time.Now().Add(2*time.Hour), nil)); err != nil {
		t.Fatalf("republish: %v", err)
	}
	if m.mounts != 1 {
		t.Errorf("tmpfs mounted %d times, want 1", m.mounts)
	}
	if got := readFile(t, filepath.Join(target, "password")); got != "v1" {
		t.Errorf("republish rewrote files: %q", got)
	}
	if drv.mounts[target].sa.token != "sa-2" {
		t.Errorf("SA token not rotated: %+v", drv.mounts[target].sa)
	}
	if f.logins != 1 {
		t.Errorf("logins = %d, want 1 (republish must not log in)", f.logins)
	}
}

func TestKubernetesAuth_RefreshReloginsAfterTuckTokenRejected(t *testing.T) {
	f := newFakeTuck(t, "sa-1", "sa-2")
	target := t.TempDir()
	drv := NewDriver("n", newK8sMounter())
	ctx := context.Background()

	req := k8sReq(f.srv.URL, target, "sa-1", time.Now().Add(time.Hour), map[string]string{ctxRefreshInterval: "1m"})
	if _, err := drv.NodePublishVolume(ctx, req); err != nil {
		t.Fatal(err)
	}
	// The Tuck token expires; kubelet republishes a fresh SA token.
	f.revokeAll()
	f.setValue("v2")
	if _, err := drv.NodePublishVolume(ctx, k8sReq(f.srv.URL, target, "sa-2", time.Now().Add(time.Hour), nil)); err != nil {
		t.Fatal(err)
	}
	drv.runDueRefreshesAt(ctx, time.Now().Add(time.Hour))

	if got := readFile(t, filepath.Join(target, "password")); got != "v2" {
		t.Errorf("file = %q, want v2 after relogin", got)
	}
	if f.logins != 2 {
		t.Errorf("logins = %d, want 2", f.logins)
	}
}

func TestKubernetesAuth_ExpiredSAKeepsFiles(t *testing.T) {
	f := newFakeTuck(t, "sa-1")
	target := t.TempDir()
	drv := NewDriver("n", newK8sMounter())
	ctx := context.Background()

	req := k8sReq(f.srv.URL, target, "sa-1", time.Now().Add(time.Hour), map[string]string{ctxRefreshInterval: "1m"})
	if _, err := drv.NodePublishVolume(ctx, req); err != nil {
		t.Fatal(err)
	}
	f.revokeAll()
	f.setValue("v2")

	drv.mu.Lock()
	drv.mounts[target].sa.expires = time.Now().Add(-time.Minute)
	drv.mu.Unlock()
	drv.runDueRefreshesAt(ctx, time.Now().Add(time.Hour))

	if got := readFile(t, filepath.Join(target, "password")); got != "v1" {
		t.Errorf("files must be kept, got %q", got)
	}
	if len(drv.mounts) != 1 {
		t.Errorf("mount must stay tracked")
	}
}

func TestKubernetesAuth_DriverRestartAdoptsExistingMount(t *testing.T) {
	f := newFakeTuck(t, "sa-1")
	target := t.TempDir()
	m := newK8sMounter()
	m.mounted[target] = true // survived the restart
	drv := NewDriver("n", m)

	req := k8sReq(f.srv.URL, target, "sa-1", time.Now().Add(time.Hour), map[string]string{ctxRefreshInterval: "5m"})
	if _, err := drv.NodePublishVolume(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if m.mounts != 0 {
		t.Errorf("must not mount over an existing mount")
	}
	if st, ok := drv.mounts[target]; !ok || st.interval != 5*time.Minute {
		t.Fatalf("mount not adopted: %+v", st)
	}
}

func TestKubernetesAuth_TokenFromSecrets(t *testing.T) {
	f := newFakeTuck(t, "sa-1")
	target := t.TempDir()
	drv := NewDriver("n", newK8sMounter())

	req := k8sReq(f.srv.URL, target, "ignored", time.Now().Add(time.Hour), nil)
	delete(req.VolumeContext, volumeKeySATokens)
	req.Secrets = map[string]string{volumeKeySATokens: saTokens("sa-1", time.Now().Add(time.Hour))}
	if _, err := drv.NodePublishVolume(context.Background(), req); err != nil {
		t.Fatalf("publish with token in secrets: %v", err)
	}
	if got := readFile(t, filepath.Join(target, "password")); got != "v1" {
		t.Errorf("file = %q", got)
	}
}

func TestTokenMode_RepublishRotatesToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"value": "x"})
	}))
	defer ts.Close()
	target := t.TempDir()
	m := newK8sMounter()
	drv := NewDriver("n", m)
	pub := func(tok string) {
		t.Helper()
		_, err := drv.NodePublishVolume(context.Background(), &csispec.NodePublishVolumeRequest{
			VolumeId: "v", TargetPath: target,
			VolumeContext: map[string]string{ctxAddr: ts.URL, ctxPaths: "a/b", ctxRefreshInterval: "1m"},
			Secrets:       map[string]string{secretKeyToken: tok},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	pub("t1")
	pub("t2")
	if m.mounts != 1 {
		t.Errorf("tmpfs mounted %d times, want 1", m.mounts)
	}
	if drv.mounts[target].token != "t2" {
		t.Errorf("token not rotated: %q", drv.mounts[target].token)
	}
}

func TestKubernetesAuth_PublishErrors(t *testing.T) {
	f := newFakeTuck(t, "sa-1")
	drv := NewDriver("n", newK8sMounter())
	ctx := context.Background()

	req := k8sReq(f.srv.URL, t.TempDir(), "sa-1", time.Now().Add(time.Hour), nil)
	delete(req.VolumeContext, volumeKeySATokens)
	if _, err := drv.NodePublishVolume(ctx, req); status.Code(err) != codes.InvalidArgument {
		t.Errorf("missing SA token: got %v", err)
	}

	req = k8sReq(f.srv.URL, t.TempDir(), "sa-1", time.Now().Add(time.Hour), map[string]string{ctxK8sAudience: "other"})
	if _, err := drv.NodePublishVolume(ctx, req); status.Code(err) != codes.InvalidArgument {
		t.Errorf("wrong audience: got %v", err)
	}

	req = k8sReq(f.srv.URL, t.TempDir(), "sa-1", time.Now().Add(time.Hour), map[string]string{ctxAuth: "bogus"})
	if _, err := drv.NodePublishVolume(ctx, req); status.Code(err) != codes.InvalidArgument {
		t.Errorf("bad auth mode: got %v", err)
	}

	m := newK8sMounter()
	drv = NewDriver("n", m)
	target := t.TempDir()
	req = k8sReq(f.srv.URL, target, "unknown-sa", time.Now().Add(time.Hour), nil)
	if _, err := drv.NodePublishVolume(ctx, req); status.Code(err) != codes.Internal {
		t.Errorf("rejected login: got %v", err)
	}
	if m.mounted[target] || len(drv.mounts) != 0 {
		t.Errorf("failed publish must unmount and not track")
	}
}
