// Package physical defines Tuck's storage abstraction: a dumb key/value
// store. Everything it persists is already ciphertext by the time it arrives
// here — encryption is the barrier's job, one layer up.
package physical

import (
	"context"
	"io"
)

// Entry is a single key/value pair in a storage backend.
type Entry struct {
	Key   string
	Value []byte
}

// Backend is a dumb key/value store. Implementations know nothing about
// encryption; they just persist bytes.
type Backend interface {
	// Get returns the entry for key, or (nil, nil) if it does not exist.
	Get(ctx context.Context, key string) (*Entry, error)
	// Put stores an entry, overwriting any existing value.
	Put(ctx context.Context, entry *Entry) error
	// Delete removes key. Deleting a missing key is not an error.
	Delete(ctx context.Context, key string) error
	// List returns the keys that start with prefix.
	List(ctx context.Context, prefix string) ([]string, error)
	// Snapshot writes a consistent database copy to w (for backup).
	// Not all backends support this — inmem returns an error.
	Snapshot(ctx context.Context, w io.Writer) error
}
