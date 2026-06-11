package k8s

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const reviewTimeout = 10 * time.Second

// Client calls the Kubernetes TokenReview API over HTTPS.
type Client struct {
	apiURL     string
	token      string // static bearer token; ignored when tokenFile is set
	tokenFile  string // if set, read per-Review call to handle projected token rotation
	httpClient *http.Client
}

// NewClient builds a Client with a static bearer token.
func NewClient(apiURL, token, caFile string) (*Client, error) {
	c, err := newClient(apiURL, caFile)
	if err != nil {
		return nil, err
	}
	c.token = token
	return c, nil
}

// NewClientFromFiles builds a Client that reads its own bearer token from
// tokenFile on every Review call, transparently handling Kubernetes projected
// token rotation.
func NewClientFromFiles(apiURL, tokenFile, caFile string) (*Client, error) {
	c, err := newClient(apiURL, caFile)
	if err != nil {
		return nil, err
	}
	c.tokenFile = tokenFile
	return c, nil
}

func newClient(apiURL, caFile string) (*Client, error) {
	tlsCfg := &tls.Config{}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("no valid certificates found in CA file")
		}
		tlsCfg.RootCAs = pool
	}
	return &Client{
		apiURL: apiURL,
		httpClient: &http.Client{
			Timeout:   reviewTimeout,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

func (c *Client) bearerToken() (string, error) {
	if c.tokenFile != "" {
		b, err := os.ReadFile(c.tokenFile)
		if err != nil {
			return "", fmt.Errorf("read k8s token file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return c.token, nil
}

// Review submits a TokenReview to the Kubernetes API and returns the result.
func (c *Client) Review(saToken string) (*ReviewResult, error) {
	body, err := json.Marshal(map[string]any{
		"apiVersion": "authentication.k8s.io/v1",
		"kind":       "TokenReview",
		"spec":       map[string]string{"token": saToken},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal TokenReview: %w", err)
	}

	url := strings.TrimRight(c.apiURL, "/") + "/apis/authentication.k8s.io/v1/tokenreviews"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	bearer, err := c.bearerToken()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TokenReview request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("TokenReview: unexpected status %d", resp.StatusCode)
	}

	var tr struct {
		Status struct {
			Authenticated bool   `json:"authenticated"`
			Error         string `json:"error"`
			User          struct {
				Username string   `json:"username"`
				UID      string   `json:"uid"`
				Groups   []string `json:"groups"`
			} `json:"user"`
		} `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decode TokenReview response: %w", err)
	}

	if !tr.Status.Authenticated || tr.Status.Error != "" {
		return &ReviewResult{Authenticated: false}, nil
	}
	return &ReviewResult{
		Authenticated: true,
		Username:      tr.Status.User.Username,
		UID:           tr.Status.User.UID,
		Groups:        tr.Status.User.Groups,
	}, nil
}

// ParseUsername extracts namespace and serviceaccount from
// "system:serviceaccount:<namespace>:<sa>"
func ParseUsername(username string) (namespace, sa string, err error) {
	const prefix = "system:serviceaccount:"
	if !strings.HasPrefix(username, prefix) {
		return "", "", fmt.Errorf("ParseUsername: unexpected format %q", username)
	}
	rest := username[len(prefix):]
	idx := strings.IndexByte(rest, ':')
	if idx < 0 {
		return "", "", fmt.Errorf("ParseUsername: missing serviceaccount in %q", username)
	}
	ns := rest[:idx]
	account := rest[idx+1:]
	if ns == "" || account == "" {
		return "", "", fmt.Errorf("ParseUsername: empty namespace or serviceaccount in %q", username)
	}
	return ns, account, nil
}
