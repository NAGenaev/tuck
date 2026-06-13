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
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	csispec "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	DriverName    = "secrets.tuck.io"
	DriverVersion = "1.5.0"

	// Volume context keys supplied by the StorageClass / PVC attributes.
	ctxAddr      = "tuck.io/addr"       // e.g. "https://tuck:8200"
	ctxPaths     = "tuck.io/paths"      // comma-separated secret paths
	ctxNamespace = "tuck.io/namespace"  // optional Tuck namespace
	ctxKVVersion = "tuck.io/kv-version" // "1" or "2" (default "1")
	ctxInsecure  = "tuck.io/insecure"   // "true" to skip TLS verification

	// Secret key holding the Tuck token, passed via NodePublishSecrets.
	secretKeyToken = "token"
)

// Driver implements the CSI Identity and Node gRPC services.
type Driver struct {
	csispec.UnimplementedIdentityServer
	csispec.UnimplementedNodeServer
	csispec.UnimplementedControllerServer

	nodeID string
	mounter Mounter
}

// NewDriver creates a Driver for the given node.
func NewDriver(nodeID string, mounter Mounter) *Driver {
	return &Driver{nodeID: nodeID, mounter: mounter}
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

	token := req.GetSecrets()[secretKeyToken]
	if token == "" {
		return nil, status.Error(codes.InvalidArgument, "node publish secret \"token\" is required")
	}

	ns := vc[ctxNamespace]
	kvVersion := vc[ctxKVVersion]
	if kvVersion == "" {
		kvVersion = "1"
	}
	insecure := strings.EqualFold(vc[ctxInsecure], "true")

	paths := splitPaths(pathsRaw)

	// Ensure target dir exists.
	if err := os.MkdirAll(req.TargetPath, 0o750); err != nil {
		return nil, status.Errorf(codes.Internal, "mkdir target path: %v", err)
	}

	// Mount tmpfs so secrets never touch disk.
	if err := d.mounter.MountTmpfs(req.TargetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "mount tmpfs: %v", err)
	}

	hc := httpClient(insecure)
	for _, p := range paths {
		value, err := fetchSecret(ctx, hc, addr, token, ns, p, kvVersion)
		if err != nil {
			_ = d.mounter.Unmount(req.TargetPath)
			return nil, status.Errorf(codes.Internal, "fetch secret %q: %v", p, err)
		}
		fname := filepath.Base(p)
		dest := filepath.Join(req.TargetPath, fname)
		if err := os.WriteFile(dest, []byte(value), 0o400); err != nil {
			_ = d.mounter.Unmount(req.TargetPath)
			return nil, status.Errorf(codes.Internal, "write secret file %q: %v", fname, err)
		}
	}
	return &csispec.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the tmpfs and removes the target directory.
func (d *Driver) NodeUnpublishVolume(_ context.Context, req *csispec.NodeUnpublishVolumeRequest) (*csispec.NodeUnpublishVolumeResponse, error) {
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path is required")
	}
	if err := d.mounter.Unmount(req.TargetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "unmount: %v", err)
	}
	_ = os.RemoveAll(req.TargetPath)
	return &csispec.NodeUnpublishVolumeResponse{}, nil
}

// ─── Secret fetching ─────────────────────────────────────────────────────────

func fetchSecret(ctx context.Context, hc *http.Client, addr, token, ns, path, kvVersion string) (string, error) {
	var urlPath string
	if kvVersion == "2" {
		urlPath = "/v2/secret/" + path
	} else {
		urlPath = "/v1/secret/" + path
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr+urlPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Tuck-Token", token)
	if ns != "" {
		req.Header.Set("X-Tuck-Namespace", ns)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tuck API returned HTTP %d: %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	v, ok := result["value"]
	if !ok {
		return "", fmt.Errorf("response missing \"value\" field")
	}
	return fmt.Sprintf("%v", v), nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func splitPaths(raw string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func httpClient(insecure bool) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 — gated by tuck.io/insecure volume attribute
	}
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}
