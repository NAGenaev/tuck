// Package client provides a Go SDK for the Tuck secrets manager API.
//
// Quick start:
//
//	c := client.New("https://tuck:8200", "tuck_mytoken")
//	val, err := c.GetSecret(ctx, "db/password")
//
// For self-signed dev certificates use client.WithInsecure():
//
//	c := client.New(addr, token, client.WithInsecure())
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a Tuck server.
type Client struct {
	addr      string
	token     string
	namespace string // X-Tuck-Namespace header value; empty = root
	http      *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithInsecure skips TLS certificate verification. Use only in development.
func WithInsecure() Option {
	return func(c *Client) {
		c.http = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 — caller opts in via WithInsecure()
			},
			Timeout: 30 * time.Second,
		}
	}
}

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithNamespace scopes all requests to a Tuck namespace.
func WithNamespace(ns string) Option {
	return func(c *Client) { c.namespace = ns }
}

// Scoped returns a shallow copy of c bound to the given namespace.
// All requests made by the returned client will carry X-Tuck-Namespace: ns.
func (c *Client) Scoped(ns string) *Client {
	cp := *c
	cp.namespace = ns
	return &cp
}

// New creates a Client. addr is the server base URL (e.g. "https://tuck:8200"),
// token is the X-Tuck-Token value.
func New(addr, token string, opts ...Option) *Client {
	c := &Client{
		addr:  strings.TrimRight(addr, "/"),
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// --- Seal ---

// SealStatus holds the current seal state.
type SealStatus struct {
	Sealed         bool   `json:"sealed"`
	Type           string `json:"type"`
	RequiredShards int    `json:"required_shards,omitempty"`
	ReceivedShards int    `json:"received_shards,omitempty"`
}

// SealStatus returns the server's current seal state.
func (c *Client) SealStatus(ctx context.Context) (*SealStatus, error) {
	var s SealStatus
	return &s, c.get(ctx, "/v1/sys/seal-status", &s)
}

// HealthInfo holds the response from GET /v1/health.
type HealthInfo struct {
	Version       string  `json:"version"`
	Commit        string  `json:"commit"`
	BuildDate     string  `json:"build_date"`
	Sealed        bool    `json:"sealed"`
	HAEnabled     bool    `json:"ha_enabled"`
	UptimeSeconds float64 `json:"uptime_seconds"`
}

// Health returns server health, version, and uptime information.
func (c *Client) Health(ctx context.Context) (*HealthInfo, error) {
	var h HealthInfo
	return &h, c.get(ctx, "/v1/health", &h)
}

// Unseal submits one Shamir share. Returns the updated seal status.
func (c *Client) Unseal(ctx context.Context, share string) (*SealStatus, error) {
	var s SealStatus
	return &s, c.post(ctx, "/v1/sys/unseal", map[string]string{"key": share}, &s)
}

// Seal re-seals the server, dropping the in-memory barrier key.
func (c *Client) Seal(ctx context.Context) error {
	return c.post(ctx, "/v1/sys/seal", nil, nil)
}

// RotateResult is returned by Rotate.
type RotateResult struct {
	OK     bool     `json:"ok"`
	Shares []string `json:"shares,omitempty"`
}

// Rotate generates a new root key and re-wraps the DEK. Returns new Shamir
// shares for ShamirSeal; nil for other seal types. Requires root policy.
func (c *Client) Rotate(ctx context.Context) (*RotateResult, error) {
	var r RotateResult
	return &r, c.post(ctx, "/v1/sys/rotate", nil, &r)
}

// Snapshot downloads a bbolt database snapshot and writes it to w.
// Requires root policy.
func (c *Client) Snapshot(ctx context.Context, w io.Writer) error {
	req, err := c.newReq(ctx, http.MethodGet, "/v1/sys/snapshot", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return err
	}
	_, err = io.Copy(w, resp.Body)
	return err
}

// --- KV v1 ---

// GetSecret reads a secret by logical path. Returns (nil, nil) if not found.
func (c *Client) GetSecret(ctx context.Context, path string) ([]byte, error) {
	var resp struct {
		Value    string `json:"value"`
		Encoding string `json:"encoding"`
	}
	if err := c.get(ctx, "/v1/secret/"+trimSlash(path), &resp); err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if resp.Encoding == "base64" {
		return base64.StdEncoding.DecodeString(resp.Value)
	}
	return []byte(resp.Value), nil
}

// PutSecret stores bytes at the logical path.
func (c *Client) PutSecret(ctx context.Context, path string, value []byte) error {
	return c.putRaw(ctx, "/v1/secret/"+trimSlash(path), value)
}

// DeleteSecret removes a secret.
func (c *Client) DeleteSecret(ctx context.Context, path string) error {
	return c.doDelete(ctx, "/v1/secret/"+trimSlash(path))
}

// ListSecrets returns all secret keys under prefix.
func (c *Client) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := c.doList(ctx, "/v1/secret/"+trimSlash(prefix), &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// --- KV v2 ---

// KVv2 returns a client scoped to the versioned KV v2 engine.
func (c *Client) KVv2() *KVv2Client { return &KVv2Client{c: c} }

// KVv2Client wraps versioned KV v2 operations.
type KVv2Client struct{ c *Client }

// WriteResult is returned by KVv2Client.Write.
type WriteResult struct {
	Version int    `json:"version"`
	Path    string `json:"path"`
}

// Write stores value as a new version. Pass cas=-1 to skip check-and-set;
// otherwise the write fails unless the current version equals cas.
func (v *KVv2Client) Write(ctx context.Context, path string, value []byte, cas int) (*WriteResult, error) {
	url := "/v2/secret/" + trimSlash(path)
	if cas >= 0 {
		url += fmt.Sprintf("?cas=%d", cas)
	}
	req, err := v.c.newReq(ctx, http.MethodPut, url, bytes.NewReader(value))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := v.c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	var res WriteResult
	return &res, json.NewDecoder(resp.Body).Decode(&res)
}

// VerMeta holds version-level metadata.
type VerMeta struct {
	Version   int        `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	Destroyed bool       `json:"destroyed"`
}

// ReadResult is returned by KVv2Client.Read.
type ReadResult struct {
	Path     string   `json:"path"`
	Version  int      `json:"version"`
	Value    []byte   // nil for soft-deleted versions
	Deleted  bool     `json:"deleted"`
	Metadata *VerMeta `json:"metadata"`
}

// Read returns the value and metadata for the given version (0 = current).
func (v *KVv2Client) Read(ctx context.Context, path string, version int) (*ReadResult, error) {
	url := "/v2/secret/" + trimSlash(path)
	if version > 0 {
		url += fmt.Sprintf("?version=%d", version)
	}
	var raw struct {
		Path     string   `json:"path"`
		Version  int      `json:"version"`
		Value    string   `json:"value"`
		Encoding string   `json:"encoding"`
		Deleted  bool     `json:"deleted"`
		Metadata *VerMeta `json:"metadata"`
	}
	if err := v.c.get(ctx, url, &raw); err != nil {
		return nil, err
	}
	res := &ReadResult{
		Path:     raw.Path,
		Version:  raw.Version,
		Deleted:  raw.Deleted,
		Metadata: raw.Metadata,
	}
	if raw.Value != "" {
		if raw.Encoding == "base64" {
			val, err := base64.StdEncoding.DecodeString(raw.Value)
			if err != nil {
				return nil, fmt.Errorf("decode base64 value: %w", err)
			}
			res.Value = val
		} else {
			res.Value = []byte(raw.Value)
		}
	}
	return res, nil
}

// SoftDelete marks versions as deleted. Data is preserved and recoverable.
func (v *KVv2Client) SoftDelete(ctx context.Context, path string, versions []int) error {
	url := fmt.Sprintf("/v2/secret/%s?versions=%s", trimSlash(path), joinInts(versions))
	return v.c.doDelete(ctx, url)
}

// Undelete recovers soft-deleted versions.
func (v *KVv2Client) Undelete(ctx context.Context, path string, versions []int) error {
	return v.c.post(ctx, "/v2/secret/undelete/"+trimSlash(path),
		map[string][]int{"versions": versions}, nil)
}

// Destroy permanently removes version data. This cannot be undone.
func (v *KVv2Client) Destroy(ctx context.Context, path string, versions []int) error {
	return v.c.post(ctx, "/v2/secret/destroy/"+trimSlash(path),
		map[string][]int{"versions": versions}, nil)
}

// KVMeta is returned by GetMeta.
type KVMeta struct {
	CurrentVersion int                `json:"current_version"`
	MaxVersions    int                `json:"max_versions"`
	Versions       map[string]VerMeta `json:"versions"`
}

// GetMeta returns version metadata for a path.
func (v *KVv2Client) GetMeta(ctx context.Context, path string) (*KVMeta, error) {
	var resp struct {
		Metadata *KVMeta `json:"metadata"`
	}
	if err := v.c.get(ctx, "/v2/secret/metadata/"+trimSlash(path), &resp); err != nil {
		return nil, err
	}
	return resp.Metadata, nil
}

// UpdateMeta sets the max_versions limit for a path.
func (v *KVv2Client) UpdateMeta(ctx context.Context, path string, maxVersions int) error {
	return v.c.putJSON(ctx, "/v2/secret/metadata/"+trimSlash(path),
		map[string]int{"max_versions": maxVersions})
}

// DeleteAll permanently removes all versions and metadata for a path.
func (v *KVv2Client) DeleteAll(ctx context.Context, path string) error {
	return v.c.doDelete(ctx, "/v2/secret/metadata/"+trimSlash(path))
}

// List returns the secret paths under prefix.
func (v *KVv2Client) List(ctx context.Context, prefix string) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := v.c.doList(ctx, "/v2/secret/metadata/"+trimSlash(prefix), &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// --- Tokens ---

// Token represents a Tuck auth token.
type Token struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Policies    []string  `json:"policies"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateTokenRequest holds parameters for token creation.
type CreateTokenRequest struct {
	DisplayName string
	Policies    []string
	TTL         time.Duration // 0 = no expiry
}

// CreateToken creates a new service token.
func (c *Client) CreateToken(ctx context.Context, req CreateTokenRequest) (*Token, error) {
	body := map[string]any{
		"display_name": req.DisplayName,
		"policies":     req.Policies,
	}
	if req.TTL > 0 {
		body["ttl"] = req.TTL.String()
	}
	var tok Token
	return &tok, c.post(ctx, "/v1/auth/token", body, &tok)
}

// GetToken looks up a token by ID.
func (c *Client) GetToken(ctx context.Context, id string) (*Token, error) {
	var tok Token
	return &tok, c.get(ctx, "/v1/auth/token/"+id, &tok)
}

// RevokeToken revokes a token immediately.
func (c *Client) RevokeToken(ctx context.Context, id string) error {
	return c.doDelete(ctx, "/v1/auth/token/"+id)
}

// RenewToken extends a token's expiry. ttl=0 uses the server default (1h).
func (c *Client) RenewToken(ctx context.Context, id string, ttl time.Duration) (*Token, error) {
	body := map[string]string{}
	if ttl > 0 {
		body["ttl"] = ttl.String()
	}
	var tok Token
	return &tok, c.post(ctx, "/v1/auth/token/"+id+"/renew", body, &tok)
}

// ListTokens returns all token IDs in the store.
func (c *Client) ListTokens(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := c.doList(ctx, "/v1/auth/token/", &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// --- Policies ---

// Rule is a single ACL rule.
type Rule struct {
	Path         string   `json:"path"`
	Capabilities []string `json:"capabilities"`
}

// Policy is a named set of ACL rules.
type Policy struct {
	Name  string `json:"name"`
	Rules []Rule `json:"rules"`
}

// GetPolicy retrieves a policy by name.
func (c *Client) GetPolicy(ctx context.Context, name string) (*Policy, error) {
	var p Policy
	return &p, c.get(ctx, "/v1/policy/"+name, &p)
}

// PutPolicy creates or replaces a policy.
func (c *Client) PutPolicy(ctx context.Context, name string, rules []Rule) error {
	return c.putJSON(ctx, "/v1/policy/"+name, rules)
}

// DeletePolicy removes a policy by name.
func (c *Client) DeletePolicy(ctx context.Context, name string) error {
	return c.doDelete(ctx, "/v1/policy/"+name)
}

// ListPolicies returns all policy names.
func (c *Client) ListPolicies(ctx context.Context) ([]string, error) {
	var resp struct {
		Keys []string `json:"keys"`
	}
	if err := c.doList(ctx, "/v1/policy/", &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// --- Errors ---

// Error is returned when the server responds with a non-2xx status.
type Error struct {
	StatusCode int
	Message    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("tuck: HTTP %d: %s", e.StatusCode, e.Message)
}

// IsNotFound reports whether the error is a 404 Not Found.
func IsNotFound(err error) bool {
	e, ok := err.(*Error)
	return ok && e.StatusCode == http.StatusNotFound
}

// IsSealed reports whether the error is a 503 (server is sealed).
func IsSealed(err error) bool {
	e, ok := err.(*Error)
	return ok && e.StatusCode == http.StatusServiceUnavailable
}

// IsUnauthorized reports whether the error is a 401 or 403.
func IsUnauthorized(err error) bool {
	e, ok := err.(*Error)
	return ok && (e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden)
}

// --- transport helpers ---

func (c *Client) newReq(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.addr+path, body)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("X-Tuck-Token", c.token)
	}
	if c.namespace != "" {
		req.Header.Set("X-Tuck-Namespace", c.namespace)
	}
	return req, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := c.newReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return err
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := c.newReq(ctx, http.MethodPost, path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return err
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) putRaw(ctx context.Context, path string, value []byte) error {
	req, err := c.newReq(ctx, http.MethodPut, path, bytes.NewReader(value))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp)
}

func (c *Client) putJSON(ctx context.Context, path string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := c.newReq(ctx, http.MethodPut, path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp)
}

func (c *Client) doDelete(ctx context.Context, path string) error {
	req, err := c.newReq(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp)
}

func (c *Client) doList(ctx context.Context, path string, out any) error {
	req, err := c.newReq(ctx, "LIST", path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	var e struct {
		Err string `json:"error"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = json.Unmarshal(b, &e)
	msg := e.Err
	if msg == "" {
		msg = strings.TrimSpace(string(b))
	}
	return &Error{StatusCode: resp.StatusCode, Message: msg}
}

func trimSlash(p string) string { return strings.TrimPrefix(p, "/") }

func joinInts(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, ",")
}
