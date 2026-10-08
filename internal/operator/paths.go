package operator

import (
	"context"
	"fmt"
	"strings"

	"github.com/NAGenaev/tuck/internal/namespace"
)

// AnnotationAllowedPaths is read from the Namespace object. Its value is a
// comma-separated list of extra Tuck path prefixes that TuckSecrets in that
// namespace may read, in addition to the default "<namespace>/". Only
// principals able to edit Namespace objects (cluster admins) can set it.
const AnnotationAllowedPaths = "tuck.io/allowed-paths"

// AnnotationTuckNamespace is read from the Namespace object. When set, every
// TuckSecret in that Kubernetes namespace reads from the named Tuck namespace
// (the operator sends it as X-Tuck-Namespace) and may use any path inside it:
// the Tuck namespace is the isolation boundary, so the per-namespace path
// prefix no longer applies. Only cluster admins can set it. It is honoured
// only with --enforce-namespace-paths, the mode that trusts Namespace objects.
const AnnotationTuckNamespace = "tuck.io/tuck-namespace"

type tuckNamespaceKey struct{}

// withTuckNamespace makes the Tuck client read from namespace ns.
func withTuckNamespace(ctx context.Context, ns string) context.Context {
	return context.WithValue(ctx, tuckNamespaceKey{}, ns)
}

func tuckNamespaceFrom(ctx context.Context) string {
	ns, _ := ctx.Value(tuckNamespaceKey{}).(string)
	return ns
}

// normalizePath validates a Tuck KV path and returns it without a leading
// slash. It rejects anything the HTTP stack could rewrite or reinterpret
// (dot segments, empty segments, query/fragment/escape characters), because a
// request for "team-a/../team-b/x" would otherwise be cleaned by the server
// after our prefix check has passed.
func normalizePath(p string) (string, error) {
	p = strings.TrimLeft(p, "/")
	if p == "" {
		return "", fmt.Errorf("empty tuck path")
	}
	for _, r := range p {
		switch {
		case r < 0x20 || r == 0x7f, r == ' ', r == '?', r == '#', r == '%', r == '\\':
			return "", fmt.Errorf("tuck path %q contains a forbidden character", p)
		}
	}
	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "":
			return "", fmt.Errorf("tuck path %q contains an empty segment", p)
		case ".", "..":
			return "", fmt.Errorf("tuck path %q contains a %q segment", p, seg)
		}
	}
	return p, nil
}

// allowedPrefixes returns the Tuck path prefixes a namespace may read.
func allowedPrefixes(namespace string, nsAnnotations map[string]string) ([]string, error) {
	prefixes := []string{namespace}
	raw := strings.TrimSpace(nsAnnotations[AnnotationAllowedPaths])
	if raw == "" {
		return prefixes, nil
	}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		n, err := normalizePath(strings.TrimRight(item, "/"))
		if err != nil {
			return nil, fmt.Errorf("annotation %s: %w", AnnotationAllowedPaths, err)
		}
		prefixes = append(prefixes, n)
	}
	return prefixes, nil
}

// pathAllowed reports whether the normalized path equals a prefix or lies
// below it on a segment boundary ("team-a" allows "team-a/db" but not
// "team-ab/db").
func pathAllowed(path string, prefixes []string) bool {
	for _, pre := range prefixes {
		if path == pre || strings.HasPrefix(path, pre+"/") {
			return true
		}
	}
	return false
}

// tuckNamespaceOf returns the validated Tuck namespace the Kubernetes
// namespace is bound to, or "" when it is not bound.
func tuckNamespaceOf(nsAnnotations map[string]string) (string, error) {
	v := strings.TrimSpace(nsAnnotations[AnnotationTuckNamespace])
	if v == "" {
		return "", nil
	}
	if err := namespace.ValidateName(v); err != nil {
		return "", fmt.Errorf("annotation %s: %w", AnnotationTuckNamespace, err)
	}
	return v, nil
}
