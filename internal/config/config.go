// Package config loads Tuck server configuration from a YAML file.
//
// Priority (highest wins): CLI flag > environment variable > config file > built-in default.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the complete server configuration.  Fields map 1-to-1 to CLI flags;
// zero values mean "not set in the file — use the flag default".
type Config struct {
	Addr string `yaml:"addr"`
	Data string `yaml:"data"`

	TLS struct {
		Cert string `yaml:"cert"`
		Key  string `yaml:"key"`
		Auto bool   `yaml:"auto"`
	} `yaml:"tls"`

	Seal struct {
		Type string `yaml:"type"`

		Dev struct {
			Key string `yaml:"key"`
		} `yaml:"dev"`

		Shamir struct {
			Config string `yaml:"config"`
			N      int    `yaml:"n"`
			K      int    `yaml:"k"`
		} `yaml:"shamir"`

		Transit struct {
			Addr    string `yaml:"addr"`
			Key     string `yaml:"key"`
			// Token is intentionally not written to disk here; use env TUCK_TRANSIT_TOKEN.
			Token   string `yaml:"token"`
			KeyFile string `yaml:"key_file"`
		} `yaml:"transit"`

	} `yaml:"seal"`

	Cluster struct {
		Enabled   bool   `yaml:"enabled"`
		NodeID    string `yaml:"node_id"`
		BindAddr  string `yaml:"bind_addr"`
		Advertise string `yaml:"advertise"`
		Dir       string `yaml:"dir"`
		Bootstrap bool   `yaml:"bootstrap"`
		Join      string `yaml:"join"`
		Peers     string `yaml:"peers"`
		// LabelLeader publishes the Raft role as the pod label tuck.io/leader.
		LabelLeader bool `yaml:"label_leader"`

		TLSCert       string `yaml:"tls_cert"`
		TLSKey        string `yaml:"tls_key"`
		TLSCA         string `yaml:"tls_ca"`
		TLSServerName string `yaml:"tls_server_name"`
	} `yaml:"cluster"`

	Kubernetes struct {
		API       string `yaml:"api"`
		TokenFile string `yaml:"token_file"`
		CAFile    string `yaml:"ca_file"`
		// Clusters lists additional trusted clusters (multi-cluster setups).
		// The top-level api/token_file/ca_file stay the default cluster.
		Clusters []K8sCluster `yaml:"clusters"`
	} `yaml:"kubernetes"`

	Barrier struct {
		// Cipher selects the at-rest cipher suite for a NEW data store:
		// "aes-256-gcm" (default) or "gost-kuznyechik-mgm". Fixed at init.
		Cipher string `yaml:"cipher"`
	} `yaml:"barrier"`


	// TrustedProxies lists load balancers (IP or CIDR) whose X-Forwarded-For
	// header is believed when deriving the client address.
	TrustedProxies []string `yaml:"trusted_proxies"`

	// RequireMlock makes startup fail when process memory cannot be locked.
	RequireMlock bool `yaml:"require_mlock"`

	Telemetry struct {
		OtelEndpoint string `yaml:"otel_endpoint"`
	} `yaml:"telemetry"`
}

// Load reads a YAML config file and returns the populated Config and the
// resolved file path.  If path is empty, it tries $TUCK_CONFIG, then
// "tuck.yaml" in the working directory.  Returns (nil, "", nil) when no
// config file is found anywhere.
func Load(path string) (*Config, string, error) {
	if path == "" {
		path = os.Getenv("TUCK_CONFIG")
	}
	if path == "" {
		if _, err := os.Stat("tuck.yaml"); err == nil {
			path = "tuck.yaml"
		}
	}
	if path == "" {
		return nil, "", nil
	}

	data, err := os.ReadFile(path) // #nosec G304 G703 — path is operator-supplied via flag/env, not user input
	if err != nil {
		return nil, "", fmt.Errorf("read config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, "", fmt.Errorf("parse config %q: %w", path, err)
	}
	return &cfg, path, nil
}

// K8sCluster is one extra trusted Kubernetes cluster. Tuck verifies
// ServiceAccount tokens against its TokenReview API using TokenFile, a
// reviewer token that must be allowed to create tokenreviews there.
type K8sCluster struct {
	Name      string `yaml:"name"`
	API       string `yaml:"api"`
	TokenFile string `yaml:"token_file"`
	CAFile    string `yaml:"ca_file"`
}
