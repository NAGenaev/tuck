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
	Namespace      string        `json:"namespace"`
	ServiceAccount string        `json:"service_account"`
	Policies       []string      `json:"policies"`
	TTL            time.Duration `json:"ttl"` // 0 = token never expires
}
