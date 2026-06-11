// Package core wires together the seal, the cryptographic barrier, and the
// logical secret operations. It is the top-level object the server talks to.
package core

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sync"
	"time"

	"github.com/NAGenaev/tuck/internal/barrier"
	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/policy"
	"github.com/NAGenaev/tuck/internal/seal"
	"github.com/NAGenaev/tuck/internal/token"
)

const secretPrefix = "secret/"

var (
	ErrUnauthorized    = errors.New("permission denied")
	ErrTokenInvalid    = errors.New("invalid or expired token")
	ErrK8sAuthDisabled = errors.New("kubernetes auth is not configured")

	// ErrNeedsUnseal is returned by Start when the seal requires interactive
	// shard collection (e.g. ShamirSeal). The server remains running; callers
	// should poll SealStatus and surface POST /v1/sys/unseal to operators.
	ErrNeedsUnseal = errors.New("core: seal requires manual unseal — supply shards via /v1/sys/unseal")

	// ErrSealNotInteractive is returned by UnsealShard when the active seal
	// does not implement SharableUnseal (e.g. Dev or Transit seals).
	ErrSealNotInteractive = errors.New("core: active seal does not support interactive shard unseal")
)

// SealStatusInfo is returned by Core.SealStatus and exposed via
// GET /v1/sys/seal-status.
type SealStatusInfo struct {
	Sealed   bool   `json:"sealed"`
	Type     string `json:"type"`
	Required int    `json:"required_shards,omitempty"` // non-zero for ShamirSeal only
	Received int    `json:"received_shards,omitempty"` // non-zero for ShamirSeal only
}

// Core is Tuck's runtime: storage + crypto + seal, plus the logical KV and identity APIs.
type Core struct {
	backend  physical.Backend
	barrier  *barrier.Barrier
	seal     seal.Seal
	tokens   *token.Store
	policies *policy.Store
	// optional — nil means k8s auth is disabled
	k8sReviewer k8sauth.Reviewer
	k8sRoles    *k8sauth.RoleStore

	// unsealMu guards unseal-shard operations so concurrent POST /v1/sys/unseal
	// requests cannot race on AcceptShard.
	unsealMu sync.Mutex

	// unsealCtx is stored during the "waiting for shards" window so that
	// UnsealShard can call barrier.Unseal once the key is reconstructed.
	unsealCtx context.Context //nolint:containedctx // intentional: stored for deferred barrier.Unseal
}

// New builds a Core without Kubernetes auth support.
func New(backend physical.Backend, s seal.Seal) *Core {
	return NewWithK8s(backend, s, nil)
}

// NewWithK8s builds a Core with an optional Kubernetes Reviewer.
// Pass nil to disable Kubernetes auth.
func NewWithK8s(backend physical.Backend, s seal.Seal, reviewer k8sauth.Reviewer) *Core {
	b := barrier.New(backend)
	return &Core{
		backend:     backend,
		barrier:     b,
		seal:        s,
		tokens:      token.NewStore(b),
		policies:    policy.NewStore(b),
		k8sReviewer: reviewer,
		k8sRoles:    k8sauth.NewRoleStore(b),
	}
}

// StartResult is returned by Core.Start on the first initialisation.
// On subsequent starts it is nil (auto-unseal seals) or ErrNeedsUnseal is
// returned (ShamirSeal).
type StartResult struct {
	// RootToken is the bootstrap token. Print it once and store it securely —
	// it is never accessible again via the API.
	RootToken *token.Token
	// Shares is non-nil only on the very first boot with a ShamirSeal. These
	// are the base64url-encoded shares that must be distributed to operators.
	// After Start returns, they are no longer accessible.
	Shares []string
}

// Start brings the core up. On first initialisation it returns a StartResult
// containing the root token (and shares if ShamirSeal). On subsequent starts
// with an auto-unseal seal it returns (nil, nil). For ShamirSeal it returns
// (nil, ErrNeedsUnseal) and waits for operators to call UnsealShard.
func (c *Core) Start(ctx context.Context) (*StartResult, error) {
	inited, err := c.barrier.Initialized(ctx)
	if err != nil {
		return nil, fmt.Errorf("check initialized: %w", err)
	}

	if !inited {
		result, err := c.seal.Init()
		if err != nil {
			return nil, fmt.Errorf("seal init: %w", err)
		}

		if err := c.barrier.Initialize(ctx, result.RootKey); err != nil {
			return nil, fmt.Errorf("barrier init: %w", err)
		}
		if err := c.barrier.Unseal(ctx, result.RootKey); err != nil {
			return nil, fmt.Errorf("barrier unseal: %w", err)
		}
		rootTok, err := token.Generate(token.RootTokenDisplayName, []string{token.RootPolicyName}, 0)
		if err != nil {
			return nil, fmt.Errorf("generate root token: %w", err)
		}
		if err := c.tokens.Put(ctx, rootTok); err != nil {
			return nil, fmt.Errorf("store root token: %w", err)
		}
		return &StartResult{RootToken: rootTok, Shares: result.Shares}, nil
	}

	// Already initialized — unseal.
	rootKey, err := c.seal.Unseal()
	if err != nil {
		if errors.Is(err, seal.ErrNeedsShards) {
			// Interactive seal: store context for later barrier.Unseal call.
			c.unsealCtx = ctx
			return nil, ErrNeedsUnseal
		}
		return nil, fmt.Errorf("seal unseal: %w", err)
	}
	if err := c.barrier.Unseal(ctx, rootKey); err != nil {
		return nil, fmt.Errorf("barrier unseal: %w", err)
	}
	return nil, nil
}

// UnsealShard accepts one base64url-encoded Shamir shard. When enough shards
// have been collected the barrier is unsealed automatically. Returns true when
// the barrier is now open.
//
// Returns an error if the active seal does not implement SharableUnseal, if
// the shard is malformed, or if a duplicate shard is provided.
func (c *Core) UnsealShard(ctx context.Context, share string) (bool, error) {
	su, ok := c.seal.(seal.SharableUnseal)
	if !ok {
		return false, ErrSealNotInteractive
	}

	c.unsealMu.Lock()
	defer c.unsealMu.Unlock()

	complete, rootKey, err := su.AcceptShard(share)
	if err != nil {
		return false, err
	}
	if !complete {
		return false, nil
	}

	// Use the stored context if the caller didn't provide a live one.
	unsealCtx := ctx
	if unsealCtx == nil && c.unsealCtx != nil {
		unsealCtx = c.unsealCtx
	}
	if unsealCtx == nil {
		unsealCtx = context.Background()
	}

	if err := c.barrier.Unseal(unsealCtx, rootKey); err != nil {
		return false, fmt.Errorf("barrier unseal after shards: %w", err)
	}
	c.unsealCtx = nil // release the stored context
	return true, nil
}

// SealStatus returns a snapshot of the current seal/unseal state.
func (c *Core) SealStatus() SealStatusInfo {
	info := SealStatusInfo{
		Sealed: c.barrier.IsSealed(),
		Type:   c.seal.Type(),
	}
	if su, ok := c.seal.(seal.SharableUnseal); ok {
		info.Required, info.Received = su.ShardsProgress()
	}
	return info
}

// Sealed reports whether the core is currently sealed.
func (c *Core) Sealed() bool { return c.barrier.IsSealed() }

// Seal re-seals the core, dropping the in-memory key.
func (c *Core) Seal() { c.barrier.Seal() }

// --- Identity ---

// Authenticate looks up tokenID and validates it. Returns ErrTokenInvalid on
// any failure so callers cannot distinguish missing from expired.
// ErrSealed is passed through as-is so HTTP callers can return 503.
func (c *Core) Authenticate(ctx context.Context, tokenID string) (*token.Token, error) {
	tok, err := c.tokens.Get(ctx, tokenID)
	if err != nil {
		if errors.Is(err, barrier.ErrSealed) {
			return nil, err
		}
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

// --- Kubernetes auth ---

// LoginK8s validates a Kubernetes ServiceAccount JWT via the configured
// Reviewer, looks up the bound role, and returns a short-lived Tuck token.
func (c *Core) LoginK8s(ctx context.Context, saToken string) (*token.Token, error) {
	if c.k8sReviewer == nil {
		return nil, ErrK8sAuthDisabled
	}
	result, err := c.k8sReviewer.Review(saToken)
	if err != nil {
		return nil, fmt.Errorf("k8s token review: %w", err)
	}
	if !result.Authenticated {
		return nil, ErrTokenInvalid
	}
	namespace, sa, err := k8sauth.ParseUsername(result.Username)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	role, err := c.k8sRoles.Get(ctx, namespace, sa)
	if err != nil {
		if errors.Is(err, k8sauth.ErrRoleNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("get k8s role: %w", err)
	}
	return c.CreateToken(ctx, "k8s:"+namespace+"/"+sa, role.Policies, role.TTL)
}

// CreateK8sRole stores a role binding for a Kubernetes ServiceAccount.
func (c *Core) CreateK8sRole(ctx context.Context, role *k8sauth.K8sRole) error {
	return c.k8sRoles.Put(ctx, role)
}

// GetK8sRole retrieves the role bound to a Kubernetes ServiceAccount.
func (c *Core) GetK8sRole(ctx context.Context, namespace, sa string) (*k8sauth.K8sRole, error) {
	return c.k8sRoles.Get(ctx, namespace, sa)
}

// DeleteK8sRole removes the role binding for a Kubernetes ServiceAccount.
func (c *Core) DeleteK8sRole(ctx context.Context, namespace, sa string) error {
	return c.k8sRoles.Delete(ctx, namespace, sa)
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

