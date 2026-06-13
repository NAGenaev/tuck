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
	defer resp.Body.Close()
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
