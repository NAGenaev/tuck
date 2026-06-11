package operator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	tuckTokenTTL    = 4 * time.Minute
	tuckTokenMargin = 30 * time.Second
)

// TuckClient authenticates to Tuck and fetches secrets.
type TuckClient struct {
	serverURL   string
	saTokenFile string // operator's k8s SA token used to log in to Tuck
	httpClient  *http.Client

	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

func NewTuckClient(serverURL, saTokenFile string) *TuckClient {
	return &TuckClient{
		serverURL:   strings.TrimRight(serverURL, "/"),
		saTokenFile: saTokenFile,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// GetSecret fetches the secret at path, refreshing the Tuck token if needed.
func (c *TuckClient) GetSecret(ctx context.Context, path string) ([]byte, error) {
	token, err := c.ensureToken(ctx)
	if err != nil {
		return nil, err
	}

	url := c.serverURL + "/v1/secret/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build secret request: %w", err)
	}
	req.Header.Set("X-Tuck-Token", token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get secret %q: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get secret %q: unexpected status %d", path, resp.StatusCode)
	}

	// Tuck returns {"path":"...","value":"<string>"} — value is a JSON string,
	// not base64-encoded bytes.
	var result struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode secret response: %w", err)
	}
	return []byte(result.Value), nil
}

// ensureToken returns a valid cached token, refreshing if it expires within
// tuckTokenMargin.
func (c *TuckClient) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedToken != "" && time.Now().Add(tuckTokenMargin).Before(c.tokenExpiry) {
		return c.cachedToken, nil
	}
	return c.login(ctx)
}

// login exchanges the SA token for a Tuck token and caches it.
// Must be called with c.mu held.
func (c *TuckClient) login(ctx context.Context) (string, error) {
	saJWT, err := os.ReadFile(c.saTokenFile)
	if err != nil {
		return "", fmt.Errorf("read SA token file: %w", err)
	}

	body, err := json.Marshal(map[string]string{"token": strings.TrimSpace(string(saJWT))})
	if err != nil {
		return "", fmt.Errorf("marshal login request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+"/v1/auth/kubernetes/login", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tuck login: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("tuck login: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("tuck login: empty token in response")
	}

	c.cachedToken = result.Token
	c.tokenExpiry = time.Now().Add(tuckTokenTTL)
	return c.cachedToken, nil
}
