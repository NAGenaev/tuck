// Package gcp implements the Tuck dynamic secrets engine for Google Cloud Platform.
// Two credential types are supported:
//
//   - service_account_key: creates a new IAM service account key (JSON key file),
//     returns it once. On revocation the key is deleted from GCP IAM.
//
//   - access_token: generates a short-lived OAuth2 access token for a service
//     account via the IAM Credentials API. The token expires naturally; no
//     cleanup is needed on revocation.
package gcp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/iam/v1"
	"google.golang.org/api/iamcredentials/v1"
	"google.golang.org/api/option"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	ErrNotFound      = errors.New("gcp: not found")
	ErrLeaseExpired  = errors.New("gcp: lease expired")
	ErrNotConfigured = errors.New("gcp: engine not configured — PUT /v1/gcp/config first")
)

const (
	configKey = "dynamic/gcp/config"
	rolesKey  = "dynamic/gcp/roles/"
	leasesKey = "dynamic/gcp/leases/"

	CredTypeServiceAccountKey = "service_account_key"
	CredTypeAccessToken       = "access_token"

	KeyAlgRSA2048 = "KEY_ALG_RSA_2048"
	KeyAlgRSA4096 = "KEY_ALG_RSA_4096"

	defaultDefaultTTL = 1 * time.Hour
	defaultMaxTTL     = 12 * time.Hour
)

// Config holds GCP credentials and settings for the engine.
// CredentialsJSON may contain inline service account JSON; empty = Application
// Default Credentials (Workload Identity, GOOGLE_APPLICATION_CREDENTIALS, etc.).
type Config struct {
	CredentialsJSON string `json:"credentials_json,omitempty"`
}

// Role defines how GCP credentials are generated.
type Role struct {
	Name                string        `json:"name"`
	CredentialType      string        `json:"credential_type"` // "service_account_key" or "access_token"
	ServiceAccountEmail string        `json:"service_account_email"`
	KeyAlgorithm        string        `json:"key_algorithm,omitempty"` // KEY_ALG_RSA_2048 (default) or KEY_ALG_RSA_4096
	Scopes              []string      `json:"scopes,omitempty"`        // OAuth2 scopes (access_token only)
	DefaultTTL          time.Duration `json:"default_ttl,omitempty"`
	MaxTTL              time.Duration `json:"max_ttl,omitempty"`
}

// Lease records a generated credential for later revocation.
type Lease struct {
	ID             string    `json:"id"`
	Role           string    `json:"role"`
	CredentialType string    `json:"credential_type"`
	GCPKeyName     string    `json:"gcp_key_name,omitempty"` // service_account_key: resource name for deletion
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Revoked        bool      `json:"revoked"`
}

// GenerateResult is returned by GenerateCreds.
type GenerateResult struct {
	LeaseID     string    `json:"lease_id"`
	PrivateKey  string    `json:"private_key,omitempty"`   // service_account_key: JSON key file
	AccessToken string    `json:"access_token,omitempty"`  // access_token: Bearer token
	TokenType   string    `json:"token_type,omitempty"`    // "Bearer" for access_token
	ExpiresAt   time.Time `json:"expires_at"`
}

// GCPAdminClient manages service account keys; override in tests via WithAdminClient.
type GCPAdminClient interface {
	CreateKey(ctx context.Context, serviceAccount, keyAlgorithm string) (gcpKeyName, privateKeyJSON string, err error)
	DeleteKey(ctx context.Context, gcpKeyName string) error
}

// GCPTokenClient generates OAuth2 access tokens; override in tests via WithTokenClient.
type GCPTokenClient interface {
	GenerateAccessToken(ctx context.Context, serviceAccount string, scopes []string, lifetime time.Duration) (token string, expiry time.Time, err error)
}

type barrierIface interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Engine manages the GCP dynamic secrets engine.
type Engine struct {
	b        barrierIface
	newAdmin func(*Config) (GCPAdminClient, error)
	newToken func(*Config) (GCPTokenClient, error)
	mu       sync.Mutex
}

// New creates an Engine backed by b.
func New(b barrierIface, opts ...func(*Engine)) *Engine {
	e := &Engine{b: b, newAdmin: defaultNewAdmin, newToken: defaultNewToken}
	for _, o := range opts {
		o(e)
	}
	return e
}

// WithAdminClient overrides the IAM admin client factory (used in tests).
func WithAdminClient(fn func(*Config) (GCPAdminClient, error)) func(*Engine) {
	return func(e *Engine) { e.newAdmin = fn }
}

// WithTokenClient overrides the IAM credentials client factory (used in tests).
func WithTokenClient(fn func(*Config) (GCPTokenClient, error)) func(*Engine) {
	return func(e *Engine) { e.newToken = fn }
}

// --- Config ---

func (e *Engine) PutConfig(ctx context.Context, cfg *Config) error {
	return e.put(ctx, configKey, cfg)
}

func (e *Engine) GetConfig(ctx context.Context) (*Config, error) {
	var cfg Config
	if err := e.get(ctx, configKey, &cfg); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotConfigured
		}
		return nil, err
	}
	return &cfg, nil
}

func (e *Engine) DeleteConfig(ctx context.Context) error {
	return e.b.Delete(ctx, configKey)
}

// --- Roles ---

func (e *Engine) PutRole(ctx context.Context, role *Role) error {
	return e.put(ctx, rolesKey+role.Name, role)
}

func (e *Engine) GetRole(ctx context.Context, name string) (*Role, error) {
	var r Role
	if err := e.get(ctx, rolesKey+name, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (e *Engine) DeleteRole(ctx context.Context, name string) error {
	return e.b.Delete(ctx, rolesKey+name)
}

func (e *Engine) ListRoles(ctx context.Context) ([]string, error) {
	return e.listTrimmed(ctx, rolesKey)
}

// --- Credential generation ---

func (e *Engine) GenerateCreds(ctx context.Context, roleName string) (*GenerateResult, error) {
	cfg, err := e.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	role, err := e.GetRole(ctx, roleName)
	if err != nil {
		return nil, err
	}
	if role.ServiceAccountEmail == "" {
		return nil, errors.New("gcp: role missing service_account_email")
	}

	ttl := role.DefaultTTL
	if ttl == 0 {
		ttl = defaultDefaultTTL
	}
	maxTTL := role.MaxTTL
	if maxTTL == 0 {
		maxTTL = defaultMaxTTL
	}
	if ttl > maxTTL {
		ttl = maxTTL
	}

	switch role.CredentialType {
	case CredTypeServiceAccountKey:
		return e.generateServiceAccountKey(ctx, cfg, role, ttl)
	case CredTypeAccessToken:
		return e.generateAccessToken(ctx, cfg, role, ttl)
	default:
		return nil, fmt.Errorf("gcp: unknown credential_type %q", role.CredentialType)
	}
}

func (e *Engine) generateServiceAccountKey(ctx context.Context, cfg *Config, role *Role, ttl time.Duration) (*GenerateResult, error) {
	adminClient, err := e.newAdmin(cfg)
	if err != nil {
		return nil, fmt.Errorf("gcp: create IAM admin client: %w", err)
	}

	alg := role.KeyAlgorithm
	if alg == "" {
		alg = KeyAlgRSA2048
	}

	gcpKeyName, privateKeyJSON, err := adminClient.CreateKey(ctx, role.ServiceAccountEmail, alg)
	if err != nil {
		return nil, fmt.Errorf("gcp: create service account key: %w", err)
	}

	expiresAt := time.Now().Add(ttl)
	leaseID, err := randomID()
	if err != nil {
		return nil, err
	}
	if err := e.put(ctx, leasesKey+leaseID, &Lease{
		ID:             leaseID,
		Role:           role.Name,
		CredentialType: CredTypeServiceAccountKey,
		GCPKeyName:     gcpKeyName,
		CreatedAt:      time.Now(),
		ExpiresAt:      expiresAt,
	}); err != nil {
		// Best-effort: delete the key we just created since we can't track it.
		_ = adminClient.DeleteKey(ctx, gcpKeyName)
		return nil, err
	}
	return &GenerateResult{
		LeaseID:    leaseID,
		PrivateKey: privateKeyJSON,
		ExpiresAt:  expiresAt,
	}, nil
}

func (e *Engine) generateAccessToken(ctx context.Context, cfg *Config, role *Role, ttl time.Duration) (*GenerateResult, error) {
	tokenClient, err := e.newToken(cfg)
	if err != nil {
		return nil, fmt.Errorf("gcp: create IAM credentials client: %w", err)
	}

	scopes := role.Scopes
	if len(scopes) == 0 {
		scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}
	}

	token, expiry, err := tokenClient.GenerateAccessToken(ctx, role.ServiceAccountEmail, scopes, ttl)
	if err != nil {
		return nil, fmt.Errorf("gcp: generate access token: %w", err)
	}
	expiresAt := expiry
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(ttl)
	}

	leaseID, err := randomID()
	if err != nil {
		return nil, err
	}
	if err := e.put(ctx, leasesKey+leaseID, &Lease{
		ID:             leaseID,
		Role:           role.Name,
		CredentialType: CredTypeAccessToken,
		CreatedAt:      time.Now(),
		ExpiresAt:      expiresAt,
	}); err != nil {
		return nil, err
	}
	return &GenerateResult{
		LeaseID:     leaseID,
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
	}, nil
}

// --- Leases ---

func (e *Engine) GetLease(ctx context.Context, id string) (*Lease, error) {
	var lease Lease
	if err := e.get(ctx, leasesKey+id, &lease); err != nil {
		return nil, err
	}
	return &lease, nil
}

func (e *Engine) RevokeLease(ctx context.Context, id string) error {
	lease, err := e.GetLease(ctx, id)
	if err != nil {
		return err
	}
	if lease.Revoked {
		return nil
	}
	if lease.CredentialType == CredTypeServiceAccountKey && lease.GCPKeyName != "" {
		cfg, cfgErr := e.GetConfig(ctx)
		if cfgErr == nil {
			if adminClient, clientErr := e.newAdmin(cfg); clientErr == nil {
				_ = adminClient.DeleteKey(ctx, lease.GCPKeyName)
			}
		}
	}
	lease.Revoked = true
	return e.put(ctx, leasesKey+id, lease)
}

func (e *Engine) ListLeases(ctx context.Context) ([]string, error) {
	return e.listTrimmed(ctx, leasesKey)
}

// RenewLease extends the lease TTL by increment, capped at CreatedAt + role.MaxTTL.
func (e *Engine) RenewLease(ctx context.Context, id string, increment time.Duration) (time.Time, error) {
	lease, err := e.GetLease(ctx, id)
	if err != nil {
		return time.Time{}, err
	}
	if time.Now().After(lease.ExpiresAt) {
		return time.Time{}, ErrLeaseExpired
	}
	newExpiry := time.Now().Add(increment)
	if role, rerr := e.GetRole(ctx, lease.Role); rerr == nil && role.MaxTTL > 0 {
		if cap := lease.CreatedAt.Add(role.MaxTTL); newExpiry.After(cap) {
			newExpiry = cap
		}
	}
	lease.ExpiresAt = newExpiry
	if err := e.put(ctx, leasesKey+id, lease); err != nil {
		return time.Time{}, err
	}
	return lease.ExpiresAt, nil
}

// RevokeExpired revokes all leases that have passed their ExpiresAt time.
func (e *Engine) RevokeExpired(ctx context.Context) error {
	ids, err := e.ListLeases(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, id := range ids {
		lease, err := e.GetLease(ctx, id)
		if err != nil || lease.Revoked || !lease.ExpiresAt.Before(now) {
			continue
		}
		_ = e.RevokeLease(ctx, id)
	}
	return nil
}

// --- barrier helpers ---

func (e *Engine) get(ctx context.Context, key string, dst interface{}) error {
	entry, err := e.b.Get(ctx, key)
	if err != nil {
		return err
	}
	if entry == nil {
		return ErrNotFound
	}
	return json.Unmarshal(entry.Value, dst)
}

func (e *Engine) put(ctx context.Context, key string, src interface{}) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return e.b.Put(ctx, &physical.Entry{Key: key, Value: data})
}

func (e *Engine) listTrimmed(ctx context.Context, prefix string) ([]string, error) {
	keys, err := e.b.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	for i, k := range keys {
		keys[i] = strings.TrimPrefix(k, prefix)
	}
	return keys, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// --- real GCP client implementations ---

func defaultNewAdmin(cfg *Config) (GCPAdminClient, error) {
	var opts []option.ClientOption
	if cfg.CredentialsJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(cfg.CredentialsJSON)))
	}
	svc, err := iam.NewService(context.Background(), opts...)
	if err != nil {
		return nil, err
	}
	return &realAdminClient{svc: svc}, nil
}

func defaultNewToken(cfg *Config) (GCPTokenClient, error) {
	var opts []option.ClientOption
	if cfg.CredentialsJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(cfg.CredentialsJSON)))
	}
	svc, err := iamcredentials.NewService(context.Background(), opts...)
	if err != nil {
		return nil, err
	}
	return &realTokenClient{svc: svc}, nil
}

type realAdminClient struct{ svc *iam.Service }

func (r *realAdminClient) CreateKey(ctx context.Context, serviceAccount, keyAlgorithm string) (string, string, error) {
	resource := "projects/-/serviceAccounts/" + serviceAccount
	key, err := r.svc.Projects.ServiceAccounts.Keys.Create(resource, &iam.CreateServiceAccountKeyRequest{
		PrivateKeyType: "TYPE_GOOGLE_CREDENTIALS_FILE",
		KeyAlgorithm:   keyAlgorithm,
	}).Context(ctx).Do()
	if err != nil {
		return "", "", err
	}
	// PrivateKeyData is base64-encoded; decode to get the raw JSON key file.
	raw, err := base64.StdEncoding.DecodeString(key.PrivateKeyData)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(key.PrivateKeyData)
		if err != nil {
			return "", "", fmt.Errorf("gcp: decode private key data: %w", err)
		}
	}
	return key.Name, string(raw), nil
}

func (r *realAdminClient) DeleteKey(ctx context.Context, gcpKeyName string) error {
	_, err := r.svc.Projects.ServiceAccounts.Keys.Delete(gcpKeyName).Context(ctx).Do()
	return err
}

type realTokenClient struct{ svc *iamcredentials.Service }

func (r *realTokenClient) GenerateAccessToken(ctx context.Context, serviceAccount string, scopes []string, lifetime time.Duration) (string, time.Time, error) {
	resource := "projects/-/serviceAccounts/" + serviceAccount
	resp, err := r.svc.Projects.ServiceAccounts.GenerateAccessToken(resource, &iamcredentials.GenerateAccessTokenRequest{
		Scope:    scopes,
		Lifetime: fmt.Sprintf("%ds", int(lifetime.Seconds())),
	}).Context(ctx).Do()
	if err != nil {
		return "", time.Time{}, err
	}
	expiry, _ := time.Parse(time.RFC3339, resp.ExpireTime)
	return resp.AccessToken, expiry, nil
}
