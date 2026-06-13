// Package sysconfig stores and retrieves server-wide runtime settings via the
// barrier so that operators can tune behaviour without a restart.
package sysconfig

import (
	"context"
	"encoding/json"

	"github.com/NAGenaev/tuck/internal/physical"
)

const configKey = "sys/config"

// Config holds all live-tunable server settings. Zero values mean "use the
// server default"; callers should check for zero before applying.
type Config struct {
	// IPRateLimitRPS is the per-IP request rate (requests/second). 0 = disabled.
	IPRateLimitRPS float64 `json:"ip_rate_limit_rps,omitempty"`
	// IPRateLimitBurst is the per-IP burst size. 0 = disabled.
	IPRateLimitBurst int `json:"ip_rate_limit_burst,omitempty"`

	// TokenRateLimitRPS is the per-token request rate (requests/second). 0 = disabled.
	TokenRateLimitRPS float64 `json:"token_rate_limit_rps,omitempty"`
	// TokenRateLimitBurst is the per-token burst size. 0 = disabled.
	TokenRateLimitBurst int `json:"token_rate_limit_burst,omitempty"`

	// MaxBodyBytes is the maximum accepted request body size. 0 = 1 MiB default.
	MaxBodyBytes int64 `json:"max_body_bytes,omitempty"`

	// TokenGCIntervalSec is the token GC sweep interval in seconds. 0 = server default.
	TokenGCIntervalSec int `json:"token_gc_interval_sec,omitempty"`
}

type barrierer interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
}

// Store persists and retrieves the server Config via the barrier.
type Store struct{ b barrierer }

// New returns a Store backed by the given barrier.
func New(b barrierer) *Store { return &Store{b: b} }

// Get returns the current config. Returns zero Config if none has been saved.
func (s *Store) Get(ctx context.Context) (Config, error) {
	e, err := s.b.Get(ctx, configKey)
	if err != nil || e == nil {
		return Config{}, err
	}
	var c Config
	return c, json.Unmarshal(e.Value, &c)
}

// Put stores the config.
func (s *Store) Put(ctx context.Context, c Config) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.b.Put(ctx, &physical.Entry{Key: configKey, Value: raw})
}
