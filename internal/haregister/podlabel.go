// Package haregister publishes this node's Raft leadership as a Kubernetes pod
// label, so a Service can select only the active node. Followers refuse writes
// (and Tuck logins are writes), so traffic must reach the leader.
package haregister

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/tlsutil"
)

// LeaderLabel is the pod label set to "true" on the leader and "false" elsewhere.
const LeaderLabel = "tuck.io/leader"

const (
	saDir     = "/var/run/secrets/kubernetes.io/serviceaccount"
	patchWait = 5 * time.Second
)

// PodLabeler patches the labels of one pod through the Kubernetes API.
type PodLabeler struct {
	API       string // https://host:port
	TokenFile string // re-read on every call (projected tokens rotate)
	CAFile    string
	Namespace string
	Pod       string

	client *http.Client
}

// InCluster builds a PodLabeler from the pod's own environment and mounted
// ServiceAccount.
func InCluster(namespace, pod string) (*PodLabeler, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, fmt.Errorf("not running in a Kubernetes pod (KUBERNETES_SERVICE_HOST/PORT unset)")
	}
	if namespace == "" || pod == "" {
		return nil, fmt.Errorf("pod namespace and name are required (set POD_NAMESPACE and POD_NAME)")
	}
	api := "https://" + hostPort(host, port)
	return NewPodLabeler(api, saDir+"/token", saDir+"/ca.crt", namespace, pod)
}

// NewPodLabeler builds a PodLabeler that verifies the API server against caFile.
func NewPodLabeler(api, tokenFile, caFile, namespace, pod string) (*PodLabeler, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if caFile != "" {
		pool, err := tlsutil.LoadCAPool(caFile)
		if err != nil {
			return nil, fmt.Errorf("kubernetes CA: %w", err)
		}
		tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &PodLabeler{
		API: strings.TrimRight(api, "/"), TokenFile: tokenFile, CAFile: caFile,
		Namespace: namespace, Pod: pod,
		client: &http.Client{Transport: tr, Timeout: patchWait},
	}, nil
}

func hostPort(host, port string) string {
	if strings.Contains(host, ":") { // IPv6 literal
		host = "[" + host + "]"
	}
	return host + ":" + port
}

// SetLeader sets the leader label on the pod.
func (l *PodLabeler) SetLeader(ctx context.Context, leader bool) error {
	tok, err := os.ReadFile(l.TokenFile) // #nosec G304 — operator-configured token path
	if err != nil {
		return fmt.Errorf("read service account token: %w", err)
	}
	val := "false"
	if leader {
		val = "true"
	}
	body := fmt.Sprintf(`{"metadata":{"labels":{%q:%q}}}`, LeaderLabel, val)
	u := fmt.Sprintf("%s/api/v1/namespaces/%s/pods/%s", l.API, url.PathEscape(l.Namespace), url.PathEscape(l.Pod))
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tok)))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("patch pod label: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

// Run keeps the label equal to isLeader() until ctx is cancelled. It patches on
// every change and re-asserts the current value every resync, so a label lost
// to a failed call or an external edit heals itself. On exit it clears the
// label so a stopping leader leaves the Service promptly.
func (l *PodLabeler) Run(ctx context.Context, isLeader func() bool, poll, resync time.Duration, logf func(string, ...any)) {
	var (
		last     bool
		haveLast bool
		lastSent time.Time
	)
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		cur := isLeader()
		if !haveLast || cur != last || time.Since(lastSent) >= resync {
			if err := l.SetLeader(ctx, cur); err != nil {
				if ctx.Err() == nil {
					logf("tuck: leader label: %v", err)
				}
			} else {
				if !haveLast || cur != last {
					logf("tuck: leader label %s=%v", LeaderLabel, cur)
				}
				last, haveLast, lastSent = cur, true, time.Now()
			}
		}
		select {
		case <-ctx.Done():
			clr, cancel := context.WithTimeout(context.Background(), patchWait)
			_ = l.SetLeader(clr, false)
			cancel()
			return
		case <-tick.C:
		}
	}
}
