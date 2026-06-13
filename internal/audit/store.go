package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/NAGenaev/tuck/internal/physical"
)

// ErrSinkNotFound is returned when a named sink config does not exist.
var ErrSinkNotFound = errors.New("audit sink not found")

const sinkPrefix = "sys/audit/"

// barrierer is the storage interface used by Store.
type barrierer interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Store persists audit sink configurations in the barrier.
type Store struct {
	barrier barrierer
}

// NewStore creates an audit config Store.
func NewStore(b barrierer) *Store { return &Store{barrier: b} }

// Put saves or replaces a sink configuration.
func (s *Store) Put(ctx context.Context, cfg *SinkConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal sink config: %w", err)
	}
	return s.barrier.Put(ctx, &physical.Entry{Key: sinkPrefix + cfg.Name, Value: data})
}

// Get retrieves a sink configuration by name.
func (s *Store) Get(ctx context.Context, name string) (*SinkConfig, error) {
	e, err := s.barrier.Get(ctx, sinkPrefix+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrSinkNotFound
	}
	var cfg SinkConfig
	if err := json.Unmarshal(e.Value, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal sink config: %w", err)
	}
	return &cfg, nil
}

// Delete removes a sink configuration.
func (s *Store) Delete(ctx context.Context, name string) error {
	return s.barrier.Delete(ctx, sinkPrefix+name)
}

// List returns all sink configuration names.
func (s *Store) List(ctx context.Context) ([]string, error) {
	keys, err := s.barrier.List(ctx, sinkPrefix)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = strings.TrimPrefix(k, sinkPrefix)
	}
	return names, nil
}
