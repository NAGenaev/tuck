package provider

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// tuckClient is a minimal HTTP client for the Tuck API.
type tuckClient struct {
	addr      string
	token     string
	namespace string
	http      *http.Client
}

func newTuckClient(addr, token, namespace string, insecure bool) *tuckClient {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 — gated by explicit insecure=true provider attribute
	}
	return &tuckClient{
		addr:      strings.TrimRight(addr, "/"),
		token:     token,
		namespace: namespace,
		http:      &http.Client{Transport: tr, Timeout: 30 * time.Second},
	}
}

func (c *tuckClient) req(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.addr+path, body) // #nosec G107 — addr is user-supplied server URL
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

func (c *tuckClient) doJSON(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		r = bytes.NewReader(data)
	}
	httpReq, err := c.req(ctx, method, path, r)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	return c.exec(httpReq)
}

func (c *tuckClient) doRaw(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	httpReq, err := c.req(ctx, method, path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	return c.exec(httpReq)
}

func (c *tuckClient) exec(req *http.Request) ([]byte, int, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return body, resp.StatusCode, nil
}

// getSecret reads a KV v1 secret. Returns ("", false, nil) if not found.
func (c *tuckClient) getSecret(ctx context.Context, path string) (string, bool, error) {
	body, status, err := c.doJSON(ctx, http.MethodGet, "/v1/secret/"+path, nil)
	if err != nil {
		return "", false, err
	}
	if status == http.StatusNotFound {
		return "", false, nil
	}
	if status != http.StatusOK {
		return "", false, fmt.Errorf("tuck GET secret/%s: HTTP %d: %s", path, status, body)
	}
	var result struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", false, fmt.Errorf("parse GET secret response: %w", err)
	}
	return result.Value, true, nil
}

// putSecret writes a KV v1 secret.
func (c *tuckClient) putSecret(ctx context.Context, path, value string) error {
	_, status, err := c.doRaw(ctx, http.MethodPut, "/v1/secret/"+path, []byte(value))
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("tuck PUT secret/%s: HTTP %d", path, status)
	}
	return nil
}

// deleteSecret removes a KV v1 secret.
func (c *tuckClient) deleteSecret(ctx context.Context, path string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/secret/"+path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE secret/%s: HTTP %d", path, status)
	}
	return nil
}

// getPolicy reads a policy. Returns (nil, false, nil) if not found.
func (c *tuckClient) getPolicy(ctx context.Context, name string) ([]byte, bool, error) {
	body, status, err := c.doJSON(ctx, http.MethodGet, "/v1/policy/"+name, nil)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("tuck GET policy/%s: HTTP %d: %s", name, status, body)
	}
	return body, true, nil
}

// putPolicy creates or replaces a policy from a JSON rules array string.
func (c *tuckClient) putPolicy(ctx context.Context, name, rulesJSON string) error {
	var rules json.RawMessage
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return fmt.Errorf("rules_json is not valid JSON: %w", err)
	}
	_, status, err := c.doJSON(ctx, http.MethodPut, "/v1/policy/"+name, map[string]any{"rules": rules})
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("tuck PUT policy/%s: HTTP %d", name, status)
	}
	return nil
}

// deletePolicy removes a policy.
func (c *tuckClient) deletePolicy(ctx context.Context, name string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/policy/"+name, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE policy/%s: HTTP %d", name, status)
	}
	return nil
}

// ─── KV v2 ───────────────────────────────────────────────────────────────────

type kvv2ReadResp struct {
	Value   string `json:"value"`
	Version int    `json:"version"`
	Deleted bool   `json:"deleted"`
}

// getKVv2 reads a KV v2 secret. version=0 returns the latest.
func (c *tuckClient) getKVv2(ctx context.Context, path string, version int) (*kvv2ReadResp, bool, error) {
	url := "/v2/secret/" + path
	if version > 0 {
		url += fmt.Sprintf("?version=%d", version)
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("tuck GET v2/secret/%s: HTTP %d: %s", path, status, body)
	}
	var resp kvv2ReadResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, fmt.Errorf("parse KV v2 response: %w", err)
	}
	return &resp, true, nil
}

// putKVv2 writes a KV v2 secret and returns the new version number.
func (c *tuckClient) putKVv2(ctx context.Context, path, value string) (int, error) {
	body, status, err := c.doRaw(ctx, http.MethodPut, "/v2/secret/"+path, []byte(value))
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("tuck PUT v2/secret/%s: HTTP %d: %s", path, status, body)
	}
	var resp struct {
		Version int `json:"version"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.Version, nil
}

// deleteKVv2 soft-deletes the current version of a KV v2 secret.
func (c *tuckClient) deleteKVv2(ctx context.Context, path string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v2/secret/"+path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE v2/secret/%s: HTTP %d", path, status)
	}
	return nil
}

// ─── Token roles ─────────────────────────────────────────────────────────────

type tokenRoleReq struct {
	Policies  []string `json:"policies"`
	TTL       string   `json:"ttl,omitempty"`
	MaxTTL    string   `json:"max_ttl,omitempty"`
	MaxUses   int      `json:"max_uses,omitempty"`
	Renewable bool     `json:"renewable"`
	Period    string   `json:"period,omitempty"`
}

// tokenRoleAPIResp mirrors token.Role JSON (durations are ns int64).
type tokenRoleAPIResp struct {
	Name      string   `json:"name"`
	Policies  []string `json:"policies"`
	TTL       int64    `json:"ttl"`
	MaxTTL    int64    `json:"max_ttl"`
	MaxUses   int      `json:"max_uses"`
	Renewable bool     `json:"renewable"`
	Period    int64    `json:"period"`
}

func (c *tuckClient) getTokenRole(ctx context.Context, name string) (*tokenRoleAPIResp, bool, error) {
	body, status, err := c.doJSON(ctx, http.MethodGet, "/v1/auth/token/roles/"+name, nil)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("tuck GET token role %s: HTTP %d: %s", name, status, body)
	}
	var resp tokenRoleAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, fmt.Errorf("parse token role response: %w", err)
	}
	return &resp, true, nil
}

func (c *tuckClient) putTokenRole(ctx context.Context, name string, req tokenRoleReq) error {
	_, status, err := c.doJSON(ctx, http.MethodPut, "/v1/auth/token/roles/"+name, req)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("tuck PUT token role %s: HTTP %d", name, status)
	}
	return nil
}

func (c *tuckClient) deleteTokenRole(ctx context.Context, name string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/auth/token/roles/"+name, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE token role %s: HTTP %d", name, status)
	}
	return nil
}

// ─── AppRole roles ───────────────────────────────────────────────────────────

type appRoleReq struct {
	Policies        []string `json:"policies"`
	TokenTTL        string   `json:"token_ttl,omitempty"`
	SecretIDTTL     string   `json:"secret_id_ttl,omitempty"`
	SecretIDNumUses int      `json:"secret_id_num_uses,omitempty"`
}

type appRoleAPIResp struct {
	Name            string   `json:"name"`
	RoleID          string   `json:"role_id"`
	Policies        []string `json:"policies"`
	TokenTTL        int64    `json:"token_ttl"`
	SecretIDTTL     int64    `json:"secret_id_ttl"`
	SecretIDNumUses int      `json:"secret_id_num_uses"`
}

func (c *tuckClient) getAppRole(ctx context.Context, name string) (*appRoleAPIResp, bool, error) {
	body, status, err := c.doJSON(ctx, http.MethodGet, "/v1/auth/approle/role/"+name, nil)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("tuck GET approle %s: HTTP %d: %s", name, status, body)
	}
	var resp appRoleAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, fmt.Errorf("parse approle response: %w", err)
	}
	return &resp, true, nil
}

func (c *tuckClient) putAppRole(ctx context.Context, name string, req appRoleReq) (*appRoleAPIResp, error) {
	body, status, err := c.doJSON(ctx, http.MethodPut, "/v1/auth/approle/role/"+name, req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("tuck PUT approle %s: HTTP %d: %s", name, status, body)
	}
	var resp appRoleAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse approle response: %w", err)
	}
	return &resp, nil
}

func (c *tuckClient) deleteAppRole(ctx context.Context, name string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/auth/approle/role/"+name, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE approle %s: HTTP %d", name, status)
	}
	return nil
}

// ─── Namespaces ──────────────────────────────────────────────────────────────

func (c *tuckClient) namespaceExists(ctx context.Context, name string) (bool, error) {
	_, status, err := c.doJSON(ctx, http.MethodGet, "/v1/sys/namespaces/"+name, nil)
	if err != nil {
		return false, err
	}
	if status == http.StatusNotFound {
		return false, nil
	}
	if status != http.StatusOK {
		return false, fmt.Errorf("tuck GET namespace %s: HTTP %d", name, status)
	}
	return true, nil
}

func (c *tuckClient) createNamespace(ctx context.Context, name string) error {
	_, status, err := c.doJSON(ctx, http.MethodPost, "/v1/sys/namespaces", map[string]string{"name": name})
	if err != nil {
		return err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return fmt.Errorf("tuck POST namespace %s: HTTP %d", name, status)
	}
	return nil
}

func (c *tuckClient) deleteNamespace(ctx context.Context, name string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/sys/namespaces/"+name, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE namespace %s: HTTP %d", name, status)
	}
	return nil
}

// nsDuration converts a nanosecond int64 (as returned by the Tuck API for time.Duration fields)
// to a Go duration string. Returns "" for zero.
func nsDuration(ns int64) string {
	if ns == 0 {
		return ""
	}
	return fmt.Sprintf("%s", (time.Duration(ns)).String())
}

// preservePlannedTTLs re-derives a duration through nsDuration always comes
// back in Go's canonical long form ("1h" round-trips as "1h0m0s"), which
// differs byte-for-byte from whatever compact form the user wrote in their
// config. When a plan attribute has a known (non-null, non-unknown) value,
// Terraform requires Create/Update to return that exact same value — not a
// semantically-equal reformatting of it — or it fails the whole apply with
// "Provider produced inconsistent result after apply". So for any TTL the
// user actually set, keep their original string in state instead of the
// server's nanosecond-derived one; only a Computed (unset) TTL takes the
// server-derived value, since there's no planned string to preserve.
func preservePlannedTTLs(defaultTTL, maxTTL *types.String, plannedDefault, plannedMax types.String) {
	if !plannedDefault.IsNull() && !plannedDefault.IsUnknown() {
		*defaultTTL = plannedDefault
	}
	if !plannedMax.IsNull() && !plannedMax.IsUnknown() {
		*maxTTL = plannedMax
	}
}

// preserveEquivalentTTL is Read's counterpart to preservePlannedTTLs: there's
// no plan to defer to during a read, only prior state, so it keeps the prior
// state's string whenever it's semantically the same duration as the
// server's freshly-read one — avoiding a spurious diff on every plan for a
// config that never actually changed (e.g. prior "1h" vs freshly-read
// "1h0m0s").
func preserveEquivalentTTL(fresh string, prior types.String) string {
	if prior.IsNull() || prior.IsUnknown() {
		return fresh
	}
	priorDur, err := time.ParseDuration(prior.ValueString())
	if err != nil {
		return fresh
	}
	freshDur, err := time.ParseDuration(fresh)
	if err != nil {
		return fresh
	}
	if priorDur == freshDur {
		return prior.ValueString()
	}
	return fresh
}

// ─── PKI roles ───────────────────────────────────────────────────────────────

type pkiRoleReq struct {
	AllowedDomains  []string `json:"allowed_domains"`
	AllowSubdomains bool     `json:"allow_subdomains"`
	AllowIPSANs     bool     `json:"allow_ip_sans"`
	AllowLocalhost  bool     `json:"allow_localhost"`
	KeyType         string   `json:"key_type"`
	KeyBits         int      `json:"key_bits,omitempty"`
	DefaultTTL      string   `json:"default_ttl,omitempty"`
	MaxTTL          string   `json:"max_ttl,omitempty"`
	ServerFlag      bool     `json:"server_flag"`
	ClientFlag      bool     `json:"client_flag"`
}

// pkiRoleAPIResp mirrors pki.Role JSON (durations are ns int64).
type pkiRoleAPIResp struct {
	Name            string   `json:"name"`
	AllowedDomains  []string `json:"allowed_domains"`
	AllowSubdomains bool     `json:"allow_subdomains"`
	AllowIPSANs     bool     `json:"allow_ip_sans"`
	AllowLocalhost  bool     `json:"allow_localhost"`
	KeyType         string   `json:"key_type"`
	KeyBits         int      `json:"key_bits"`
	DefaultTTL      int64    `json:"default_ttl"`
	MaxTTL          int64    `json:"max_ttl"`
	ServerFlag      bool     `json:"server_flag"`
	ClientFlag      bool     `json:"client_flag"`
}

func (c *tuckClient) getPKIRole(ctx context.Context, name string) (*pkiRoleAPIResp, bool, error) {
	body, status, err := c.doJSON(ctx, http.MethodGet, "/v1/pki/roles/"+name, nil)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("tuck GET pki role %s: HTTP %d: %s", name, status, body)
	}
	var resp pkiRoleAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, fmt.Errorf("parse pki role response: %w", err)
	}
	return &resp, true, nil
}

func (c *tuckClient) putPKIRole(ctx context.Context, name string, req pkiRoleReq) (*pkiRoleAPIResp, error) {
	body, status, err := c.doJSON(ctx, http.MethodPut, "/v1/pki/roles/"+name, req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("tuck PUT pki role %s: HTTP %d: %s", name, status, body)
	}
	var resp pkiRoleAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse pki role response: %w", err)
	}
	return &resp, nil
}

func (c *tuckClient) deletePKIRole(ctx context.Context, name string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/pki/roles/"+name, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE pki role %s: HTTP %d", name, status)
	}
	return nil
}

// ─── Transit keys ────────────────────────────────────────────────────────────

// transitKeyAPIResp mirrors transit.Key JSON.
type transitKeyAPIResp struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	LatestVersion int    `json:"latest_version"`
	Deletable     bool   `json:"deletable"`
}

func (c *tuckClient) getTransitKey(ctx context.Context, name string) (*transitKeyAPIResp, bool, error) {
	body, status, err := c.doJSON(ctx, http.MethodGet, "/v1/transit/keys/"+name, nil)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("tuck GET transit key %s: HTTP %d: %s", name, status, body)
	}
	var resp transitKeyAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, fmt.Errorf("parse transit key response: %w", err)
	}
	return &resp, true, nil
}

func (c *tuckClient) createTransitKey(ctx context.Context, name, keyType string) (*transitKeyAPIResp, error) {
	body, status, err := c.doJSON(ctx, http.MethodPost, "/v1/transit/keys/"+name, map[string]string{"type": keyType})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("tuck POST transit key %s: HTTP %d: %s", name, status, body)
	}
	var resp transitKeyAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse transit key response: %w", err)
	}
	return &resp, nil
}

// setTransitKeyDeletable flips the key's deletable flag. Transit keys are
// not deletable by default (a deliberate safety guard against accidental
// deletion) — DELETE fails with 409 until this has been called with true.
func (c *tuckClient) setTransitKeyDeletable(ctx context.Context, name string, deletable bool) error {
	body, status, err := c.doJSON(ctx, http.MethodPost, "/v1/transit/keys/"+name+"/config",
		map[string]any{"deletable": deletable})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("tuck POST transit key %s config: HTTP %d: %s", name, status, body)
	}
	return nil
}

func (c *tuckClient) deleteTransitKey(ctx context.Context, name string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/transit/keys/"+name, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE transit key %s: HTTP %d", name, status)
	}
	return nil
}

// ─── SSH roles ───────────────────────────────────────────────────────────────

type sshRoleReq struct {
	AllowedUsers      []string          `json:"allowed_users"`
	DefaultExtensions map[string]string `json:"default_extensions,omitempty"`
	CertType          string            `json:"cert_type,omitempty"`
	DefaultTTL        string            `json:"default_ttl,omitempty"`
	MaxTTL            string            `json:"max_ttl,omitempty"`
}

// sshRoleAPIResp mirrors ssh.Role JSON (durations are ns int64).
type sshRoleAPIResp struct {
	Name              string            `json:"name"`
	AllowedUsers      []string          `json:"allowed_users"`
	DefaultExtensions map[string]string `json:"default_extensions,omitempty"`
	CertType          string            `json:"cert_type"`
	DefaultTTL        int64             `json:"default_ttl"`
	MaxTTL            int64             `json:"max_ttl"`
}

func (c *tuckClient) getSSHRole(ctx context.Context, name string) (*sshRoleAPIResp, bool, error) {
	body, status, err := c.doJSON(ctx, http.MethodGet, "/v1/ssh/roles/"+name, nil)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("tuck GET ssh role %s: HTTP %d: %s", name, status, body)
	}
	var resp sshRoleAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, fmt.Errorf("parse ssh role response: %w", err)
	}
	return &resp, true, nil
}

func (c *tuckClient) putSSHRole(ctx context.Context, name string, req sshRoleReq) (*sshRoleAPIResp, error) {
	body, status, err := c.doJSON(ctx, http.MethodPut, "/v1/ssh/roles/"+name, req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("tuck PUT ssh role %s: HTTP %d: %s", name, status, body)
	}
	var resp sshRoleAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse ssh role response: %w", err)
	}
	return &resp, nil
}

func (c *tuckClient) deleteSSHRole(ctx context.Context, name string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/ssh/roles/"+name, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE ssh role %s: HTTP %d", name, status)
	}
	return nil
}

// ─── Database dynamic secrets ────────────────────────────────────────────────

type dbConnectionReq struct {
	PluginName    string `json:"plugin_name"`
	ConnectionURL string `json:"connection_url"`
	Database      string `json:"database,omitempty"`
	MaxOpenConns  int    `json:"max_open_conns,omitempty"`
}

// dbConnectionAPIResp mirrors database.Config JSON. ConnectionURL comes back
// "[redacted]" from a GET — callers must not use it to overwrite state.
type dbConnectionAPIResp struct {
	Name          string `json:"name"`
	PluginName    string `json:"plugin_name"`
	ConnectionURL string `json:"connection_url"`
	Database      string `json:"database,omitempty"`
	MaxOpenConns  int    `json:"max_open_conns,omitempty"`
}

func (c *tuckClient) getDBConnection(ctx context.Context, name string) (*dbConnectionAPIResp, bool, error) {
	body, status, err := c.doJSON(ctx, http.MethodGet, "/v1/database/config/"+name, nil)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("tuck GET database config %s: HTTP %d: %s", name, status, body)
	}
	var resp dbConnectionAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, fmt.Errorf("parse database config response: %w", err)
	}
	return &resp, true, nil
}

func (c *tuckClient) putDBConnection(ctx context.Context, name string, req dbConnectionReq) (*dbConnectionAPIResp, error) {
	body, status, err := c.doJSON(ctx, http.MethodPut, "/v1/database/config/"+name, req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("tuck PUT database config %s: HTTP %d: %s", name, status, body)
	}
	var resp dbConnectionAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse database config response: %w", err)
	}
	return &resp, nil
}

func (c *tuckClient) deleteDBConnection(ctx context.Context, name string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/database/config/"+name, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE database config %s: HTTP %d", name, status)
	}
	return nil
}

type dbRoleReq struct {
	DBName               string `json:"db_name"`
	CreationStatements   string `json:"creation_statements"`
	RevocationStatements string `json:"revocation_statements,omitempty"`
	DefaultTTL           string `json:"default_ttl,omitempty"`
	MaxTTL               string `json:"max_ttl,omitempty"`
}

// dbRoleAPIResp mirrors database.Role JSON (durations are ns int64).
type dbRoleAPIResp struct {
	Name                 string `json:"name"`
	DBName               string `json:"db_name"`
	CreationStatements   string `json:"creation_statements"`
	RevocationStatements string `json:"revocation_statements"`
	DefaultTTL           int64  `json:"default_ttl"`
	MaxTTL               int64  `json:"max_ttl"`
}

func (c *tuckClient) getDBRole(ctx context.Context, name string) (*dbRoleAPIResp, bool, error) {
	body, status, err := c.doJSON(ctx, http.MethodGet, "/v1/database/role/"+name, nil)
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("tuck GET database role %s: HTTP %d: %s", name, status, body)
	}
	var resp dbRoleAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, false, fmt.Errorf("parse database role response: %w", err)
	}
	return &resp, true, nil
}

func (c *tuckClient) putDBRole(ctx context.Context, name string, req dbRoleReq) (*dbRoleAPIResp, error) {
	body, status, err := c.doJSON(ctx, http.MethodPut, "/v1/database/role/"+name, req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("tuck PUT database role %s: HTTP %d: %s", name, status, body)
	}
	var resp dbRoleAPIResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse database role response: %w", err)
	}
	return &resp, nil
}

func (c *tuckClient) deleteDBRole(ctx context.Context, name string) error {
	_, status, err := c.doJSON(ctx, http.MethodDelete, "/v1/database/role/"+name, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusNotFound {
		return fmt.Errorf("tuck DELETE database role %s: HTTP %d", name, status)
	}
	return nil
}
