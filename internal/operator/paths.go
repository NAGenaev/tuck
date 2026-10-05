package operator

import (
	"fmt"
	"strings"
)

// AnnotationAllowedPaths is read from the Namespace object. Its value is a
// comma-separated list of extra Tuck path prefixes that TuckSecrets in that
// namespace may read, in addition to the default "<namespace>/". Only
// principals able to edit Namespace objects (cluster admins) can set it.
const AnnotationAllowedPaths = "tuck.io/allowed-paths"

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
