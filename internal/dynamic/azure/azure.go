// Package azure implements the Tuck dynamic secrets engine for Microsoft Azure.
// It generates short-lived Azure AD client secrets for existing app registrations,
// using the Microsoft Graph API (https://graph.microsoft.com/v1.0/).
//
// Credential type:
//
//   - client_secret: adds a new password credential to an existing Azure AD
//     application. Returns the client_id + client_secret once. On revocation
//     the password credential is deleted via the Graph API removePassword call.
package azure

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	ErrNotFound      = errors.New("azure: not found")
	ErrNotConfigured = errors.New("azure: engine not configured — PUT /v1/azure/config first")
)

const (
	configKey = "dynamic/azure/config"
	rolesKey  = "dynamic/azure/roles/"
	leasesKey = "dynamic/azure/leases/"

	CredTypeClientSecret = "client_secret"

	defaultDefaultTTL = 1 * time.Hour
	defaultMaxTTL     = 12 * time.Hour

	graphBaseURL = "https://graph.microsoft.com/v1.0"
)

// Config holds Azure tenant credentials for the engine.
// ClientID + ClientSecret identify the service principal used to call Graph API;
// leave both empty to use DefaultAzureCredential (Managed Identity / env vars / CLI).
type Config struct {
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// Role defines how Azure credentials are generated.
type Role struct {
	Name                string        `json:"name"`
	ApplicationObjectID string        `json:"application_object_id"` // used for Graph API calls
	ApplicationID       string        `json:"application_id"`        // client_id returned to the caller
	DefaultTTL          time.Duration `json:"default_ttl,omitempty"`
	MaxTTL              time.Duration `json:"max_ttl,omitempty"`
}

// Lease records a generated credential for later revocation.
type Lease struct {
	ID                  string    `json:"id"`
	Role                string    `json:"role"`
	ApplicationObjectID string    `json:"application_object_id"`
	KeyID               string    `json:"key_id"` // Azure AD password credential keyId
	ExpiresAt           time.Time `json:"expires_at"`
	Revoked             bool      `json:"revoked"`
}

// GenerateResult is returned by GenerateCreds.
type GenerateResult struct {
	LeaseID      string    `json:"lease_id"`
	TenantID     string    `json:"tenant_id"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// AzureGraphClient adds and removes password credentials on Azure AD applications.
// Override in tests via WithGraphClient.
type AzureGraphClient interface {
	AddPassword(ctx context.Context, appObjectID, displayName string, expiresAt time.Time) (keyID, secretText string, err error)
	RemovePassword(ctx context.Context, appObjectID, keyID string) error
}

type barrierIface interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Engine manages the Azure dynamic secrets engine.
type Engine struct {
	b        barrierIface
	newGraph func(*Config) (AzureGraphClient, error)
	mu       sync.Mutex
}

// New creates an Engine backed by b.
func New(b barrierIface, opts ...func(*Engine)) *Engine {
	e := &Engine{b: b, newGraph: defaultNewGraph}
	for _, o := range opts {
		o(e)
	}
	return e
}

// WithGraphClient overrides the Graph API client factory (used in tests).
func WithGraphClient(fn func(*Config) (AzureGraphClient, error)) func(*Engine) {
	return func(e *Engine) { e.newGraph = fn }
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
	if role.ApplicationObjectID == "" {
		return nil, errors.New("azure: role missing application_object_id")
	}
	if role.ApplicationID == "" {
		return nil, errors.New("azure: role missing application_id")
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

	graphClient, err := e.newGraph(cfg)
	if err != nil {
		return nil, fmt.Errorf("azure: create Graph client: %w", err)
	}

	expiresAt := time.Now().Add(ttl)
	displayName := fmt.Sprintf("tuck-%s-%s", roleName, time.Now().UTC().Format("20060102150405"))

	keyID, secretText, err := graphClient.AddPassword(ctx, role.ApplicationObjectID, displayName, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("azure: add password credential: %w", err)
	}

	leaseID, err := randomID()
	if err != nil {
		return nil, err
	}
	if err := e.put(ctx, leasesKey+leaseID, &Lease{
		ID:                  leaseID,
		Role:                roleName,
		ApplicationObjectID: role.ApplicationObjectID,
		KeyID:               keyID,
		ExpiresAt:           expiresAt,
	}); err != nil {
		_ = graphClient.RemovePassword(ctx, role.ApplicationObjectID, keyID)
		return nil, err
	}

	return &GenerateResult{
		LeaseID:      leaseID,
		TenantID:     cfg.TenantID,
		ClientID:     role.ApplicationID,
		ClientSecret: secretText,
		ExpiresAt:    expiresAt,
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
	if lease.KeyID != "" && lease.ApplicationObjectID != "" {
		cfg, cfgErr := e.GetConfig(ctx)
		if cfgErr == nil {
			if graphClient, clientErr := e.newGraph(cfg); clientErr == nil {
				_ = graphClient.RemovePassword(ctx, lease.ApplicationObjectID, lease.KeyID)
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

// --- real Azure Graph client implementation ---

func defaultNewGraph(cfg *Config) (AzureGraphClient, error) {
	var tg azcore.TokenCredential
	var err error
	if cfg.ClientID != "" && cfg.ClientSecret != "" && cfg.TenantID != "" {
		tg, err = azidentity.NewClientSecretCredential(cfg.TenantID, cfg.ClientID, cfg.ClientSecret, nil)
	} else {
		tg, err = azidentity.NewDefaultAzureCredential(nil)
	}
	if err != nil {
		return nil, err
	}
	return &realGraphClient{
		cred:       tg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

type realGraphClient struct {
	cred       azcore.TokenCredential
	httpClient *http.Client
}

func (c *realGraphClient) getToken(ctx context.Context) (string, error) {
	tok, err := c.cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://graph.microsoft.com/.default"},
	})
	if err != nil {
		return "", err
	}
	return tok.Token, nil
}

func (c *realGraphClient) AddPassword(ctx context.Context, appObjectID, displayName string, expiresAt time.Time) (string, string, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return "", "", err
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"passwordCredential": map[string]interface{}{
			"displayName": displayName,
			"endDateTime": expiresAt.UTC().Format(time.RFC3339),
		},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		graphBaseURL+"/applications/"+appObjectID+"/addPassword",
		bytes.NewReader(reqBody))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("azure: addPassword: status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		KeyID      string `json:"keyId"`
		SecretText string `json:"secretText"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("azure: decode addPassword response: %w", err)
	}
	return result.KeyID, result.SecretText, nil
}

func (c *realGraphClient) RemovePassword(ctx context.Context, appObjectID, keyID string) error {
	token, err := c.getToken(ctx)
	if err != nil {
		return err
	}

	reqBody, _ := json.Marshal(map[string]string{"keyId": keyID})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		graphBaseURL+"/applications/"+appObjectID+"/removePassword",
		bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("azure: removePassword: status %d: %s", resp.StatusCode, body)
	}
	return nil
}
