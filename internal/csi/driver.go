// Package csi implements a Kubernetes CSI Node driver that fetches secrets
// from a Tuck server and writes them as files inside a per-pod tmpfs mount.
//
// Only Identity and Node services are implemented (no Controller — the driver
// is fully stateless). Volumes are ephemeral: created on NodePublish and
// removed on NodeUnpublish.
package csi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	csispec "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/NAGenaev/tuck/internal/tlsutil"
)

const (
	DriverName    = "secrets.tuck.io"
	DriverVersion = "1.5.0"

	// Volume context keys supplied by the StorageClass / PVC attributes.
	ctxAddr            = "tuck.io/addr"             // e.g. "https://tuck:8200"
	ctxPaths           = "tuck.io/paths"            // comma-separated secret paths; "path=file" names the file explicitly
	ctxNamespace       = "tuck.io/namespace"        // optional Tuck namespace
	ctxKVVersion       = "tuck.io/kv-version"       // "1" or "2" (default "1")
	ctxInsecure        = "tuck.io/insecure"         // "true" to skip TLS verification
	ctxExpandKeys      = "tuck.io/expand-keys"      // "true" → JSON object value exploded to per-key files
	ctxMode            = "tuck.io/mode"             // octal file permission string, e.g. "0400" (default "0400")
	ctxRefreshInterval = "tuck.io/refresh-interval" // duration string, e.g. "5m" — omit to disable background refresh (default)

	// Secret key holding the Tuck token (tuck.io/auth=token, the default),
	// passed via NodePublishSecrets. Kubernetes auth needs no secret.
	secretKeyToken = "token"

	// refreshTickInterval is how often the background loop scans for mounts
	// whose refresh interval has elapsed. A per-mount tuck.io/refresh-interval
	// shorter than this is clamped up to it — same fixed-tick-checks-per-item
	// shape as internal/operator/controller.go's 30s reconcile ticker.
	refreshTickInterval = 30 * time.Second
)

// Driver implements the CSI Identity and Node gRPC services.
type Driver struct {
	csispec.UnimplementedIdentityServer
	csispec.UnimplementedNodeServer
	csispec.UnimplementedControllerServer

	nodeID  string
	mounter Mounter
	caFile  string // PEM CA bundle for verifying Tuck servers; "" = system roots

	mu     sync.Mutex
	mounts map[string]*mountState // keyed by TargetPath; only holds mounts with a valid refresh interval
}

// mountState is the bookkeeping a background refresh needs to re-fetch and
// re-write a mount's secret files. It's a snapshot of everything
// NodePublishVolume already resolved once — the token in particular is
// reused unchanged for the mount's whole lifetime, since Kubernetes never
// redelivers nodePublishSecretRef to an already-published volume.
//
// With tuck.io/auth=kubernetes, token is the cached Tuck token obtained by
// logging in with sa; kubelet's periodic republish (CSIDriver
// requiresRepublish) swaps in a fresh sa, and the next login uses it.
type mountState struct {
	addr        string
	token       string
	k8s         bool
	cluster     string
	sa          saToken
	ns          string
	kvVersion   string
	insecure    bool
	expandKeys  bool
	mode        os.FileMode
	paths       []string
	interval    time.Duration
	nextRefresh time.Time
}

// NewDriver creates a Driver for the given node.
func NewDriver(nodeID string, mounter Mounter) *Driver {
	return &Driver{nodeID: nodeID, mounter: mounter, mounts: make(map[string]*mountState)}
}

// ─── Identity ───────────────────────────────────────────────────────────────

func (d *Driver) GetPluginInfo(_ context.Context, _ *csispec.GetPluginInfoRequest) (*csispec.GetPluginInfoResponse, error) {
	return &csispec.GetPluginInfoResponse{
		Name:          DriverName,
		VendorVersion: DriverVersion,
	}, nil
}

func (d *Driver) GetPluginCapabilities(_ context.Context, _ *csispec.GetPluginCapabilitiesRequest) (*csispec.GetPluginCapabilitiesResponse, error) {
	return &csispec.GetPluginCapabilitiesResponse{}, nil // no controller capabilities
}

func (d *Driver) Probe(_ context.Context, _ *csispec.ProbeRequest) (*csispec.ProbeResponse, error) {
	return &csispec.ProbeResponse{}, nil
}

// ─── Node ────────────────────────────────────────────────────────────────────

func (d *Driver) NodeGetCapabilities(_ context.Context, _ *csispec.NodeGetCapabilitiesRequest) (*csispec.NodeGetCapabilitiesResponse, error) {
	return &csispec.NodeGetCapabilitiesResponse{}, nil
}

func (d *Driver) NodeGetInfo(_ context.Context, _ *csispec.NodeGetInfoRequest) (*csispec.NodeGetInfoResponse, error) {
	return &csispec.NodeGetInfoResponse{NodeId: d.nodeID}, nil
}

// NodePublishVolume fetches the requested Tuck secrets and writes them as
// files into a tmpfs mounted at req.TargetPath.
//
// Additional volume context attributes beyond the base set:
//   - tuck.io/expand-keys=true — when the secret value is a JSON object,
//     each key is written as a separate file rather than the whole JSON blob.
//   - tuck.io/mode=<octal> — file permission bits, default 0400.
func (d *Driver) NodePublishVolume(ctx context.Context, req *csispec.NodePublishVolumeRequest) (*csispec.NodePublishVolumeResponse, error) {
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}

	vc := req.GetVolumeContext()
	addr := vc[ctxAddr]
	if addr == "" {
		return nil, status.Error(codes.InvalidArgument, "volume context tuck.io/addr is required")
	}
	pathsRaw := vc[ctxPaths]
	if pathsRaw == "" {
		return nil, status.Error(codes.InvalidArgument, "volume context tuck.io/paths is required")
	}

	var (
		token   string
		sa      saToken
		useK8s  bool
		cluster = vc[ctxK8sCluster]
	)
	switch vc[ctxAuth] {
	case "", authToken:
		token = req.GetSecrets()[secretKeyToken]
		if token == "" {
			return nil, status.Error(codes.InvalidArgument, "node publish secret \"token\" is required")
		}
	case authKubernetes:
		useK8s = true
		var err error
		sa, err = serviceAccountToken(vc, req.GetSecrets(), vc[ctxK8sAudience])
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "tuck.io/auth=kubernetes: %v", err)
		}
	default:
		return nil, status.Errorf(codes.InvalidArgument, "volume context %s must be %q or %q", ctxAuth, authToken, authKubernetes)
	}

	ns := vc[ctxNamespace]
	kvVersion := vc[ctxKVVersion]
	if kvVersion == "" {
		kvVersion = "1"
	}
	insecure := strings.EqualFold(vc[ctxInsecure], "true")
	expandKeys := strings.EqualFold(vc[ctxExpandKeys], "true")

	mode := os.FileMode(0o400)
	if modeStr := vc[ctxMode]; modeStr != "" {
		if m, err := strconv.ParseUint(modeStr, 8, 32); err == nil {
			mode = os.FileMode(m)
		}
	}

	refreshInterval, err := parseRefreshInterval(vc[ctxRefreshInterval])
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "volume context tuck.io/refresh-interval: %v", err)
	}

	paths := splitPaths(pathsRaw)

	st := &mountState{
		addr: addr, token: token, k8s: useK8s, cluster: cluster, sa: sa,
		ns: ns, kvVersion: kvVersion, insecure: insecure, expandKeys: expandKeys,
		mode: mode, paths: paths, interval: refreshInterval,
		nextRefresh: time.Now().Add(refreshInterval),
	}

	// kubelet re-calls NodePublishVolume for an already-mounted volume when the
	// CSIDriver sets requiresRepublish, to deliver a rotated ServiceAccount
	// token or Secret. That only rotates credentials: the tmpfs and its files
	// stay put (CSI requires NodePublishVolume to be idempotent).
	d.mu.Lock()
	cur, tracked := d.mounts[req.TargetPath]
	if tracked && cur.k8s == useK8s {
		if useK8s {
			if cur.sa.token != sa.token {
				slog.Info("csi: ServiceAccount token rotated", "target", req.TargetPath, "expires", sa.expires)
			}
			cur.sa = sa
		} else {
			cur.token = token
		}
		d.mu.Unlock()
		return &csispec.NodePublishVolumeResponse{}, nil
	}
	d.mu.Unlock()
	if d.isMounted(req.TargetPath) {
		// Driver restarted: the mount survived, our bookkeeping did not.
		if useK8s || refreshInterval > 0 {
			st.nextRefresh = time.Now()
			d.mu.Lock()
			d.mounts[req.TargetPath] = st
			d.mu.Unlock()
		}
		return &csispec.NodePublishVolumeResponse{}, nil
	}

	// Ensure target dir exists.
	if err := os.MkdirAll(req.TargetPath, 0o750); err != nil {
		return nil, status.Errorf(codes.Internal, "mkdir target path: %v", err)
	}

	// Mount tmpfs so secrets never touch disk.
	if err := d.mounter.MountTmpfs(req.TargetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "mount tmpfs: %v", err)
	}

	hc, err := httpClient(insecure, d.caFile)
	if err != nil {
		_ = d.mounter.Unmount(req.TargetPath)
		return nil, status.Errorf(codes.Internal, "tuck TLS config: %v", err)
	}
	files, err := fetchAll(ctx, hc, st)
	if err != nil {
		_ = d.mounter.Unmount(req.TargetPath)
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	for fname, value := range files {
		dest := filepath.Join(req.TargetPath, fname)
		if err := writeFileAtomic(dest, []byte(value), mode); err != nil {
			_ = d.mounter.Unmount(req.TargetPath)
			return nil, status.Errorf(codes.Internal, "write secret file %q: %v", fname, err)
		}
	}

	if refreshInterval > 0 || useK8s {
		d.mu.Lock()
		d.mounts[req.TargetPath] = st
		d.mu.Unlock()
	}
	return &csispec.NodePublishVolumeResponse{}, nil
}

// mountChecker is implemented by Mounters that can tell whether a path is
// already a mount point.
type mountChecker interface {
	IsMounted(target string) (bool, error)
}

func (d *Driver) isMounted(target string) bool {
	c, ok := d.mounter.(mountChecker)
	if !ok {
		return false
	}
	m, err := c.IsMounted(target)
	return err == nil && m
}

// fetchAll fetches every path of a mount. For Kubernetes auth it logs in
// first when there is no cached Tuck token, and once more if Tuck rejects the
// cached one (expired). st.token is updated in place.
func fetchAll(ctx context.Context, hc *http.Client, st *mountState) (map[string]string, error) {
	login := func() error {
		if !st.sa.usable(time.Now()) {
			return fmt.Errorf("kubernetes login: ServiceAccount token expired, waiting for kubelet to republish")
		}
		t, err := loginKubernetes(ctx, hc, st.addr, st.cluster, st.sa.token)
		if err != nil {
			return err
		}
		st.token = t
		return nil
	}
	if st.k8s && st.token == "" {
		if err := login(); err != nil {
			return nil, err
		}
	}
	for attempt := 0; ; attempt++ {
		all := make(map[string]string)
		source := make(map[string]string) // file name -> path that produced it
		var failed error
		for _, spec := range st.paths {
			p, alias, specErr := parsePathSpec(spec)
			if specErr != nil {
				return nil, specErr
			}
			files, err := fetchSecretFiles(ctx, hc, st.addr, st.token, st.ns, p, st.kvVersion, st.expandKeys)
			if err != nil {
				failed = fmt.Errorf("fetch secret %q: %w", p, err)
				break
			}
			if alias != "" {
				if len(files) != 1 {
					return nil, status.Errorf(codes.InvalidArgument,
						"tuck.io/paths: %q=%s cannot rename a secret that expands to %d files; use \"path=file\" only for single-value secrets",
						p, alias, len(files))
				}
				for fname, value := range files { // a single value takes the explicit name
					delete(files, fname)
					files[alias] = value
				}
			}
			for fname, value := range files {
				// Two secrets must never silently share one file: the pod would
				// read the wrong credential without any error.
				if prev, dup := source[fname]; dup {
					return nil, status.Errorf(codes.InvalidArgument,
						"file name %q is produced by both %q and %q; give them distinct names with \"path=filename\" in tuck.io/paths",
						fname, prev, p)
				}
				source[fname] = p
				all[fname] = value
			}
		}
		if failed == nil {
			return all, nil
		}
		var se *httpStatusError
		if st.k8s && attempt == 0 && errors.As(failed, &se) && se.authRejected() {
			if err := login(); err != nil {
				return nil, err
			}
			continue
		}
		return nil, failed
	}
}

// parsePathSpec splits one tuck.io/paths entry, "path" or "path=filename".
func parsePathSpec(spec string) (path, alias string, err error) {
	path, alias, _ = strings.Cut(strings.TrimSpace(spec), "=")
	path, alias = strings.TrimSpace(path), strings.TrimSpace(alias)
	if path == "" {
		return "", "", status.Errorf(codes.InvalidArgument, "tuck.io/paths: empty path in %q", spec)
	}
	if alias != "" && (strings.ContainsAny(alias, `/\`) || alias == "." || alias == "..") {
		return "", "", status.Errorf(codes.InvalidArgument, "tuck.io/paths: file name %q must be a plain name", alias)
	}
	return path, alias, nil
}

// parseRefreshInterval validates tuck.io/refresh-interval. Empty (the
// default) returns 0, meaning "no background refresh" — the exact behavior
// this attribute didn't exist before it was added. An unparseable non-empty
// value is a hard publish error rather than a silent fallback: unlike a
// TuckSecret CR, a CSI mount has no ongoing status a typo could be spotted
// through later, so failing the mount immediately (visible in `kubectl
// describe pod`) beats silently never refreshing. A parsed value below the
// driver's refreshTickInterval is clamped up to it with a warning — that's
// a capability limit, not a user mistake, so it doesn't fail the mount.
func parseRefreshInterval(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("must not be negative")
	}
	if d < refreshTickInterval {
		slog.Warn("csi: refresh-interval below refresh check granularity, clamping",
			"requested", d, "clamped_to", refreshTickInterval)
		return refreshTickInterval, nil
	}
	return d, nil
}

// NodeGetVolumeStats returns empty volume statistics (tmpfs metrics are not
// tracked; returning an empty response is preferable to UNIMPLEMENTED).
func (d *Driver) NodeGetVolumeStats(_ context.Context, _ *csispec.NodeGetVolumeStatsRequest) (*csispec.NodeGetVolumeStatsResponse, error) {
	return &csispec.NodeGetVolumeStatsResponse{}, nil
}

// NodeUnpublishVolume unmounts the tmpfs and removes the target directory.
func (d *Driver) NodeUnpublishVolume(_ context.Context, req *csispec.NodeUnpublishVolumeRequest) (*csispec.NodeUnpublishVolumeResponse, error) {
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}
	d.mu.Lock()
	delete(d.mounts, req.TargetPath) // no-op if this mount was never tracked
	d.mu.Unlock()
	if err := d.mounter.Unmount(req.TargetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "unmount: %v", err)
	}
	_ = os.RemoveAll(req.TargetPath)
	return &csispec.NodeUnpublishVolumeResponse{}, nil
}

// ─── Background refresh ─────────────────────────────────────────────────────

// RunRefreshLoop periodically re-fetches and re-writes secret files for
// every mount published with a tuck.io/refresh-interval, until ctx is
// cancelled. It's cheap to run unconditionally (mirrors the always-on
// reconcile ticker in internal/operator/controller.go): with no refresh-aware
// mounts tracked, each tick is a no-op scan of an empty map.
func (d *Driver) RunRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(refreshTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.runDueRefreshes(ctx)
		}
	}
}

func (d *Driver) runDueRefreshes(ctx context.Context) {
	d.runDueRefreshesAt(ctx, time.Now())
}

// runDueRefreshesAt does one refresh pass, treating now as the current time.
// Split out from runDueRefreshes purely so tests can force mounts to be
// "due" deterministically without waiting on the real ticker or reaching
// into mountState to backdate nextRefresh by hand.
func (d *Driver) runDueRefreshesAt(ctx context.Context, now time.Time) {
	type due struct {
		target string
		st     mountState // copy — refreshed outside the lock
	}

	d.mu.Lock()
	var list []due
	for target, st := range d.mounts {
		if st.interval > 0 && now.After(st.nextRefresh) {
			list = append(list, due{target, *st})
		}
	}
	d.mu.Unlock()

	for _, item := range list {
		d.refreshMount(ctx, item.target, item.st)
	}
}

// refreshMount re-fetches every path for one mount and rewrites its files.
// All paths are fetched before anything is written, so a container never
// sees a mix of pre- and post-refresh values from one cycle. On any
// failure, existing files are left untouched, the mount stays tracked, and
// nextRefresh is NOT advanced — the same mount is simply retried on the
// next tick, mirroring the retry-until-success shape of the Operator's
// reconcile (internal/operator/controller.go).
func (d *Driver) refreshMount(ctx context.Context, target string, st mountState) {
	hc, err := httpClient(st.insecure, d.caFile)
	if err != nil {
		slog.Warn("csi: background refresh failed, keeping existing files", "target", target, "err", err)
		return
	}

	allFiles, err := fetchAll(ctx, hc, &st)
	if st.k8s && st.token != "" {
		d.mu.Lock()
		if cur, ok := d.mounts[target]; ok {
			cur.token = st.token
		}
		d.mu.Unlock()
	}
	if err != nil {
		slog.Warn("csi: background refresh failed, keeping existing files", "target", target, "err", err)
		return
	}

	for fname, value := range allFiles {
		dest := filepath.Join(target, fname)
		if err := writeFileAtomic(dest, []byte(value), st.mode); err != nil {
			slog.Warn("csi: background refresh write failed, keeping existing files",
				"target", target, "file", fname, "err", err)
			return
		}
	}

	d.mu.Lock()
	if cur, ok := d.mounts[target]; ok {
		cur.nextRefresh = time.Now().Add(cur.interval)
	}
	d.mu.Unlock()

	slog.Info("csi: refreshed secret files", "target", target, "paths", st.paths)
}

// ─── Secret fetching ─────────────────────────────────────────────────────────

// fetchSecretFiles fetches a secret from Tuck and returns a filename→value map.
//
// When expandKeys is true and the secret value is a JSON object, each top-level
// key becomes a separate file. Otherwise returns a single entry keyed by the
// base name of path.
func fetchSecretFiles(ctx context.Context, hc *http.Client, addr, token, ns, path, kvVersion string, expandKeys bool) (map[string]string, error) {
	var urlPath string
	if kvVersion == "2" {
		urlPath = "/v2/secret/" + path
	} else {
		urlPath = "/v1/secret/" + path
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr+urlPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Tuck-Token", token)
	if ns != "" {
		req.Header.Set("X-Tuck-Namespace", ns)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{code: resp.StatusCode, body: string(body)}
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	v, ok := result["value"]
	if !ok {
		return nil, fmt.Errorf("response missing \"value\" field")
	}
	value := fmt.Sprintf("%v", v)

	if expandKeys {
		var obj map[string]any
		if err := json.Unmarshal([]byte(value), &obj); err == nil && len(obj) > 0 {
			files := make(map[string]string, len(obj))
			for k, fv := range obj {
				files[k] = fmt.Sprintf("%v", fv)
			}
			return files, nil
		}
	}

	return map[string]string{filepath.Base(path): value}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// writeFileAtomic writes data to dest by writing to a temp file in the same
// directory and renaming it into place, so a concurrent reader (a container
// process that already opened the file, or one racing a background refresh)
// never observes a partially-written file. Safe on the tmpfs this package
// mounts (mounter_linux.go): temp file and dest live on the same filesystem,
// so rename(2) is an atomic, same-device metadata operation.
func writeFileAtomic(dest string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dest)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Best-effort: on Windows, MoveFileEx (what os.Rename uses under the
	// hood) refuses to replace a read-only destination — and secret files
	// are typically 0400 by default — even though POSIX rename(2) never
	// cares about the target's permission bits, only the directory's write
	// permission. Clearing it first is a no-op on Linux (the only OS this
	// driver actually mounts on, per mounter_linux.go/mounter_other.go) and
	// a no-op if dest doesn't exist yet, so this is safe to attempt
	// unconditionally rather than gating it behind a build tag.
	_ = os.Chmod(dest, 0o600)
	return os.Rename(tmpPath, dest)
}

func splitPaths(raw string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// httpClient builds the client used to reach Tuck. A CA bundle pins server
// verification to it; tuck.io/insecure takes precedence only if set explicitly.
func httpClient(insecure bool, caFile string) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	switch {
	case insecure:
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 — gated by tuck.io/insecure volume attribute
	case caFile != "":
		pool, err := tlsutil.LoadCAPool(caFile)
		if err != nil {
			return nil, err
		}
		tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}, nil
}

// WithCAFile sets the PEM CA bundle the driver uses to verify Tuck servers.
func (d *Driver) WithCAFile(caFile string) *Driver {
	d.caFile = caFile
	return d
}
