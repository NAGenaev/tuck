package mount

import (
	"context"
	"encoding/json"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

const configPrefix = "sys/mount-config/"

// Config holds the tunable parameters for a single mount.
type Config struct {
	// DefaultLeaseTTL is the TTL applied to secrets that do not specify one.
	// 0 means use the system default.
	DefaultLeaseTTL time.Duration `json:"default_lease_ttl"`
	// MaxLeaseTTL is the upper bound on any lease TTL for this mount.
	// 0 means use the system default.
	MaxLeaseTTL time.Duration `json:"max_lease_ttl"`
	// ForceNoCache disables caching for all reads from this mount.
	ForceNoCache bool `json:"force_no_cache"`
	// AllowedResponseHeaders is a list of additional headers the engine may set.
	AllowedResponseHeaders []string `json:"allowed_response_headers,omitempty"`
	// PassthroughRequestHeaders are headers forwarded to the engine as-is.
	PassthroughRequestHeaders []string `json:"passthrough_request_headers,omitempty"`
	// Description is a human-readable label; mirrors Entry.Description if unset.
	Description string `json:"description,omitempty"`
	// UpdatedAt is the last time this config was written.
	UpdatedAt time.Time `json:"updated_at"`
}

// ConfigStore manages per-mount configuration, stored separately from the
// mount table so that the two can evolve independently.
type ConfigStore struct{ b barrierer }

// NewConfigStore returns a ConfigStore backed by the given barrier.
func NewConfigStore(b barrierer) *ConfigStore { return &ConfigStore{b: b} }

// Get returns the Config for the given mount path.
// Returns a zero-value Config (not an error) if none has been saved yet.
func (s *ConfigStore) Get(ctx context.Context, mountPath string) (Config, error) {
	raw, err := s.b.Get(ctx, configKey(mountPath))
	if err != nil || raw == nil {
		return Config{}, nil
	}
	var c Config
	if err := json.Unmarshal(raw.Value, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Put writes the Config for the given mount path.
func (s *ConfigStore) Put(ctx context.Context, mountPath string, c Config) error {
	c.UpdatedAt = time.Now().UTC()
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.b.Put(ctx, &physical.Entry{Key: configKey(mountPath), Value: b})
}

// Delete removes the Config for the given mount path.
func (s *ConfigStore) Delete(ctx context.Context, mountPath string) error {
	return s.b.Delete(ctx, configKey(mountPath))
}

// List returns the path→Config map for all mounts that have stored config.
func (s *ConfigStore) List(ctx context.Context) (map[string]Config, error) {
	keys, err := s.b.List(ctx, configPrefix)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Config, len(keys))
	for _, k := range keys {
		raw, err := s.b.Get(ctx, k)
		if err != nil || raw == nil {
			continue
		}
		var c Config
		if err := json.Unmarshal(raw.Value, &c); err != nil {
			continue
		}
		// Reconstruct the mount path from the storage key.
		out[mountPathFromKey(k)] = c
	}
	return out, nil
}

func configKey(mountPath string) string {
	return configPrefix + pathKey(normalizePath(mountPath))
}

func mountPathFromKey(k string) string {
	base := k[len(configPrefix):]
	// Reverse the pathKey transform: replace _ with / and add trailing slash.
	import_p := ""
	for _, ch := range base {
		if ch == '_' {
			import_p += "/"
		} else {
			import_p += string(ch)
		}
	}
	if import_p != "" && import_p[len(import_p)-1] != '/' {
		import_p += "/"
	}
	return import_p
}
