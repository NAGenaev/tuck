package operator

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// KubeClient talks to the Kubernetes API over HTTPS.
type KubeClient struct {
	apiURL     string
	tokenFile  string
	httpClient *http.Client
}

func NewKubeClient(apiURL, tokenFile, caFile string) (*KubeClient, error) {
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
	return &KubeClient{
		apiURL:    strings.TrimRight(apiURL, "/"),
		tokenFile: tokenFile,
		// No global timeout: Watch uses long-lived connections.
		httpClient: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

func (c *KubeClient) bearerToken() (string, error) {
	b, err := os.ReadFile(c.tokenFile)
	if err != nil {
		return "", fmt.Errorf("read k8s token file: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

func (c *KubeClient) newReq(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	token, err := c.bearerToken()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// List returns all TuckSecret resources in namespace (empty = all namespaces).
func (c *KubeClient) List(ctx context.Context, namespace string) (*TuckSecretList, error) {
	url := c.tucksecretURL(namespace)
	req, err := c.newReq(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list TuckSecrets: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list TuckSecrets: unexpected status %d", resp.StatusCode)
	}
	var list TuckSecretList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode TuckSecretList: %w", err)
	}
	return &list, nil
}

// Watch opens a long-lived HTTP connection and streams WatchEvents into the
// returned channel. The channel is closed when ctx is cancelled or the
// connection drops. resourceVersion is the version to watch from ("" = now).
func (c *KubeClient) Watch(ctx context.Context, namespace, resourceVersion string) (<-chan WatchEvent, error) {
	url := c.tucksecretURL(namespace) + "?watch=true&allowWatchBookmarks=true"
	if resourceVersion != "" {
		url += "&resourceVersion=" + resourceVersion
	}
	req, err := c.newReq(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("watch TuckSecrets: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("watch TuckSecrets: unexpected status %d", resp.StatusCode)
	}

	ch := make(chan WatchEvent)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			var event WatchEvent
			if err := json.Unmarshal(line, &event); err != nil {
				continue
			}
			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// ApplySecret creates or updates a K8s Secret using GET then POST or PUT.
func (c *KubeClient) ApplySecret(ctx context.Context, secret *KubeSecret) error {
	ns := secret.Metadata.Namespace
	name := secret.Metadata.Name
	getURL := fmt.Sprintf("%s/api/v1/namespaces/%s/secrets/%s", c.apiURL, ns, name)

	getReq, err := c.newReq(ctx, http.MethodGet, getURL, nil)
	if err != nil {
		return err
	}
	getResp, err := c.httpClient.Do(getReq)
	if err != nil {
		return fmt.Errorf("get secret %s/%s: %w", ns, name, err)
	}
	defer getResp.Body.Close()

	body, err := json.Marshal(secret)
	if err != nil {
		return fmt.Errorf("marshal secret: %w", err)
	}

	var method, applyURL string
	if getResp.StatusCode == http.StatusNotFound {
		method = http.MethodPost
		applyURL = fmt.Sprintf("%s/api/v1/namespaces/%s/secrets", c.apiURL, ns)
	} else if getResp.StatusCode >= 200 && getResp.StatusCode < 300 {
		// Carry over the resourceVersion so the PUT satisfies optimistic concurrency.
		var existing KubeSecret
		if err := json.NewDecoder(getResp.Body).Decode(&existing); err != nil {
			return fmt.Errorf("decode existing secret: %w", err)
		}
		secret.Metadata.ResourceVersion = existing.Metadata.ResourceVersion
		body, err = json.Marshal(secret)
		if err != nil {
			return fmt.Errorf("marshal secret: %w", err)
		}
		method = http.MethodPut
		applyURL = getURL
	} else {
		return fmt.Errorf("get secret %s/%s: unexpected status %d", ns, name, getResp.StatusCode)
	}

	applyReq, err := c.newReq(ctx, method, applyURL, body)
	if err != nil {
		return err
	}
	applyResp, err := c.httpClient.Do(applyReq)
	if err != nil {
		return fmt.Errorf("apply secret %s/%s: %w", ns, name, err)
	}
	defer applyResp.Body.Close()
	if applyResp.StatusCode < 200 || applyResp.StatusCode >= 300 {
		return fmt.Errorf("apply secret %s/%s: unexpected status %d", ns, name, applyResp.StatusCode)
	}
	return nil
}

func (c *KubeClient) tucksecretURL(namespace string) string {
	if namespace == "" {
		return c.apiURL + "/apis/tuck.io/v1alpha1/tucksecrets"
	}
	return fmt.Sprintf("%s/apis/tuck.io/v1alpha1/namespaces/%s/tucksecrets", c.apiURL, namespace)
}
