package core

import (
	"context"

	"github.com/NAGenaev/tuck/internal/kvv2"
	"github.com/NAGenaev/tuck/internal/policy"
)

// KV2MountPath is the mount whose tuning config carries the KV v2 options
// (max_versions, cas_required). KV v2 is served under secret/.
const KV2MountPath = "secret/"

// kv2MountDefaults loads the mount-level KV v2 options from the per-mount
// tuning config of KV2MountPath. A read failure yields no overrides.
func (c *Core) kv2MountDefaults(ctx context.Context) kvv2.Defaults {
	cfg, err := c.mountConfigStore.Get(ctx, KV2MountPath)
	if err != nil {
		return kvv2.Defaults{}
	}
	return kvv2.Defaults{MaxVersions: cfg.MaxVersions, CASRequired: cfg.CASRequired}
}

// CapabilityChecker returns a function that reports whether the token holds
// cap on a logical path, resolving the token's policies once. It is meant for
// filtering many paths in one request (enforcement per path would reload the
// policies every time).
func (c *Core) CapabilityChecker(ctx context.Context, tokenID string, cap policy.Capability) (func(logicalPath string) bool, error) {
	tok, err := c.Authenticate(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	policies, err := c.resolvePolicies(ctx, tok.Namespace, tok.Policies)
	if err != nil {
		return func(string) bool { return false }, nil
	}
	return func(p string) bool { return policy.Allowed(policies, p, cap) }, nil
}
