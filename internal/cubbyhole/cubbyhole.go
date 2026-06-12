// Package cubbyhole implements per-token private storage.
//
// Each Tuck token has its own isolated cubbyhole namespace. No other token
// can read or write a different token's cubbyhole. All entries for a token
// are automatically purged when the token is revoked or expires.
//
// Storage layout:
//
//	cubbyhole/<token_id>/<path>  →  JSON object
package cubbyhole

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/NAGenaev/tuck/internal/physical"
)

var ErrNotFound = errors.New("cubbyhole: not found")

const storagePrefix = "cubbyhole/"

type barrierIface interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Store manages per-token cubbyhole storage.
type Store struct {
	b barrierIface
}

// NewStore returns a Store backed by b.
func NewStore(b barrierIface) *Store {
	return &Store{b: b}
}

// Get retrieves the JSON object at path inside tokenID's cubbyhole.
func (s *Store) Get(ctx context.Context, tokenID, path string) (map[string]interface{}, error) {
	entry, err := s.b.Get(ctx, s.key(tokenID, path))
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, ErrNotFound
	}
	var data map[string]interface{}
	if err := json.Unmarshal(entry.Value, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// Put stores a JSON object at path inside tokenID's cubbyhole.
func (s *Store) Put(ctx context.Context, tokenID, path string, data map[string]interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return s.b.Put(ctx, &physical.Entry{Key: s.key(tokenID, path), Value: raw})
}

// Delete removes the entry at path inside tokenID's cubbyhole.
func (s *Store) Delete(ctx context.Context, tokenID, path string) error {
	return s.b.Delete(ctx, s.key(tokenID, path))
}

// List returns the logical keys under pathPrefix inside tokenID's cubbyhole.
func (s *Store) List(ctx context.Context, tokenID, pathPrefix string) ([]string, error) {
	fullPrefix := storagePrefix + tokenID + "/" + pathPrefix
	keys, err := s.b.List(ctx, fullPrefix)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, strings.TrimPrefix(k, fullPrefix))
	}
	return out, nil
}

// PurgeToken deletes all cubbyhole entries for tokenID.
// Called when a token is revoked or expires.
func (s *Store) PurgeToken(ctx context.Context, tokenID string) error {
	prefix := storagePrefix + tokenID + "/"
	keys, err := s.b.List(ctx, prefix)
	if err != nil {
		return err
	}
	for _, k := range keys {
		_ = s.b.Delete(ctx, k)
	}
	return nil
}

func (s *Store) key(tokenID, path string) string {
	return storagePrefix + tokenID + "/" + path
}
