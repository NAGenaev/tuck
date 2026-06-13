// Package core wires together the seal, the cryptographic barrier, and the
// logical secret operations. It is the top-level object the server talks to.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/NAGenaev/tuck/internal/auth/approle"
	"github.com/NAGenaev/tuck/internal/auth/jwt"
	authlda "github.com/NAGenaev/tuck/internal/auth/ldap"
	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/cubbyhole"
	dynaws "github.com/NAGenaev/tuck/internal/dynamic/aws"
	dynazure "github.com/NAGenaev/tuck/internal/dynamic/azure"
	"github.com/NAGenaev/tuck/internal/dynamic/database"
	"github.com/NAGenaev/tuck/internal/dynamic/gcp"
	"github.com/NAGenaev/tuck/internal/dynamic/pki"
	dynSSH "github.com/NAGenaev/tuck/internal/dynamic/ssh"
	dynTOTP "github.com/NAGenaev/tuck/internal/dynamic/totp"
	"github.com/NAGenaev/tuck/internal/dynamic/transit"
	"github.com/NAGenaev/tuck/internal/identity"
	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/kvv2"
	"github.com/NAGenaev/tuck/internal/namespace"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/policy"
	"github.com/NAGenaev/tuck/internal/seal"
	"github.com/NAGenaev/tuck/internal/token"
	"github.com/NAGenaev/tuck/internal/wrapping"
)

const secretPrefix = "secret/"

var (
	ErrUnauthorized    = errors.New("permission denied")
	ErrTokenInvalid    = errors.New("invalid or expired token")
	ErrNotRenewable    = errors.New("token is not renewable")
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

// ClusterBackender is the optional interface exposed by Raft-backed physical
// backends. Core surfaces it to the API layer through ClusterBackend().
type ClusterBackender interface {
	IsLeader() bool
	LeaderAddr() string
	AddVoter(id, addr string) error
	RemoveServer(id string) error
	ClusterStatus() any
}

// Core is Tuck's runtime: storage + crypto + seal, plus the logical KV and identity APIs.
type Core struct {
	backend    physical.Backend
	barrier    *barrier.Barrier
	seal       seal.Seal
	tokens     *token.Store
	policies   *policy.Store
	kv2        *kvv2.Store
	namespaces *namespace.Store
	jwtStore     *jwt.Store
	approleStore *approle.Store
	ldapStore    *authlda.Store
	cubbyhole    *cubbyhole.Store
	wrapping     *wrapping.Store
	dbManager    *database.Manager
	awsEngine    *dynaws.Engine
	gcpEngine    *gcp.Engine
	azureEngine  *dynazure.Engine
	pkiManager     *pki.Manager
	transitManager *transit.Manager
	sshManager     *dynSSH.Manager
	totpManager    *dynTOTP.Manager
	identity *identity.Store

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
		kv2:         kvv2.New(b),
		namespaces:  namespace.NewStore(b),
		jwtStore:     jwt.NewStore(b),
		approleStore: approle.NewStore(b),
		ldapStore:    authlda.NewStore(b),
		cubbyhole:    cubbyhole.NewStore(b),
		wrapping:     wrapping.NewStore(b),
		dbManager:    database.NewManager(b),
		awsEngine:    dynaws.New(b),
		gcpEngine:    gcp.New(b),
		azureEngine:  dynazure.New(b),
		pkiManager:     pki.NewManager(b),
		transitManager: transit.NewManager(b),
		sshManager:     dynSSH.NewManager(b),
		totpManager:    dynTOTP.NewManager(b),
		identity:     identity.NewStore(b),
		k8sReviewer:  reviewer,
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
		clear(result.RootKey)
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
	clear(rootKey)
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
	clear(rootKey)
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
// Authenticate does NOT count uses — call TrackUse once per HTTP request.
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

// TrackUse increments the use counter for tokens with MaxUses > 0.
// Call this exactly once per HTTP request, after Authenticate succeeds.
// Returns ErrTokenInvalid (and auto-revokes) when the token is exhausted.
func (c *Core) TrackUse(ctx context.Context, tokenID string) error {
	tok, err := c.tokens.Get(ctx, tokenID)
	if err != nil {
		return ErrTokenInvalid
	}
	if tok.MaxUses == 0 {
		return nil // unlimited
	}
	tok.UseCount++
	if tok.UseCount > tok.MaxUses {
		_ = c.tokens.Delete(ctx, tok.ID)
		return ErrTokenInvalid
	}
	_ = c.tokens.Put(ctx, tok)
	return nil
}

// EnforceAccess authenticates tokenID then checks that cap is permitted on
// path. path must be the full logical path including the mount prefix
// (e.g. "secret/db/password").
func (c *Core) EnforceAccess(ctx context.Context, tokenID, logicalPath string, cap policy.Capability) error {
	tok, err := c.Authenticate(ctx, tokenID)
	if err != nil {
		return err
	}
	policies, err := c.resolvePolicies(ctx, tok.Namespace, tok.Policies)
	if err != nil {
		return fmt.Errorf("resolve policies: %w", err)
	}
	if !policy.Allowed(policies, logicalPath, cap) {
		return ErrUnauthorized
	}
	return nil
}

// resolvePolicies loads named policies from ns. "root" is never stored — it is
// injected directly so it cannot be accidentally deleted via the API.
func (c *Core) resolvePolicies(ctx context.Context, ns string, names []string) ([]policy.Policy, error) {
	store := c.Policies(ns)
	out := make([]policy.Policy, 0, len(names))
	for _, name := range names {
		if name == token.RootPolicyName {
			out = append(out, policy.RootPolicy)
			continue
		}
		p, err := store.Get(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("policy %q: %w", name, err)
		}
		out = append(out, *p)
	}
	return out, nil
}

// --- Token management ---

// TokenOpt is a functional option applied to a token at creation time.
type TokenOpt func(*token.Token)

// WithRenewable marks the token as renewable. maxTTL, if > 0, caps the
// maximum lifetime: the token cannot be renewed past CreatedAt + maxTTL.
func WithRenewable(maxTTL time.Duration) TokenOpt {
	return func(t *token.Token) {
		t.Renewable = true
		t.MaxTTL = maxTTL
	}
}

// WithMaxUses limits the token to n authenticated API calls. After n uses the
// token is automatically revoked. n=0 (default) means unlimited uses.
func WithMaxUses(n int) TokenOpt {
	return func(t *token.Token) {
		t.MaxUses = n
	}
}

func (c *Core) CreateToken(ctx context.Context, displayName string, policies []string, ttl time.Duration, opts ...TokenOpt) (*token.Token, error) {
	tok, err := token.Generate(displayName, policies, ttl)
	if err != nil {
		return nil, err
	}
	for _, o := range opts {
		o(tok)
	}
	return tok, c.tokens.Put(ctx, tok)
}

func (c *Core) RevokeToken(ctx context.Context, tokenID string) error {
	_ = c.cubbyhole.PurgeToken(ctx, tokenID)
	return c.tokens.Delete(ctx, tokenID)
}

func (c *Core) LookupToken(ctx context.Context, tokenID string) (*token.Token, error) {
	return c.tokens.Get(ctx, tokenID)
}

// LookupSelf returns the token record for the authenticated caller.
func (c *Core) LookupSelf(ctx context.Context, tokenID string) (*token.Token, error) {
	tok, err := c.tokens.Get(ctx, tokenID)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	if tok.IsExpired() {
		return nil, ErrTokenInvalid
	}
	return tok, nil
}

// RenewSelf renews the authenticated caller's own token.
func (c *Core) RenewSelf(ctx context.Context, tokenID string, ttl time.Duration) (*token.Token, error) {
	return c.RenewToken(ctx, tokenID, ttl)
}

// LookupTokenByAccessor returns token metadata for the given accessor.
func (c *Core) LookupTokenByAccessor(ctx context.Context, accessor string) (*token.Token, error) {
	return c.tokens.GetByAccessor(ctx, accessor)
}

// RevokeTokenByAccessor revokes the token identified by accessor.
func (c *Core) RevokeTokenByAccessor(ctx context.Context, accessor string) error {
	// First find the token so we can purge its cubbyhole.
	tok, err := c.tokens.GetByAccessor(ctx, accessor)
	if err == nil && tok != nil {
		_ = c.cubbyhole.PurgeToken(ctx, tok.ID)
	}
	return c.tokens.DeleteByAccessor(ctx, accessor)
}

// ListTokens returns all token IDs in the store.
func (c *Core) ListTokens(ctx context.Context) ([]string, error) {
	return c.tokens.List(ctx)
}

// RenewToken extends tokenID's expiry by ttl (default 1h when ttl≤0).
// Returns ErrTokenInvalid if the token is expired, ErrNotRenewable if the
// token was not created with WithRenewable. MaxTTL, when set, caps the new
// ExpiresAt to CreatedAt + MaxTTL.
func (c *Core) RenewToken(ctx context.Context, tokenID string, ttl time.Duration) (*token.Token, error) {
	tok, err := c.tokens.Get(ctx, tokenID)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	if tok.IsExpired() {
		return nil, ErrTokenInvalid
	}
	if !tok.Renewable {
		return nil, ErrNotRenewable
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	newExpiry := time.Now().UTC().Add(ttl)
	if tok.MaxTTL > 0 {
		cap := tok.CreatedAt.Add(tok.MaxTTL)
		if newExpiry.After(cap) {
			newExpiry = cap
		}
	}
	tok.ExpiresAt = newExpiry
	if err := c.tokens.Put(ctx, tok); err != nil {
		return nil, err
	}
	return tok, nil
}

// --- Policy management ---

func (c *Core) PutPolicy(ctx context.Context, ns string, p *policy.Policy) error {
	return c.Policies(ns).Put(ctx, p)
}

func (c *Core) GetPolicy(ctx context.Context, ns, name string) (*policy.Policy, error) {
	return c.Policies(ns).Get(ctx, name)
}

func (c *Core) DeletePolicy(ctx context.Context, ns, name string) error {
	return c.Policies(ns).Delete(ctx, name)
}

// ListPolicies returns all policy names in the given namespace.
func (c *Core) ListPolicies(ctx context.Context, ns string) ([]string, error) {
	return c.Policies(ns).List(ctx)
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
	tok, err := c.CreateToken(ctx, "k8s:"+namespace+"/"+sa, role.Policies, role.TTL)
	if err != nil {
		return nil, err
	}
	c.attachEntityToToken(ctx, tok, "auth_kubernetes", namespace+"/"+sa, nil)
	return tok, nil
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

// ListSecrets returns all secret keys whose storage key starts with the given
// prefix path. An empty prefix lists every secret.
// ns is the active namespace name (empty or "root" means root namespace).
func (c *Core) ListSecrets(ctx context.Context, ns, prefix string) ([]string, error) {
	nsPrefix := namespace.StoragePrefix(ns)
	storagePrefix := nsPrefix + secretPrefix
	if prefix != "" {
		p := nsPrefix + secretKey(prefix)
		if !strings.HasSuffix(p, "/") {
			p += "/"
		}
		storagePrefix = p
	}
	keys, err := c.barrier.List(ctx, storagePrefix)
	if err != nil {
		return nil, err
	}
	trim := nsPrefix + secretPrefix
	result := make([]string, len(keys))
	for i, k := range keys {
		result[i] = strings.TrimPrefix(k, trim)
	}
	return result, nil
}

// GetSecret returns the bytes stored at the logical path, or (nil, false) if
// nothing is stored there.
// ns is the active namespace name (empty or "root" means root namespace).
func (c *Core) GetSecret(ctx context.Context, ns, p string) ([]byte, bool, error) {
	key := namespace.StoragePrefix(ns) + secretKey(p)
	e, err := c.barrier.Get(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if e == nil {
		return nil, false, nil
	}
	return e.Value, true, nil
}

// PutSecret stores bytes at the logical path.
// ns is the active namespace name (empty or "root" means root namespace).
func (c *Core) PutSecret(ctx context.Context, ns, p string, value []byte) error {
	key := namespace.StoragePrefix(ns) + secretKey(p)
	return c.barrier.Put(ctx, &physical.Entry{Key: key, Value: value})
}

// DeleteSecret removes the secret at the logical path.
// ns is the active namespace name (empty or "root" means root namespace).
func (c *Core) DeleteSecret(ctx context.Context, ns, p string) error {
	return c.barrier.Delete(ctx, namespace.StoragePrefix(ns)+secretKey(p))
}

// secretKey maps a logical path to a backend key, cleaning it so a request
// cannot escape the secret/ namespace via "..".
func secretKey(p string) string {
	return secretPrefix + path.Clean("/"+p)[1:]
}

// KVv2 returns the versioned KV store for the given namespace.
// Root namespace returns the default store; named namespaces get a prefix-scoped view.
func (c *Core) KVv2(ns string) *kvv2.Store {
	if prefix := namespace.StoragePrefix(ns); prefix != "" {
		return kvv2.New(c.barrier.View(prefix))
	}
	return c.kv2
}

// Policies returns the policy store for the given namespace.
func (c *Core) Policies(ns string) *policy.Store {
	if prefix := namespace.StoragePrefix(ns); prefix != "" {
		return policy.NewStore(c.barrier.View(prefix))
	}
	return c.policies
}

// ClusterBackend returns the cluster interface if the physical backend
// implements it (i.e. Raft mode), otherwise nil.
func (c *Core) ClusterBackend() ClusterBackender {
	if cb, ok := c.backend.(ClusterBackender); ok {
		return cb
	}
	return nil
}

// RotateKey generates a new root key via the seal and re-wraps the barrier DEK.
// No data re-encryption is needed — only the keyring envelope changes.
// For ShamirSeal the new shares are returned; for other seals the slice is nil.
func (c *Core) RotateKey(ctx context.Context) ([]string, error) {
	result, err := c.seal.Init()
	if err != nil {
		return nil, fmt.Errorf("seal re-init: %w", err)
	}
	if err := c.barrier.Rekey(ctx, result.RootKey); err != nil {
		clear(result.RootKey)
		return nil, fmt.Errorf("rekey barrier: %w", err)
	}
	clear(result.RootKey)
	return result.Shares, nil
}

// Snapshotter returns a function that writes a database snapshot to an io.Writer,
// and true if the backend supports snapshots. Returns (nil, false) otherwise.
func (c *Core) Snapshotter() (func(ctx context.Context, w io.Writer) error, bool) {
	type snapBackend interface {
		Snapshot(ctx context.Context, w io.Writer) error
	}
	if sb, ok := c.backend.(snapBackend); ok {
		return sb.Snapshot, true
	}
	return nil, false
}

// StartGC launches a background goroutine that periodically removes expired
// tokens. It returns when ctx is cancelled.
func (c *Core) StartGC(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !c.barrier.IsSealed() {
					c.runGC(ctx)
				}
			}
		}
	}()
}

// --- JWT/OIDC auth ---

// ConfigureJWT stores the JWT auth configuration.
func (c *Core) ConfigureJWT(ctx context.Context, cfg *jwt.Config) error {
	return c.jwtStore.PutConfig(ctx, cfg)
}

// GetJWTConfig returns the current JWT auth configuration.
func (c *Core) GetJWTConfig(ctx context.Context) (*jwt.Config, error) {
	return c.jwtStore.GetConfig(ctx)
}

// PutJWTRole creates or updates a JWT auth role.
func (c *Core) PutJWTRole(ctx context.Context, role *jwt.Role) error {
	return c.jwtStore.PutRole(ctx, role)
}

// GetJWTRole returns a JWT auth role by name.
func (c *Core) GetJWTRole(ctx context.Context, name string) (*jwt.Role, error) {
	return c.jwtStore.GetRole(ctx, name)
}

// DeleteJWTRole removes a JWT auth role.
func (c *Core) DeleteJWTRole(ctx context.Context, name string) error {
	return c.jwtStore.DeleteRole(ctx, name)
}

// ListJWTRoles returns all JWT role names.
func (c *Core) ListJWTRoles(ctx context.Context) ([]string, error) {
	return c.jwtStore.ListRoles(ctx)
}

// LoginJWT validates a JWT against configured roles and issues a Tuck token.
func (c *Core) LoginJWT(ctx context.Context, tokenStr string) (*token.Token, error) {
	cfg, err := c.jwtStore.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := c.jwtStore.AllRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("load jwt roles: %w", err)
	}
	provider := jwt.NewProvider(*cfg)
	result, err := provider.Login(ctx, tokenStr, roles)
	if err != nil {
		return nil, err
	}
	tok, err := c.CreateToken(ctx, "jwt:"+result.Subject, result.Policies, result.TTL)
	if err != nil {
		return nil, err
	}
	c.attachEntityToToken(ctx, tok, "auth_jwt", result.Subject, result.Groups)
	return tok, nil
}

// --- TOTP secrets engine ---

func (c *Core) TOTPCreateKey(ctx context.Context, name string, req dynTOTP.CreateKeyRequest) (*dynTOTP.CreateResult, error) {
	return c.totpManager.CreateKey(ctx, name, req)
}
func (c *Core) TOTPGetKey(ctx context.Context, name string) (*dynTOTP.KeyInfo, error) {
	return c.totpManager.GetKey(ctx, name)
}
func (c *Core) TOTPDeleteKey(ctx context.Context, name string) error {
	return c.totpManager.DeleteKey(ctx, name)
}
func (c *Core) TOTPListKeys(ctx context.Context) ([]string, error) {
	return c.totpManager.ListKeys(ctx)
}
func (c *Core) TOTPGenerateCode(ctx context.Context, name string) (*dynTOTP.GenerateResult, error) {
	return c.totpManager.GenerateCode(ctx, name)
}
func (c *Core) TOTPValidateCode(ctx context.Context, name, code string) (bool, error) {
	return c.totpManager.ValidateCode(ctx, name, code)
}

// --- SSH secrets engine ---

func (c *Core) SSHGenerateCA(ctx context.Context, keyType string) (string, error) {
	return c.sshManager.GenerateCA(ctx, keyType)
}
func (c *Core) SSHImportCA(ctx context.Context, privateKeyPEM string) error {
	return c.sshManager.ImportCA(ctx, privateKeyPEM)
}
func (c *Core) SSHGetCAPublicKey(ctx context.Context) (string, error) {
	return c.sshManager.GetCAPublicKey(ctx)
}
func (c *Core) SSHPutRole(ctx context.Context, r *dynSSH.Role) error {
	return c.sshManager.PutRole(ctx, r)
}
func (c *Core) SSHGetRole(ctx context.Context, name string) (*dynSSH.Role, error) {
	return c.sshManager.GetRole(ctx, name)
}
func (c *Core) SSHDeleteRole(ctx context.Context, name string) error {
	return c.sshManager.DeleteRole(ctx, name)
}
func (c *Core) SSHListRoles(ctx context.Context) ([]string, error) {
	return c.sshManager.ListRoles(ctx)
}
func (c *Core) SSHSignPublicKey(ctx context.Context, roleName, publicKeyStr string, validPrincipals []string, ttl time.Duration) (*dynSSH.SignedCert, error) {
	return c.sshManager.SignPublicKey(ctx, roleName, publicKeyStr, validPrincipals, ttl)
}

// --- Transit secrets engine ---

func (c *Core) TransitCreateKey(ctx context.Context, name, keyType string) error {
	return c.transitManager.CreateKey(ctx, name, keyType)
}
func (c *Core) TransitGetKey(ctx context.Context, name string) (*transit.Key, error) {
	return c.transitManager.GetKey(ctx, name)
}
func (c *Core) TransitDeleteKey(ctx context.Context, name string) error {
	return c.transitManager.DeleteKey(ctx, name)
}
func (c *Core) TransitListKeys(ctx context.Context) ([]string, error) {
	return c.transitManager.ListKeys(ctx)
}
func (c *Core) TransitRotate(ctx context.Context, name string) error {
	return c.transitManager.Rotate(ctx, name)
}
func (c *Core) TransitUpdateKey(ctx context.Context, name string, minVersion int, deletable bool) error {
	return c.transitManager.UpdateKey(ctx, name, minVersion, deletable)
}
func (c *Core) TransitEncrypt(ctx context.Context, name string, plaintext []byte) (string, error) {
	return c.transitManager.Encrypt(ctx, name, plaintext)
}
func (c *Core) TransitDecrypt(ctx context.Context, name string, ciphertext string) ([]byte, error) {
	return c.transitManager.Decrypt(ctx, name, ciphertext)
}
func (c *Core) TransitRewrap(ctx context.Context, name string, ciphertext string) (string, error) {
	return c.transitManager.Rewrap(ctx, name, ciphertext)
}
func (c *Core) TransitSign(ctx context.Context, name string, input []byte, hashAlg string) (string, error) {
	return c.transitManager.Sign(ctx, name, input, hashAlg)
}
func (c *Core) TransitVerify(ctx context.Context, name string, input []byte, sig string, hashAlg string) (bool, error) {
	return c.transitManager.Verify(ctx, name, input, sig, hashAlg)
}
func (c *Core) TransitHMAC(ctx context.Context, name string, input []byte, hashAlg string) (string, error) {
	return c.transitManager.HMAC(ctx, name, input, hashAlg)
}

// --- PKI secrets engine ---

func (c *Core) GeneratePKICA(ctx context.Context, cfg *pki.CAConfig) (string, error) {
	return c.pkiManager.GenerateCA(ctx, cfg)
}
func (c *Core) ImportPKICA(ctx context.Context, certPEM, keyPEM string) error {
	return c.pkiManager.ImportCA(ctx, certPEM, keyPEM)
}
func (c *Core) GetPKICACert(ctx context.Context) (string, error) {
	return c.pkiManager.GetCACert(ctx)
}
func (c *Core) GetPKICRL(ctx context.Context) (string, error) {
	return c.pkiManager.GetCRL(ctx)
}
func (c *Core) PutPKIRole(ctx context.Context, r *pki.Role) error {
	return c.pkiManager.PutRole(ctx, r)
}
func (c *Core) GetPKIRole(ctx context.Context, name string) (*pki.Role, error) {
	return c.pkiManager.GetRole(ctx, name)
}
func (c *Core) DeletePKIRole(ctx context.Context, name string) error {
	return c.pkiManager.DeleteRole(ctx, name)
}
func (c *Core) ListPKIRoles(ctx context.Context) ([]string, error) {
	return c.pkiManager.ListRoles(ctx)
}
func (c *Core) IssuePKICert(ctx context.Context, roleName, commonName string, altNames []string, ttl time.Duration) (*pki.IssuedCert, error) {
	return c.pkiManager.IssueCert(ctx, roleName, commonName, altNames, ttl)
}
func (c *Core) RevokePKICert(ctx context.Context, serial string) error {
	return c.pkiManager.RevokeCert(ctx, serial)
}
func (c *Core) GetPKICert(ctx context.Context, serial string) (*pki.CertRecord, error) {
	return c.pkiManager.GetCert(ctx, serial)
}
func (c *Core) ListPKICerts(ctx context.Context) ([]string, error) {
	return c.pkiManager.ListCerts(ctx)
}

// --- AppRole auth ---

func (c *Core) PutAppRole(ctx context.Context, r *approle.Role) error {
	return c.approleStore.PutRole(ctx, r)
}
func (c *Core) GetAppRole(ctx context.Context, name string) (*approle.Role, error) {
	return c.approleStore.GetRole(ctx, name)
}
func (c *Core) DeleteAppRole(ctx context.Context, name string) error {
	return c.approleStore.DeleteRole(ctx, name)
}
func (c *Core) ListAppRoles(ctx context.Context) ([]string, error) {
	return c.approleStore.ListRoles(ctx)
}
func (c *Core) GenerateSecretID(ctx context.Context, roleName string) (*approle.SecretID, error) {
	return c.approleStore.GenerateSecretID(ctx, roleName)
}
func (c *Core) LookupSecretID(ctx context.Context, id string) (*approle.SecretID, error) {
	return c.approleStore.LookupSecretID(ctx, id)
}
func (c *Core) DestroySecretID(ctx context.Context, id string) error {
	return c.approleStore.DestroySecretID(ctx, id)
}
func (c *Core) LoginAppRole(ctx context.Context, roleID, secretID string) (*token.Token, error) {
	result, err := c.approleStore.Login(ctx, roleID, secretID)
	if err != nil {
		return nil, err
	}
	tok, err := c.CreateToken(ctx, result.Subject, result.Policies, result.TTL)
	if err != nil {
		return nil, err
	}
	c.attachEntityToToken(ctx, tok, "auth_approle", result.Subject, nil)
	return tok, nil
}

// --- Dynamic secrets: Database engine ---

func (c *Core) PutDBConfig(ctx context.Context, cfg *database.Config) error {
	return c.dbManager.PutConfig(ctx, cfg)
}
func (c *Core) GetDBConfig(ctx context.Context, name string) (*database.Config, error) {
	return c.dbManager.GetConfig(ctx, name)
}
func (c *Core) DeleteDBConfig(ctx context.Context, name string) error {
	return c.dbManager.DeleteConfig(ctx, name)
}
func (c *Core) ListDBConfigs(ctx context.Context) ([]string, error) {
	return c.dbManager.ListConfigs(ctx)
}
func (c *Core) PutDBRole(ctx context.Context, r *database.Role) error {
	return c.dbManager.PutRole(ctx, r)
}
func (c *Core) GetDBRole(ctx context.Context, name string) (*database.Role, error) {
	return c.dbManager.GetRole(ctx, name)
}
func (c *Core) DeleteDBRole(ctx context.Context, name string) error {
	return c.dbManager.DeleteRole(ctx, name)
}
func (c *Core) ListDBRoles(ctx context.Context) ([]string, error) {
	return c.dbManager.ListRoles(ctx)
}
func (c *Core) GenerateDBCreds(ctx context.Context, roleName string) (*database.Credentials, error) {
	return c.dbManager.GenerateCreds(ctx, roleName)
}
func (c *Core) RevokeDBLease(ctx context.Context, leaseID string) error {
	return c.dbManager.RevokeLease(ctx, leaseID)
}
func (c *Core) GetDBLease(ctx context.Context, leaseID string) (*database.Lease, error) {
	return c.dbManager.GetLease(ctx, leaseID)
}
func (c *Core) ListDBLeases(ctx context.Context) ([]string, error) {
	return c.dbManager.ListLeases(ctx)
}

func (c *Core) runGC(ctx context.Context) {
	expired, err := c.tokens.ListExpired(ctx)
	if err != nil {
		return
	}
	for _, id := range expired {
		_ = c.tokens.Delete(ctx, id)
		_ = c.cubbyhole.PurgeToken(ctx, id)
	}
	_ = c.dbManager.RevokeExpired(ctx)
	_ = c.awsEngine.RevokeExpired(ctx)
	_ = c.gcpEngine.RevokeExpired(ctx)
	_ = c.azureEngine.RevokeExpired(ctx)
	_ = c.wrapping.RevokeExpired(ctx)
}

// --- GCP dynamic secrets engine ---

func (c *Core) PutGCPConfig(ctx context.Context, cfg *gcp.Config) error {
	return c.gcpEngine.PutConfig(ctx, cfg)
}

func (c *Core) GetGCPConfig(ctx context.Context) (*gcp.Config, error) {
	cfg, err := c.gcpEngine.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	cfg.CredentialsJSON = "" // never expose credentials through the API
	return cfg, nil
}

func (c *Core) DeleteGCPConfig(ctx context.Context) error {
	return c.gcpEngine.DeleteConfig(ctx)
}

func (c *Core) PutGCPRole(ctx context.Context, role *gcp.Role) error {
	return c.gcpEngine.PutRole(ctx, role)
}

func (c *Core) GetGCPRole(ctx context.Context, name string) (*gcp.Role, error) {
	return c.gcpEngine.GetRole(ctx, name)
}

func (c *Core) DeleteGCPRole(ctx context.Context, name string) error {
	return c.gcpEngine.DeleteRole(ctx, name)
}

func (c *Core) ListGCPRoles(ctx context.Context) ([]string, error) {
	return c.gcpEngine.ListRoles(ctx)
}

func (c *Core) GenerateGCPCreds(ctx context.Context, roleName string) (*gcp.GenerateResult, error) {
	return c.gcpEngine.GenerateCreds(ctx, roleName)
}

func (c *Core) GetGCPLease(ctx context.Context, id string) (*gcp.Lease, error) {
	return c.gcpEngine.GetLease(ctx, id)
}

func (c *Core) RevokeGCPLease(ctx context.Context, id string) error {
	return c.gcpEngine.RevokeLease(ctx, id)
}

func (c *Core) ListGCPLeases(ctx context.Context) ([]string, error) {
	return c.gcpEngine.ListLeases(ctx)
}

// --- Azure dynamic secrets engine ---

func (c *Core) PutAzureConfig(ctx context.Context, cfg *dynazure.Config) error {
	return c.azureEngine.PutConfig(ctx, cfg)
}

func (c *Core) GetAzureConfig(ctx context.Context) (*dynazure.Config, error) {
	cfg, err := c.azureEngine.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	cfg.ClientSecret = "" // never expose credentials through the API
	return cfg, nil
}

func (c *Core) DeleteAzureConfig(ctx context.Context) error {
	return c.azureEngine.DeleteConfig(ctx)
}

func (c *Core) PutAzureRole(ctx context.Context, role *dynazure.Role) error {
	return c.azureEngine.PutRole(ctx, role)
}

func (c *Core) GetAzureRole(ctx context.Context, name string) (*dynazure.Role, error) {
	return c.azureEngine.GetRole(ctx, name)
}

func (c *Core) DeleteAzureRole(ctx context.Context, name string) error {
	return c.azureEngine.DeleteRole(ctx, name)
}

func (c *Core) ListAzureRoles(ctx context.Context) ([]string, error) {
	return c.azureEngine.ListRoles(ctx)
}

func (c *Core) GenerateAzureCreds(ctx context.Context, roleName string) (*dynazure.GenerateResult, error) {
	return c.azureEngine.GenerateCreds(ctx, roleName)
}

func (c *Core) GetAzureLease(ctx context.Context, id string) (*dynazure.Lease, error) {
	return c.azureEngine.GetLease(ctx, id)
}

func (c *Core) RevokeAzureLease(ctx context.Context, id string) error {
	return c.azureEngine.RevokeLease(ctx, id)
}

func (c *Core) ListAzureLeases(ctx context.Context) ([]string, error) {
	return c.azureEngine.ListLeases(ctx)
}

// --- AWS dynamic secrets engine ---

func (c *Core) PutAWSConfig(ctx context.Context, cfg *dynaws.Config) error {
	return c.awsEngine.PutConfig(ctx, cfg)
}

func (c *Core) GetAWSConfig(ctx context.Context) (*dynaws.Config, error) {
	cfg, err := c.awsEngine.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	cfg.SecretAccessKey = "" // never expose credentials through the API
	return cfg, nil
}

func (c *Core) DeleteAWSConfig(ctx context.Context) error {
	return c.awsEngine.DeleteConfig(ctx)
}

func (c *Core) PutAWSRole(ctx context.Context, role *dynaws.Role) error {
	return c.awsEngine.PutRole(ctx, role)
}

func (c *Core) GetAWSRole(ctx context.Context, name string) (*dynaws.Role, error) {
	return c.awsEngine.GetRole(ctx, name)
}

func (c *Core) DeleteAWSRole(ctx context.Context, name string) error {
	return c.awsEngine.DeleteRole(ctx, name)
}

func (c *Core) ListAWSRoles(ctx context.Context) ([]string, error) {
	return c.awsEngine.ListRoles(ctx)
}

func (c *Core) GenerateAWSCreds(ctx context.Context, roleName string) (*dynaws.GenerateResult, error) {
	return c.awsEngine.GenerateCreds(ctx, roleName)
}

func (c *Core) GetAWSLease(ctx context.Context, id string) (*dynaws.Lease, error) {
	return c.awsEngine.GetLease(ctx, id)
}

func (c *Core) RevokeAWSLease(ctx context.Context, id string) error {
	return c.awsEngine.RevokeLease(ctx, id)
}

func (c *Core) ListAWSLeases(ctx context.Context) ([]string, error) {
	return c.awsEngine.ListLeases(ctx)
}

// --- LDAP auth ---

// GetLDAPConfig returns the current LDAP auth configuration (BindPassword redacted).
func (c *Core) GetLDAPConfig(ctx context.Context) (*authlda.Config, error) {
	cfg, err := c.ldapStore.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	// Never expose the bind password through the API.
	cfg.BindPassword = ""
	return cfg, nil
}

// PutLDAPConfig stores the LDAP auth configuration.
func (c *Core) PutLDAPConfig(ctx context.Context, cfg *authlda.Config) error {
	return c.ldapStore.PutConfig(ctx, cfg)
}

// PutLDAPRole creates or replaces an LDAP role.
func (c *Core) PutLDAPRole(ctx context.Context, r *authlda.Role) error {
	return c.ldapStore.PutRole(ctx, r)
}

// GetLDAPRole returns a role by name.
func (c *Core) GetLDAPRole(ctx context.Context, name string) (*authlda.Role, error) {
	return c.ldapStore.GetRole(ctx, name)
}

// DeleteLDAPRole removes a role.
func (c *Core) DeleteLDAPRole(ctx context.Context, name string) error {
	return c.ldapStore.DeleteRole(ctx, name)
}

// ListLDAPRoles returns all role names.
func (c *Core) ListLDAPRoles(ctx context.Context) ([]string, error) {
	return c.ldapStore.ListRoles(ctx)
}

// LoginLDAP validates LDAP credentials and returns a Tuck token.
func (c *Core) LoginLDAP(ctx context.Context, username, password string) (*token.Token, error) {
	cfg, err := c.ldapStore.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := c.ldapStore.AllRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("load ldap roles: %w", err)
	}
	auth := authlda.NewAuthenticator(*cfg)
	result, err := auth.Login(ctx, roles, username, password)
	if err != nil {
		return nil, err
	}
	tok, err := c.CreateToken(ctx, "ldap:"+result.Username, result.Policies, result.TTL)
	if err != nil {
		return nil, err
	}
	c.attachEntityToToken(ctx, tok, "auth_ldap", result.Username, result.Groups)
	return tok, nil
}


// --- Response wrapping ---

func (c *Core) WrapPayload(ctx context.Context, payload json.RawMessage, ttl time.Duration) (string, time.Time, error) {
	return c.wrapping.Wrap(ctx, payload, ttl)
}

func (c *Core) UnwrapPayload(ctx context.Context, token string) (json.RawMessage, error) {
	return c.wrapping.Unwrap(ctx, token)
}

func (c *Core) LookupWrappingToken(ctx context.Context, token string) (*wrapping.TokenInfo, error) {
	return c.wrapping.Lookup(ctx, token)
}

func (c *Core) RevokeWrappingToken(ctx context.Context, token string) error {
	return c.wrapping.Revoke(ctx, token)
}

// --- Entity & Identity ---

// attachEntityToToken auto-creates (or looks up) the entity alias for the
// given auth-method mount and alias name, merges entity+group policies into
// tok.Policies, and persists the updated token. Errors are silently swallowed
// — identity enrichment is best-effort and must not break auth.
func (c *Core) attachEntityToToken(ctx context.Context, tok *token.Token, mount, aliasName string, externalGroups []string) {
	entity, err := c.identity.EnsureAlias(ctx, mount, aliasName)
	if err != nil {
		return
	}
	tok.EntityID = entity.ID
	extra := c.identity.ResolveEntityPolicies(ctx, entity.ID)
	extGroupPolicies := c.identity.ResolveExternalGroupPolicies(ctx, mount, externalGroups)
	tok.Policies = mergePolicies(tok.Policies, extra, extGroupPolicies)
	_ = c.tokens.Put(ctx, tok)
}

func mergePolicies(slices ...[]string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range slices {
		for _, p := range s {
			if !seen[p] {
				seen[p] = true
				result = append(result, p)
			}
		}
	}
	return result
}

func (c *Core) IdentityCreateEntity(ctx context.Context, name string, policies []string, meta map[string]string) (*identity.Entity, error) {
	return c.identity.CreateEntity(ctx, name, policies, meta)
}
func (c *Core) IdentityPutEntity(ctx context.Context, e *identity.Entity) error {
	return c.identity.PutEntity(ctx, e)
}
func (c *Core) IdentityGetEntityByID(ctx context.Context, id string) (*identity.Entity, error) {
	return c.identity.GetEntityByID(ctx, id)
}
func (c *Core) IdentityGetEntityByName(ctx context.Context, name string) (*identity.Entity, error) {
	return c.identity.GetEntityByName(ctx, name)
}
func (c *Core) IdentityDeleteEntity(ctx context.Context, id string) error {
	return c.identity.DeleteEntity(ctx, id)
}
func (c *Core) IdentityListEntityIDs(ctx context.Context) ([]string, error) {
	return c.identity.ListEntityIDs(ctx)
}

func (c *Core) IdentityCreateAlias(ctx context.Context, entityID, mount, name string, meta map[string]string) (*identity.EntityAlias, error) {
	return c.identity.CreateAlias(ctx, entityID, mount, name, meta)
}
func (c *Core) IdentityPutAlias(ctx context.Context, a *identity.EntityAlias) error {
	return c.identity.PutAlias(ctx, a)
}
func (c *Core) IdentityGetAliasByID(ctx context.Context, id string) (*identity.EntityAlias, error) {
	return c.identity.GetAliasByID(ctx, id)
}
func (c *Core) IdentityGetAliasByMount(ctx context.Context, mount, name string) (*identity.EntityAlias, error) {
	return c.identity.GetAliasByMount(ctx, mount, name)
}
func (c *Core) IdentityDeleteAlias(ctx context.Context, id string) error {
	return c.identity.DeleteAlias(ctx, id)
}
func (c *Core) IdentityListAliasIDs(ctx context.Context) ([]string, error) {
	return c.identity.ListAliasIDs(ctx)
}

func (c *Core) IdentityCreateGroup(ctx context.Context, name string, policies []string, memberEntityIDs, memberGroupIDs []string, meta map[string]string) (*identity.Group, error) {
	return c.identity.CreateGroup(ctx, name, policies, memberEntityIDs, memberGroupIDs, meta)
}
func (c *Core) IdentityPutGroup(ctx context.Context, g *identity.Group) error {
	return c.identity.PutGroup(ctx, g)
}
func (c *Core) IdentityGetGroupByID(ctx context.Context, id string) (*identity.Group, error) {
	return c.identity.GetGroupByID(ctx, id)
}
func (c *Core) IdentityGetGroupByName(ctx context.Context, name string) (*identity.Group, error) {
	return c.identity.GetGroupByName(ctx, name)
}
func (c *Core) IdentityDeleteGroup(ctx context.Context, id string) error {
	return c.identity.DeleteGroup(ctx, id)
}
func (c *Core) IdentityListGroupIDs(ctx context.Context) ([]string, error) {
	return c.identity.ListGroupIDs(ctx)
}

func (c *Core) IdentityCreateGroupAlias(ctx context.Context, groupID, mount, name string, meta map[string]string) (*identity.GroupAlias, error) {
	return c.identity.CreateGroupAlias(ctx, groupID, mount, name, meta)
}
func (c *Core) IdentityPutGroupAlias(ctx context.Context, ga *identity.GroupAlias) error {
	return c.identity.PutGroupAlias(ctx, ga)
}
func (c *Core) IdentityGetGroupAliasByID(ctx context.Context, id string) (*identity.GroupAlias, error) {
	return c.identity.GetGroupAliasByID(ctx, id)
}
func (c *Core) IdentityDeleteGroupAlias(ctx context.Context, id string) error {
	return c.identity.DeleteGroupAlias(ctx, id)
}
func (c *Core) IdentityListGroupAliasIDs(ctx context.Context) ([]string, error) {
	return c.identity.ListGroupAliasIDs(ctx)
}

// --- Namespace management ---

// CreateNamespace creates a new namespace. Returns an error if the name is invalid
// or the namespace already exists.
func (c *Core) CreateNamespace(ctx context.Context, name string) (*namespace.Namespace, error) {
	if err := namespace.ValidateName(name); err != nil {
		return nil, err
	}
	existing, err := c.namespaces.Get(ctx, name)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("namespace %q already exists", name)
	}
	ns := &namespace.Namespace{Name: name, CreatedAt: time.Now().UTC()}
	if err := c.namespaces.Put(ctx, ns); err != nil {
		return nil, err
	}
	return ns, nil
}

// GetNamespace returns the namespace with the given name.
func (c *Core) GetNamespace(ctx context.Context, name string) (*namespace.Namespace, error) {
	return c.namespaces.Get(ctx, name)
}

// DeleteNamespace removes a namespace. Secrets and policies stored within it
// remain in storage but become inaccessible until the namespace is recreated.
func (c *Core) DeleteNamespace(ctx context.Context, name string) error {
	if name == "" || name == namespace.RootNamespace {
		return fmt.Errorf("cannot delete the root namespace")
	}
	return c.namespaces.Delete(ctx, name)
}

// ListNamespaces returns the names of all non-root namespaces.
func (c *Core) ListNamespaces(ctx context.Context) ([]string, error) {
	return c.namespaces.List(ctx)
}

// --- Cubbyhole (per-token private storage) ---

func (c *Core) CubbyholeGet(ctx context.Context, tokenID, path string) (map[string]interface{}, error) {
	return c.cubbyhole.Get(ctx, tokenID, path)
}

func (c *Core) CubbyholePut(ctx context.Context, tokenID, path string, data map[string]interface{}) error {
	return c.cubbyhole.Put(ctx, tokenID, path, data)
}

func (c *Core) CubbyholeDelete(ctx context.Context, tokenID, path string) error {
	return c.cubbyhole.Delete(ctx, tokenID, path)
}

func (c *Core) CubbyholeList(ctx context.Context, tokenID, pathPrefix string) ([]string, error) {
	return c.cubbyhole.List(ctx, tokenID, pathPrefix)
}
