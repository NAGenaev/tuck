package csi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// authToken (default) uses the static Tuck token from nodePublishSecretRef.
	authToken = "token"
	// authKubernetes logs in to Tuck with the pod's ServiceAccount token that
	// kubelet delivers through the CSIDriver tokenRequests mechanism.
	authKubernetes = "kubernetes"

	ctxAuth        = "tuck.io/auth"         // "token" (default) or "kubernetes"
	ctxK8sAudience = "tuck.io/k8s-audience" // audience key in serviceAccount.tokens; "" = API server default
	ctxK8sCluster  = "tuck.io/k8s-cluster"  // trusted cluster name on the Tuck server; "" = default

	// volumeKeySATokens is set by kubelet when the CSIDriver declares
	// tokenRequests: a JSON object audience → {token, expirationTimestamp}.
	volumeKeySATokens = "csi.storage.k8s.io/serviceAccount.tokens"

	// saTokenSkew: a ServiceAccount token this close to expiry is treated as expired.
	saTokenSkew = 10 * time.Second
)

// httpStatusError is returned for a non-200 answer from Tuck so callers can
// tell an expired Tuck token (401/403) from other failures.
type httpStatusError struct {
	code int
	body string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("tuck API returned HTTP %d: %s", e.code, e.body)
}

func (e *httpStatusError) authRejected() bool {
	return e.code == http.StatusUnauthorized || e.code == http.StatusForbidden
}

// saToken is a pod ServiceAccount token with the time kubelet says it expires.
type saToken struct {
	token   string
	expires time.Time // zero = unknown
}

func (s saToken) usable(now time.Time) bool {
	if s.token == "" {
		return false
	}
	return s.expires.IsZero() || now.Add(saTokenSkew).Before(s.expires)
}

// serviceAccountToken extracts the pod's ServiceAccount token for audience.
// Kubelet delivers tokens in the request secrets when the CSIDriver sets
// serviceAccountTokenInSecrets (so they stay out of volume-context logs) and
// in the volume context otherwise. When audience is empty and exactly one
// token is present, it is used regardless of its audience key.
func serviceAccountToken(vc, secrets map[string]string, audience string) (saToken, error) {
	raw := secrets[volumeKeySATokens]
	if raw == "" {
		raw = vc[volumeKeySATokens]
	}
	if raw == "" {
		return saToken{}, fmt.Errorf("kubelet passed no ServiceAccount token: the CSIDriver must declare tokenRequests (and requiresRepublish for rotation)")
	}
	var all map[string]struct {
		Token               string `json:"token"`
		ExpirationTimestamp string `json:"expirationTimestamp"`
	}
	if err := json.Unmarshal([]byte(raw), &all); err != nil {
		return saToken{}, fmt.Errorf("parse %s: %w", volumeKeySATokens, err)
	}
	e, ok := all[audience]
	if !ok && audience == "" && len(all) == 1 {
		for _, v := range all {
			e, ok = v, true
		}
	}
	if !ok || e.Token == "" {
		return saToken{}, fmt.Errorf("no ServiceAccount token for audience %q (set tuck.io/k8s-audience to match CSIDriver tokenRequests)", audience)
	}
	out := saToken{token: e.Token}
	if e.ExpirationTimestamp != "" {
		if t, err := time.Parse(time.RFC3339, e.ExpirationTimestamp); err == nil {
			out.expires = t
		}
	}
	return out, nil
}

// loginKubernetes exchanges a ServiceAccount token for a Tuck token.
func loginKubernetes(ctx context.Context, hc *http.Client, addr, cluster, sa string) (string, error) {
	body := map[string]string{"token": sa}
	if cluster != "" {
		body["cluster"] = cluster
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr+"/v1/auth/kubernetes/login", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kubernetes login: %w", &httpStatusError{code: resp.StatusCode, body: string(data)})
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.Token == "" {
		return "", fmt.Errorf("kubernetes login: response has no token")
	}
	return out.Token, nil
}
