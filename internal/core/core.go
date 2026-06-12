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
	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/kvv2"
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
	backend  physical.Backend
	barrier  *barrier.Barrier
	seal     seal.Seal
	tokens   *token.Store
	policies *policy.Store
	kv2      *kvv2.Store
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
	_ = c.cubbyhole.PurgeToken(ctx, tokenID)
	return c.tokens.Delete(ctx, tokenID)
}

func (c *Core) LookupToken(ctx context.Context, tokenID string) (*token.Token, error) {
	return c.tokens.Get(ctx, tokenID)
}

// ListTokens returns all token IDs in the store.
func (c *Core) ListTokens(ctx context.Context) ([]string, error) {
	return c.tokens.List(ctx)
}

// RenewToken extends tokenID's expiry by ttl (default 1h when ttl≤0).
// Returns ErrTokenInvalid if the token doesn't exist or is already expired.
func (c *Core) RenewToken(ctx context.Context, tokenID string, ttl time.Duration) (*token.Token, error) {
	tok, err := c.tokens.Get(ctx, tokenID)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	if tok.IsExpired() {
		return nil, ErrTokenInvalid
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	tok.ExpiresAt = time.Now().UTC().Add(ttl)
	if err := c.tokens.Put(ctx, tok); err != nil {
		return nil, err
	}
	return tok, nil
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

// ListPolicies returns all policy names in the store.
func (c *Core) ListPolicies(ctx context.Context) ([]string, error) {
	return c.policies.List(ctx)
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

// ListSecrets returns all secret keys whose storage key starts with the given
// prefix path. An empty prefix lists every secret.
func (c *Core) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	storagePrefix := secretPrefix
	if prefix != "" {
		p := secretKey(prefix)
		// Ensure a trailing slash so we list within the directory, not just
		// exact matches.
		if !strings.HasSuffix(p, "/") {
			p += "/"
		}
		storagePrefix = p
	}
	keys, err := c.barrier.List(ctx, storagePrefix)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(keys))
	for i, k := range keys {
		result[i] = strings.TrimPrefix(k, secretPrefix)
	}
	return result, nil
}

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

// KVv2 returns the versioned KV store.
func (c *Core) KVv2() *kvv2.Store { return c.kv2 }

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
	displayName := "jwt:" + result.Subject
	return c.CreateToken(ctx, displayName, result.Policies, result.TTL)
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
	return c.CreateToken(ctx, result.Subject, result.Policies, result.TTL)
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
	displayName := "ldap:" + result.Username
	return c.CreateToken(ctx, displayName, result.Policies, result.TTL)
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
