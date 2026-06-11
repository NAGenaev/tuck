// Package core wires together the seal, the cryptographic barrier, and the
// logical secret operations. It is the top-level object the server talks to.
package core

import (
	"context"
	"fmt"
	"path"

	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

// secretPrefix namespaces user secrets in the backend, keeping them clear of
// the barrier's reserved "barrier/" keyring.
const secretPrefix = "secret/"

// Core is Tuck's runtime: storage + crypto + seal, plus the logical KV API.
type Core struct {
	backend physical.Backend
	barrier *barrier.Barrier
	seal    seal.Seal
}

// New builds a Core over the given backend and seal.
func New(backend physical.Backend, s seal.Seal) *Core {
	return &Core{
		backend: backend,
		barrier: barrier.New(backend),
		seal:    s,
	}
}

// Start brings the core up: it initializes a fresh backend if needed, then
// unseals. With the dev seal this is fully automatic — zero ceremony.
func (c *Core) Start(ctx context.Context) error {
	inited, err := c.barrier.Initialized(ctx)
	if err != nil {
		return fmt.Errorf("check initialized: %w", err)
	}
	if !inited {
		rootKey, err := c.seal.Init()
		if err != nil {
			return fmt.Errorf("seal init: %w", err)
		}
		if err := c.barrier.Initialize(ctx, rootKey); err != nil {
			return fmt.Errorf("barrier init: %w", err)
		}
	}
	rootKey, err := c.seal.Unseal()
	if err != nil {
		return fmt.Errorf("seal unseal: %w", err)
	}
	if err := c.barrier.Unseal(ctx, rootKey); err != nil {
		return fmt.Errorf("barrier unseal: %w", err)
	}
	return nil
}

// Sealed reports whether the core is currently sealed.
func (c *Core) Sealed() bool { return c.barrier.IsSealed() }

// Seal re-seals the core, dropping the in-memory key.
func (c *Core) Seal() { c.barrier.Seal() }

// GetSecret returns the bytes stored at the logical path, or (nil, false) if
// nothing is stored there.
func (c *Core) GetSecret(ctx context.Context, p string) ([]byte, bool, error) {
	e, err := c.barrier.Get(ctx, secretKey(p))
	if err != nil {
		return nil, false, err
	}
	if e == nil {
		return nil, false, nil
	}
	return e.Value, true, nil
}

// PutSecret stores bytes at the logical path.
func (c *Core) PutSecret(ctx context.Context, p string, value []byte) error {
	return c.barrier.Put(ctx, &physical.Entry{Key: secretKey(p), Value: value})
}

// DeleteSecret removes the secret at the logical path.
func (c *Core) DeleteSecret(ctx context.Context, p string) error {
	return c.barrier.Delete(ctx, secretKey(p))
}

// secretKey maps a logical path to a backend key, cleaning it so a request
// cannot escape the secret/ namespace via "..".
func secretKey(p string) string {
	return secretPrefix + path.Clean("/"+p)[1:]
}
