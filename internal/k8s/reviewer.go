// Package k8s implements Kubernetes TokenReview-based authentication for Tuck.
package k8s

import "time"

// ReviewResult is the verified identity from a successful TokenReview.
type ReviewResult struct {
	Authenticated bool
	Username      string // e.g. "system:serviceaccount:mynamespace:mysa"
	UID           string
	Groups        []string
}

// Reviewer validates a Kubernetes ServiceAccount JWT via the TokenReview API.
type Reviewer interface {
	Review(token string) (*ReviewResult, error)
}

// K8sRole maps a Kubernetes ServiceAccount to a set of Tuck policies.
type K8sRole struct {
	// Cluster names the trusted cluster the ServiceAccount lives in. Empty =
	// the default cluster (--k8s-api).
	Cluster        string        `json:"cluster,omitempty"`
	Namespace      string        `json:"namespace"`
	ServiceAccount string        `json:"service_account"`
	Policies       []string      `json:"policies"`
	TTL            time.Duration `json:"ttl"` // 0 = falls back to a 1h token TTL (see Core.LoginK8s)
	// TuckNamespace binds issued tokens to this Tuck namespace (must exist).
	// Empty = root namespace. Never taken from the client.
	TuckNamespace string `json:"tuck_namespace,omitempty"`
}

// ValidClusterName reports whether name is usable as a cluster identifier
// (it becomes part of a storage key and a URL path segment).
func ValidClusterName(name string) bool {
	if name == "" || len(name) > 63 {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case i > 0 && (r == '-' || r == '_' || r == '.'):
		default:
			return false
		}
	}
	return true
}
