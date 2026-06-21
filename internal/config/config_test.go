package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates a temp file with the given content and returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	return path
}

const fullYAML = `
addr: ":9200"
data: /var/tuck

tls:
  cert: /etc/tuck/cert.pem
  key:  /etc/tuck/key.pem
  auto: true

seal:
  type: shamir
  shamir:
    n: 5
    k: 3
  transit:
    addr: https://vault.example.com
    key:  tuck-root
    key_file: /run/secrets/transit-token
  aws_kms:
    key_id: alias/tuck
    region: us-east-1
  gcp_kms:
    key_name: projects/p/locations/global/keyRings/r/cryptoKeys/k
  azure_kv:
    vault_url: https://kv.vault.azure.net
    key_name:  tuck-key
    algorithm: RSA-OAEP-256

cluster:
  enabled:   true
  node_id:   node1
  bind_addr: ":7000"
  advertise: "10.0.0.1:7000"
  dir:       /var/tuck/raft
  bootstrap: true
  join:      "10.0.0.2:7000"
  peers:     "10.0.0.1:7000,10.0.0.2:7000"

kubernetes:
  api:        https://k8s.local
  token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
  ca_file:    /var/run/secrets/kubernetes.io/serviceaccount/ca.crt

telemetry:
  otel_endpoint: http://otel-collector:4318
`

// TestLoad_ExplicitPath verifies that Load reads a file at the given path.
func TestLoad_ExplicitPath(t *testing.T) {
	path := writeFile(t, "tuck.yaml", fullYAML)
	cfg, got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != path {
		t.Errorf("returned path = %q, want %q", got, path)
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
	}
	if cfg.Addr != ":9200" {
		t.Errorf("Addr = %q, want :9200", cfg.Addr)
	}
	if cfg.Data != "/var/tuck" {
		t.Errorf("Data = %q, want /var/tuck", cfg.Data)
	}
}

// TestLoad_TLSFields verifies TLS sub-struct is populated correctly.
func TestLoad_TLSFields(t *testing.T) {
	path := writeFile(t, "tuck.yaml", fullYAML)
	cfg, _, _ := Load(path)
	if cfg.TLS.Cert != "/etc/tuck/cert.pem" {
		t.Errorf("TLS.Cert = %q, want /etc/tuck/cert.pem", cfg.TLS.Cert)
	}
	if cfg.TLS.Key != "/etc/tuck/key.pem" {
		t.Errorf("TLS.Key = %q, want /etc/tuck/key.pem", cfg.TLS.Key)
	}
	if !cfg.TLS.Auto {
		t.Error("TLS.Auto = false, want true")
	}
}

// TestLoad_SealFields verifies all seal sub-structs are populated.
func TestLoad_SealFields(t *testing.T) {
	path := writeFile(t, "tuck.yaml", fullYAML)
	cfg, _, _ := Load(path)

	if cfg.Seal.Type != "shamir" {
		t.Errorf("Seal.Type = %q, want shamir", cfg.Seal.Type)
	}
	if cfg.Seal.Shamir.N != 5 || cfg.Seal.Shamir.K != 3 {
		t.Errorf("Shamir N/K = %d/%d, want 5/3", cfg.Seal.Shamir.N, cfg.Seal.Shamir.K)
	}
	if cfg.Seal.Transit.Addr != "https://vault.example.com" {
		t.Errorf("Transit.Addr = %q, want https://vault.example.com", cfg.Seal.Transit.Addr)
	}
	if cfg.Seal.Transit.Key != "tuck-root" {
		t.Errorf("Transit.Key = %q, want tuck-root", cfg.Seal.Transit.Key)
	}
	if cfg.Seal.AWSKMS.KeyID != "alias/tuck" {
		t.Errorf("AWSKMS.KeyID = %q, want alias/tuck", cfg.Seal.AWSKMS.KeyID)
	}
	if cfg.Seal.AWSKMS.Region != "us-east-1" {
		t.Errorf("AWSKMS.Region = %q, want us-east-1", cfg.Seal.AWSKMS.Region)
	}
	if cfg.Seal.GCPKMS.KeyName == "" {
		t.Error("GCPKMS.KeyName is empty")
	}
	if cfg.Seal.AzureKV.VaultURL != "https://kv.vault.azure.net" {
		t.Errorf("AzureKV.VaultURL = %q", cfg.Seal.AzureKV.VaultURL)
	}
	if cfg.Seal.AzureKV.Algorithm != "RSA-OAEP-256" {
		t.Errorf("AzureKV.Algorithm = %q, want RSA-OAEP-256", cfg.Seal.AzureKV.Algorithm)
	}
}

// TestLoad_ClusterFields verifies the cluster sub-struct.
func TestLoad_ClusterFields(t *testing.T) {
	path := writeFile(t, "tuck.yaml", fullYAML)
	cfg, _, _ := Load(path)

	if !cfg.Cluster.Enabled {
		t.Error("Cluster.Enabled = false, want true")
	}
	if cfg.Cluster.NodeID != "node1" {
		t.Errorf("Cluster.NodeID = %q, want node1", cfg.Cluster.NodeID)
	}
	if cfg.Cluster.BindAddr != ":7000" {
		t.Errorf("Cluster.BindAddr = %q, want :7000", cfg.Cluster.BindAddr)
	}
	if !cfg.Cluster.Bootstrap {
		t.Error("Cluster.Bootstrap = false, want true")
	}
	if cfg.Cluster.Peers == "" {
		t.Error("Cluster.Peers is empty")
	}
}

// TestLoad_KubernetesAndTelemetry verifies remaining sub-structs.
func TestLoad_KubernetesAndTelemetry(t *testing.T) {
	path := writeFile(t, "tuck.yaml", fullYAML)
	cfg, _, _ := Load(path)

	if cfg.Kubernetes.API != "https://k8s.local" {
		t.Errorf("Kubernetes.API = %q, want https://k8s.local", cfg.Kubernetes.API)
	}
	if cfg.Telemetry.OtelEndpoint != "http://otel-collector:4318" {
		t.Errorf("Telemetry.OtelEndpoint = %q", cfg.Telemetry.OtelEndpoint)
	}
}

// TestLoad_EnvVar verifies fallback to $TUCK_CONFIG when path is empty.
func TestLoad_EnvVar(t *testing.T) {
	path := writeFile(t, "via-env.yaml", `addr: ":8888"`)
	t.Setenv("TUCK_CONFIG", path)

	cfg, got, err := Load("")
	if err != nil {
		t.Fatalf("Load via env: %v", err)
	}
	if got != path {
		t.Errorf("returned path = %q, want %q", got, path)
	}
	if cfg.Addr != ":8888" {
		t.Errorf("Addr = %q, want :8888", cfg.Addr)
	}
}

// TestLoad_NoConfigAnywhere verifies (nil, "", nil) when nothing is found.
func TestLoad_NoConfigAnywhere(t *testing.T) {
	t.Setenv("TUCK_CONFIG", "")

	// Create the empty temp dir first, then register chdir-back AFTER so that
	// LIFO cleanup order is: chdir-back first, then TempDir removal.
	// (On Windows the directory cannot be deleted while it is the CWD.)
	emptyDir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(emptyDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg, path, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("cfg = %+v, want nil", cfg)
	}
	if path != "" {
		t.Errorf("path = %q, want empty", path)
	}
}

// TestLoad_AutoDetectTuckYAML verifies that tuck.yaml in CWD is found automatically.
func TestLoad_AutoDetectTuckYAML(t *testing.T) {
	t.Setenv("TUCK_CONFIG", "")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tuck.yaml"), []byte(`addr: ":7777"`), 0o600); err != nil {
		t.Fatal(err)
	}
	orig, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	cfg, got, err := Load("")
	if err != nil {
		t.Fatalf("Load auto-detect: %v", err)
	}
	if got != "tuck.yaml" {
		t.Errorf("path = %q, want tuck.yaml", got)
	}
	if cfg.Addr != ":7777" {
		t.Errorf("Addr = %q, want :7777", cfg.Addr)
	}
}

// TestLoad_InvalidYAML verifies a parse error is returned for malformed YAML.
func TestLoad_InvalidYAML(t *testing.T) {
	path := writeFile(t, "bad.yaml", "addr: [\nunclosed bracket")
	_, _, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// TestLoad_NonExistentPath verifies a read error is returned for a missing file.
func TestLoad_NonExistentPath(t *testing.T) {
	_, _, err := Load("/no/such/file/tuck.yaml")
	if err == nil {
		t.Error("expected error for non-existent path, got nil")
	}
}

// TestLoad_EmptyFile verifies that an empty YAML file returns a zero Config.
func TestLoad_EmptyFile(t *testing.T) {
	path := writeFile(t, "empty.yaml", "")
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg is nil for empty file")
	}
	if cfg.Addr != "" || cfg.Data != "" {
		t.Errorf("non-zero fields in empty config: %+v", cfg)
	}
}

// TestLoad_PartialConfig verifies that only specified fields are set.
func TestLoad_PartialConfig(t *testing.T) {
	path := writeFile(t, "partial.yaml", "addr: \":1234\"\ntls:\n  auto: true\n")
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load partial: %v", err)
	}
	if cfg.Addr != ":1234" {
		t.Errorf("Addr = %q, want :1234", cfg.Addr)
	}
	if !cfg.TLS.Auto {
		t.Error("TLS.Auto = false, want true")
	}
	// Unset fields must stay zero.
	if cfg.Data != "" {
		t.Errorf("Data = %q, want empty", cfg.Data)
	}
	if cfg.Seal.Type != "" {
		t.Errorf("Seal.Type = %q, want empty", cfg.Seal.Type)
	}
}

// TestLoad_ExplicitPathOverridesEnv verifies explicit path takes priority over env var.
func TestLoad_ExplicitPathOverridesEnv(t *testing.T) {
	envPath := writeFile(t, "env.yaml", `addr: ":1111"`)
	explicitPath := writeFile(t, "explicit.yaml", `addr: ":2222"`)
	t.Setenv("TUCK_CONFIG", envPath)

	cfg, got, err := Load(explicitPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != explicitPath {
		t.Errorf("path = %q, want %q", got, explicitPath)
	}
	if cfg.Addr != ":2222" {
		t.Errorf("Addr = %q, want :2222 (explicit should win over env)", cfg.Addr)
	}
}
