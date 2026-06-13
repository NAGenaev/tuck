// Package aws implements the Tuck dynamic secrets engine for Amazon Web Services.
// On each credential request it either assumes an IAM role via STS (returning
// temporary credentials that expire naturally) or creates a short-lived IAM user
// with an access key that is deleted when the lease is revoked or expires.
package aws

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

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awscreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	ErrNotFound      = errors.New("aws: not found")
	ErrLeaseExpired  = errors.New("aws: lease expired")
	ErrNotConfigured = errors.New("aws: engine not configured — PUT /v1/aws/config first")
)

const (
	configKey = "dynamic/aws/config"
	rolesKey  = "dynamic/aws/roles/"
	leasesKey = "dynamic/aws/leases/"

	CredTypeIAMUser     = "iam_user"
	CredTypeAssumedRole = "assumed_role"

	defaultDefaultTTL = 1 * time.Hour
	defaultMaxTTL     = 12 * time.Hour
)

// Config holds AWS credentials and connection settings.
// If AccessKeyID is empty, credentials are resolved via the default chain
// (environment, shared config, EC2 instance metadata, IRSA).
type Config struct {
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	Region          string `json:"region"`
	IAMEndpoint     string `json:"iam_endpoint,omitempty"`
	STSEndpoint     string `json:"sts_endpoint,omitempty"`
}

// Role defines how credentials are generated.
type Role struct {
	Name           string        `json:"name"`
	CredentialType string        `json:"credential_type"` // "iam_user" or "assumed_role"
	PolicyARNs     []string      `json:"policy_arns,omitempty"`
	PolicyDocument string        `json:"policy_document,omitempty"`
	RoleARNs       []string      `json:"role_arns,omitempty"` // first ARN is assumed for assumed_role
	DefaultTTL     time.Duration `json:"default_ttl,omitempty"`
	MaxTTL         time.Duration `json:"max_ttl,omitempty"`
}

// Lease records a generated credential for later revocation.
type Lease struct {
	ID             string    `json:"id"`
	Role           string    `json:"role"`
	CredentialType string    `json:"credential_type"`
	Username       string    `json:"username,omitempty"`      // iam_user only
	AccessKeyID    string    `json:"access_key_id,omitempty"` // iam_user: key to delete on revoke
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Revoked        bool      `json:"revoked"`
}

// GenerateResult is returned by GenerateCreds.
type GenerateResult struct {
	LeaseID         string    `json:"lease_id"`
	AccessKeyID     string    `json:"access_key_id"`
	SecretAccessKey string    `json:"secret_access_key"`
	SessionToken    string    `json:"session_token,omitempty"` // assumed_role only
	ExpiresAt       time.Time `json:"expires_at"`
}

// IAMClient is a narrow interface for IAM operations; override in tests via WithIAMClient.
type IAMClient interface {
	CreateUser(ctx context.Context, username string) error
	PutUserPolicy(ctx context.Context, username, policyName, policyDoc string) error
	AttachUserPolicy(ctx context.Context, username, policyARN string) error
	CreateAccessKey(ctx context.Context, username string) (keyID, secret string, err error)
	DeleteAccessKey(ctx context.Context, username, keyID string) error
	DetachUserPolicy(ctx context.Context, username, policyARN string) error
	DeleteUserPolicy(ctx context.Context, username, policyName string) error
	DeleteUser(ctx context.Context, username string) error
}

// STSClient is a narrow interface for STS operations; override in tests via WithSTSClient.
type STSClient interface {
	AssumeRole(ctx context.Context, roleARN, sessionName string, duration time.Duration, policyDoc string) (keyID, secret, token string, expiry time.Time, err error)
}

type barrierIface interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Engine manages the AWS dynamic secrets engine.
type Engine struct {
	b      barrierIface
	newIAM func(*Config) (IAMClient, error)
	newSTS func(*Config) (STSClient, error)
	mu     sync.Mutex
}

// New creates an Engine backed by b.
func New(b barrierIface, opts ...func(*Engine)) *Engine {
	e := &Engine{b: b, newIAM: defaultNewIAM, newSTS: defaultNewSTS}
	for _, o := range opts {
		o(e)
	}
	return e
}

// WithIAMClient overrides the IAM client factory (used in tests).
func WithIAMClient(fn func(*Config) (IAMClient, error)) func(*Engine) {
	return func(e *Engine) { e.newIAM = fn }
}

// WithSTSClient overrides the STS client factory (used in tests).
func WithSTSClient(fn func(*Config) (STSClient, error)) func(*Engine) {
	return func(e *Engine) { e.newSTS = fn }
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
	case CredTypeAssumedRole:
		return e.generateAssumedRole(ctx, cfg, role, ttl)
	case CredTypeIAMUser:
		return e.generateIAMUser(ctx, cfg, role, ttl)
	default:
		return nil, fmt.Errorf("aws: unknown credential_type %q", role.CredentialType)
	}
}

func (e *Engine) generateAssumedRole(ctx context.Context, cfg *Config, role *Role, ttl time.Duration) (*GenerateResult, error) {
	if len(role.RoleARNs) == 0 {
		return nil, errors.New("aws: assumed_role requires at least one role_arn")
	}

	stsClient, err := e.newSTS(cfg)
	if err != nil {
		return nil, fmt.Errorf("aws: create STS client: %w", err)
	}

	sessionName := fmt.Sprintf("tuck-%s-%d", role.Name, time.Now().Unix())
	if len(sessionName) > 64 {
		sessionName = sessionName[:64]
	}

	keyID, secret, token, expiry, err := stsClient.AssumeRole(ctx, role.RoleARNs[0], sessionName, ttl, role.PolicyDocument)
	if err != nil {
		return nil, fmt.Errorf("aws: assume role: %w", err)
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
		CredentialType: CredTypeAssumedRole,
		AccessKeyID:    keyID,
		CreatedAt:      time.Now(),
		ExpiresAt:      expiresAt,
	}); err != nil {
		return nil, err
	}
	return &GenerateResult{
		LeaseID:         leaseID,
		AccessKeyID:     keyID,
		SecretAccessKey: secret,
		SessionToken:    token,
		ExpiresAt:       expiresAt,
	}, nil
}

func (e *Engine) generateIAMUser(ctx context.Context, cfg *Config, role *Role, ttl time.Duration) (*GenerateResult, error) {
	iamClient, err := e.newIAM(cfg)
	if err != nil {
		return nil, fmt.Errorf("aws: create IAM client: %w", err)
	}

	suffix, err := randomHex(6)
	if err != nil {
		return nil, err
	}
	username := fmt.Sprintf("tuck-%s-%s", role.Name, suffix)
	if len(username) > 64 {
		username = username[:64]
	}

	if err := iamClient.CreateUser(ctx, username); err != nil {
		return nil, fmt.Errorf("aws: create IAM user: %w", err)
	}

	for _, arn := range role.PolicyARNs {
		if err := iamClient.AttachUserPolicy(ctx, username, arn); err != nil {
			_ = iamClient.DeleteUser(ctx, username)
			return nil, fmt.Errorf("aws: attach policy %s: %w", arn, err)
		}
	}
	if role.PolicyDocument != "" {
		if err := iamClient.PutUserPolicy(ctx, username, "tuck-inline", role.PolicyDocument); err != nil {
			_ = iamClient.DeleteUser(ctx, username)
			return nil, fmt.Errorf("aws: put inline policy: %w", err)
		}
	}

	keyID, secret, err := iamClient.CreateAccessKey(ctx, username)
	if err != nil {
		_ = iamClient.DeleteUser(ctx, username)
		return nil, fmt.Errorf("aws: create access key: %w", err)
	}

	expiresAt := time.Now().Add(ttl)
	leaseID, err := randomID()
	if err != nil {
		return nil, err
	}
	if err := e.put(ctx, leasesKey+leaseID, &Lease{
		ID:             leaseID,
		Role:           role.Name,
		CredentialType: CredTypeIAMUser,
		Username:       username,
		AccessKeyID:    keyID,
		CreatedAt:      time.Now(),
		ExpiresAt:      expiresAt,
	}); err != nil {
		return nil, err
	}
	return &GenerateResult{
		LeaseID:         leaseID,
		AccessKeyID:     keyID,
		SecretAccessKey: secret,
		ExpiresAt:       expiresAt,
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
	if lease.CredentialType == CredTypeIAMUser && lease.Username != "" {
		cfg, cfgErr := e.GetConfig(ctx)
		role, roleErr := e.GetRole(ctx, lease.Role)
		if cfgErr == nil {
			if iamClient, iamErr := e.newIAM(cfg); iamErr == nil {
				if lease.AccessKeyID != "" {
					_ = iamClient.DeleteAccessKey(ctx, lease.Username, lease.AccessKeyID)
				}
				if roleErr == nil {
					for _, arn := range role.PolicyARNs {
						_ = iamClient.DetachUserPolicy(ctx, lease.Username, arn)
					}
					if role.PolicyDocument != "" {
						_ = iamClient.DeleteUserPolicy(ctx, lease.Username, "tuck-inline")
					}
				}
				_ = iamClient.DeleteUser(ctx, lease.Username)
			}
		}
	}
	lease.Revoked = true
	return e.put(ctx, leasesKey+id, lease)
}

func (e *Engine) ListLeases(ctx context.Context) ([]string, error) {
	return e.listTrimmed(ctx, leasesKey)
}

// RevokeExpired revokes all leases that have passed their ExpiresAt time.
// Call this from a background goroutine on a regular interval.
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

// --- random helpers ---

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// --- real AWS client implementations ---

func defaultNewIAM(cfg *Config) (IAMClient, error) {
	awsCfg, err := loadAWSConfig(cfg)
	if err != nil {
		return nil, err
	}
	var opts []func(*iam.Options)
	if cfg.IAMEndpoint != "" {
		ep := cfg.IAMEndpoint
		opts = append(opts, func(o *iam.Options) { o.BaseEndpoint = &ep })
	}
	return &realIAMClient{c: iam.NewFromConfig(awsCfg, opts...)}, nil
}

func defaultNewSTS(cfg *Config) (STSClient, error) {
	awsCfg, err := loadAWSConfig(cfg)
	if err != nil {
		return nil, err
	}
	var opts []func(*sts.Options)
	if cfg.STSEndpoint != "" {
		ep := cfg.STSEndpoint
		opts = append(opts, func(o *sts.Options) { o.BaseEndpoint = &ep })
	}
	return &realSTSClient{c: sts.NewFromConfig(awsCfg, opts...)}, nil
}

func loadAWSConfig(cfg *Config) (awsv2.Config, error) {
	var loaderOpts []func(*awsconfig.LoadOptions) error
	if cfg.Region != "" {
		loaderOpts = append(loaderOpts, awsconfig.WithRegion(cfg.Region))
	}
	if cfg.AccessKeyID != "" {
		loaderOpts = append(loaderOpts, awsconfig.WithCredentialsProvider(
			awscreds.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}
	return awsconfig.LoadDefaultConfig(context.Background(), loaderOpts...)
}

type realIAMClient struct{ c *iam.Client }

func (r *realIAMClient) CreateUser(ctx context.Context, username string) error {
	_, err := r.c.CreateUser(ctx, &iam.CreateUserInput{UserName: awsv2.String(username)})
	return err
}

func (r *realIAMClient) PutUserPolicy(ctx context.Context, username, policyName, policyDoc string) error {
	_, err := r.c.PutUserPolicy(ctx, &iam.PutUserPolicyInput{
		UserName:       awsv2.String(username),
		PolicyName:     awsv2.String(policyName),
		PolicyDocument: awsv2.String(policyDoc),
	})
	return err
}

func (r *realIAMClient) AttachUserPolicy(ctx context.Context, username, policyARN string) error {
	_, err := r.c.AttachUserPolicy(ctx, &iam.AttachUserPolicyInput{
		UserName:  awsv2.String(username),
		PolicyArn: awsv2.String(policyARN),
	})
	return err
}

func (r *realIAMClient) CreateAccessKey(ctx context.Context, username string) (string, string, error) {
	out, err := r.c.CreateAccessKey(ctx, &iam.CreateAccessKeyInput{UserName: awsv2.String(username)})
	if err != nil {
		return "", "", err
	}
	return awsv2.ToString(out.AccessKey.AccessKeyId), awsv2.ToString(out.AccessKey.SecretAccessKey), nil
}

func (r *realIAMClient) DeleteAccessKey(ctx context.Context, username, keyID string) error {
	_, err := r.c.DeleteAccessKey(ctx, &iam.DeleteAccessKeyInput{
		UserName:    awsv2.String(username),
		AccessKeyId: awsv2.String(keyID),
	})
	return err
}

func (r *realIAMClient) DetachUserPolicy(ctx context.Context, username, policyARN string) error {
	_, err := r.c.DetachUserPolicy(ctx, &iam.DetachUserPolicyInput{
		UserName:  awsv2.String(username),
		PolicyArn: awsv2.String(policyARN),
	})
	return err
}

func (r *realIAMClient) DeleteUserPolicy(ctx context.Context, username, policyName string) error {
	_, err := r.c.DeleteUserPolicy(ctx, &iam.DeleteUserPolicyInput{
		UserName:   awsv2.String(username),
		PolicyName: awsv2.String(policyName),
	})
	return err
}

func (r *realIAMClient) DeleteUser(ctx context.Context, username string) error {
	_, err := r.c.DeleteUser(ctx, &iam.DeleteUserInput{UserName: awsv2.String(username)})
	return err
}

type realSTSClient struct{ c *sts.Client }

func (r *realSTSClient) AssumeRole(ctx context.Context, roleARN, sessionName string, duration time.Duration, policyDoc string) (string, string, string, time.Time, error) {
	in := &sts.AssumeRoleInput{
		RoleArn:         awsv2.String(roleARN),
		RoleSessionName: awsv2.String(sessionName),
		DurationSeconds: awsv2.Int32(int32(duration.Seconds())),
	}
	if policyDoc != "" {
		in.Policy = awsv2.String(policyDoc)
	}
	out, err := r.c.AssumeRole(ctx, in)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	creds := out.Credentials
	expiry := time.Time{}
	if creds != nil && creds.Expiration != nil {
		expiry = *creds.Expiration
	}
	keyID, secret, token := "", "", ""
	if creds != nil {
		keyID = awsv2.ToString(creds.AccessKeyId)
		secret = awsv2.ToString(creds.SecretAccessKey)
		token = awsv2.ToString(creds.SessionToken)
	}
	return keyID, secret, token, expiry, nil
}
