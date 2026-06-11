// Package core wires together the seal, the cryptographic barrier, and the
// logical secret operations. It is the top-level object the server talks to.
package core

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/policy"
	"github.com/NAGenaev/tuck/internal/seal"
	"github.com/NAGenaev/tuck/internal/token"
)

const secretPrefix = "secret/"

var (
	ErrUnauthorized = errors.New("permission denied")
	ErrTokenInvalid = errors.New("invalid or expired token")
)

// Core is Tuck's runtime: storage + crypto + seal, plus the logical KV and identity APIs.
type Core struct {
	backend  physical.Backend
	barrier  *barrier.Barrier
	seal     seal.Seal
	tokens   *token.Store
	policies *policy.Store
}

// New builds a Core over the given backend and seal.
func New(backend physical.Backend, s seal.Seal) *Core {
	b := barrier.New(backend)
	return &Core{
		backend:  backend,
		barrier:  b,
		seal:     s,
		tokens:   token.NewStore(b),
		policies: policy.NewStore(b),
	}
}

// Start brings the core up. On first initialisation it returns the root token
// — print it, it will never be shown again. On subsequent starts it returns nil.
func (c *Core) Start(ctx context.Context) (*token.Token, error) {
	inited, err := c.barrier.Initialized(ctx)
	if err != nil {
		return nil, fmt.Errorf("check initialized: %w", err)
	}
	if !inited {
		rootKey, err := c.seal.Init()
		if err != nil {
			return nil, fmt.Errorf("seal init: %w", err)
		}
		if err := c.barrier.Initialize(ctx, rootKey); err != nil {
			return nil, fmt.Errorf("barrier init: %w", err)
		}
		if err := c.barrier.Unseal(ctx, rootKey); err != nil {
			return nil, fmt.Errorf("barrier unseal: %w", err)
		}
		rootTok, err := token.Generate(token.RootTokenDisplayName, []string{token.RootPolicyName}, 0)
		if err != nil {
			return nil, fmt.Errorf("generate root token: %w", err)
		}
		if err := c.tokens.Put(ctx, rootTok); err != nil {
			return nil, fmt.Errorf("store root token: %w", err)
		}
		return rootTok, nil
	}
	rootKey, err := c.seal.Unseal()
	if err != nil {
		return nil, fmt.Errorf("seal unseal: %w", err)
	}
	if err := c.barrier.Unseal(ctx, rootKey); err != nil {
		return nil, fmt.Errorf("barrier unseal: %w", err)
	}
	return nil, nil
}

// Sealed reports whether the core is currently sealed.
func (c *Core) Sealed() bool { return c.barrier.IsSealed() }

// Seal re-seals the core, dropping the in-memory key.
func (c *Core) Seal() { c.barrier.Seal() }

// --- Identity ---

// Authenticate looks up tokenID and validates it. Returns ErrTokenInvalid on
// any failure so callers cannot distinguish missing from expired.
func (c *Core) Authenticate(ctx context.Context, tokenID string) (*token.Token, error) {
	tok, err := c.tokens.Get(ctx, tokenID)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	if tok.IsExpired() {
		return nil, ErrTokenInvalid
	}
	return tok, nil
}

// EnforceAccess authenticates tokenID then checks that cap is permitted on
// path. path must be the full logical path including the mount prefix
// (e.g. "secret/db/password").
func (c *Core) EnforceAccess(ctx context.Context, tokenID, logicalPath string, cap policy.Capability) error {
	tok, err := c.Authenticate(ctx, tokenID)
	if err != nil {
		return err
	}
	policies, err := c.resolvePolicies(ctx, tok.Policies)
	if err != nil {
		return fmt.Errorf("resolve policies: %w", err)
	}
	if !policy.Allowed(policies, logicalPath, cap) {
		return ErrUnauthorized
	}
	return nil
}

// resolvePolicies loads named policies. "root" is never stored — it is
// injected directly so it cannot be accidentally deleted via the API.
func (c *Core) resolvePolicies(ctx context.Context, names []string) ([]policy.Policy, error) {
	out := make([]policy.Policy, 0, len(names))
	for _, name := range names {
		if name == token.RootPolicyName {
			out = append(out, policy.RootPolicy)
			continue
		}
		p, err := c.policies.Get(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("policy %q: %w", name, err)
		}
		out = append(out, *p)
	}
	return out, nil
}

// --- Token management ---

func (c *Core) CreateToken(ctx context.Context, displayName string, policies []string, ttl time.Duration) (*token.Token, error) {
	tok, err := token.Generate(displayName, policies, ttl)
	if err != nil {
		return nil, err
	}
	return tok, c.tokens.Put(ctx, tok)
}

func (c *Core) RevokeToken(ctx context.Context, tokenID string) error {
	return c.tokens.Delete(ctx, tokenID)
}

func (c *Core) LookupToken(ctx context.Context, tokenID string) (*token.Token, error) {
	return c.tokens.Get(ctx, tokenID)
}

// --- Policy management ---

func (c *Core) PutPolicy(ctx context.Context, p *policy.Policy) error {
	return c.policies.Put(ctx, p)
}

func (c *Core) GetPolicy(ctx context.Context, name string) (*policy.Policy, error) {
	return c.policies.Get(ctx, name)
}

func (c *Core) DeletePolicy(ctx context.Context, name string) error {
	return c.policies.Delete(ctx, name)
}

// --- KV secrets ---

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
