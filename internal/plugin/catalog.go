// Package plugin implements the plugin catalog — a registry of external
// secret engine and auth method plugins that can be mounted via the mount
// table. Plugins are identified by type + name (e.g. "secret/my-engine").
// Each entry stores the SHA-256 sum of the binary for integrity verification.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

const catalogPrefix = "sys/plugins/catalog/"

// PluginType classifies the plugin.
type PluginType string

const (
	TypeSecret   PluginType = "secret"   // secret engine plugin
	TypeAuth     PluginType = "auth"     // auth method plugin
	TypeDatabase PluginType = "database" // database plugin
)

var validTypes = map[PluginType]bool{
	TypeSecret:   true,
	TypeAuth:     true,
	TypeDatabase: true,
}

var (
	// ErrNotFound is returned when a plugin is not in the catalog.
	ErrNotFound = pluginError("plugin not found")
	// ErrInvalidType is returned when an unknown plugin type is specified.
	ErrInvalidType = pluginError("invalid plugin type; must be secret, auth, or database")
	// ErrInvalidName is returned when the plugin name is empty or contains "/".
	ErrInvalidName = pluginError("invalid plugin name")
)

type pluginError string

func (e pluginError) Error() string { return string(e) }
func (e pluginError) Is(target error) bool {
	t, ok := target.(pluginError)
	return ok && t == e
}

// Entry describes a plugin in the catalog.
type Entry struct {
	Name        string     `json:"name"`
	Type        PluginType `json:"type"`
	Command     string     `json:"command"`              // path to plugin binary
	SHA256      string     `json:"sha256"`               // hex-encoded SHA-256 of binary
	Args        []string   `json:"args,omitempty"`       // optional arguments
	Env         []string   `json:"env,omitempty"`        // optional env vars (VAR=value)
	Builtin     bool       `json:"builtin"`              // true for pre-registered plugins
	Version     string     `json:"version,omitempty"`
	RegisteredAt time.Time `json:"registered_at"`
}

// barrierer is the subset of barrier.Barrier used by the catalog.
type barrierer interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Catalog manages the persisted plugin registry.
type Catalog struct{ b barrierer }

// New returns a Catalog backed by the given barrier.
func New(b barrierer) *Catalog { return &Catalog{b: b} }

// Register adds or replaces a plugin entry. Command and SHA256 are required.
func (c *Catalog) Register(ctx context.Context, e *Entry) error {
	if err := validate(e); err != nil {
		return err
	}
	e.RegisteredAt = time.Now().UTC()
	e.Builtin = false
	return c.put(ctx, e)
}

// RegisterBuiltin registers a builtin plugin (idempotent — skip if already present).
func (c *Catalog) RegisterBuiltin(ctx context.Context, e *Entry) error {
	if _, err := c.Get(ctx, e.Type, e.Name); err == nil {
		return nil
	}
	e.RegisteredAt = time.Now().UTC()
	e.Builtin = true
	return c.put(ctx, e)
}

// Get returns the entry for (pluginType, name).
func (c *Catalog) Get(ctx context.Context, t PluginType, name string) (*Entry, error) {
	raw, err := c.b.Get(ctx, key(t, name))
	if err != nil || raw == nil {
		return nil, ErrNotFound
	}
	var e Entry
	if err := json.Unmarshal(raw.Value, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// Delete removes a plugin from the catalog (builtin plugins may be deleted).
func (c *Catalog) Delete(ctx context.Context, t PluginType, name string) error {
	if _, err := c.Get(ctx, t, name); err != nil {
		return ErrNotFound
	}
	return c.b.Delete(ctx, key(t, name))
}

// List returns all plugin entries. If t is non-empty, only entries of that type
// are returned.
func (c *Catalog) List(ctx context.Context, t PluginType) ([]*Entry, error) {
	prefix := catalogPrefix
	if t != "" {
		prefix += string(t) + "/"
	}
	keys, err := c.b.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]*Entry, 0, len(keys))
	for _, k := range keys {
		raw, err := c.b.Get(ctx, k)
		if err != nil || raw == nil {
			continue
		}
		var e Entry
		if err := json.Unmarshal(raw.Value, &e); err != nil {
			continue
		}
		out = append(out, &e)
	}
	return out, nil
}

func (c *Catalog) put(ctx context.Context, e *Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return c.b.Put(ctx, &physical.Entry{Key: key(e.Type, e.Name), Value: b})
}

func key(t PluginType, name string) string {
	return catalogPrefix + string(t) + "/" + name
}

func validate(e *Entry) error {
	if !validTypes[e.Type] {
		return ErrInvalidType
	}
	if e.Name == "" || strings.Contains(e.Name, "/") {
		return ErrInvalidName
	}
	return nil
}

// Sentinel check.
var _ = errors.Is(ErrNotFound, ErrNotFound)
