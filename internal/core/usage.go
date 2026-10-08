package core

import (
	"context"
	"sync"
	"time"

	"github.com/NAGenaev/tuck/internal/namespace"
	"github.com/NAGenaev/tuck/internal/token"
)

// UsageCacheTTL is how long usage counters are reused. The counters back
// dashboards that poll, so a few seconds of staleness is fine and keeps the
// scans off the hot path.
const UsageCacheTTL = 15 * time.Second

// maxRoleScan bounds how many auth roles of one type are inspected when
// counting roles per namespace.
const maxRoleScan = 2000

type usageEntry struct {
	at  time.Time
	val any
}

// usageCache is a tiny TTL cache keyed by string. Its zero value is ready.
type usageCache struct {
	mu sync.Mutex
	m  map[string]usageEntry
}

// do returns the cached value for key or computes it with load. Concurrent
// misses may both compute; the last write wins, which is harmless for
// read-only counters.
func (u *usageCache) do(key string, load func() (any, error)) (any, error) {
	u.mu.Lock()
	if e, ok := u.m[key]; ok && time.Since(e.at) < UsageCacheTTL {
		u.mu.Unlock()
		return e.val, nil
	}
	u.mu.Unlock()
	v, err := load()
	if err != nil {
		return nil, err
	}
	u.mu.Lock()
	if u.m == nil || len(u.m) > 256 {
		u.m = make(map[string]usageEntry)
	}
	u.m[key] = usageEntry{at: time.Now(), val: v}
	u.mu.Unlock()
	return v, nil
}

// nsKey normalises a namespace name: the root namespace is "".
func nsKey(ns string) string {
	if ns == namespace.RootNamespace {
		return ""
	}
	return ns
}

// SecretCounts returns the number of KV v1 secrets and KV v2 secret paths in
// the namespace. It lists keys only (nothing is decrypted beyond key names).
func (c *Core) SecretCounts(ctx context.Context, ns string) (kv1, kv2 int, err error) {
	ns = nsKey(ns)
	type pair struct{ v1, v2 int }
	v, err := c.usage.do("secrets:"+ns, func() (any, error) {
		keys, err := c.barrier.List(ctx, namespace.StoragePrefix(ns)+secretPrefix)
		if err != nil {
			return nil, err
		}
		n2, err := c.KVv2(ns).CountSecrets(ctx)
		if err != nil {
			return nil, err
		}
		return pair{len(keys), n2}, nil
	})
	if err != nil {
		return 0, 0, err
	}
	p := v.(pair)
	return p.v1, p.v2, nil
}

// TokenCounts returns the number of live tokens per namespace ("" is root).
// truncated is true when the scan limit was hit.
func (c *Core) TokenCounts(ctx context.Context) (counts map[string]int, truncated bool, err error) {
	type res struct {
		m   map[string]int
		trc bool
	}
	v, err := c.usage.do("tokens", func() (any, error) {
		m, trc, err := c.tokens.CountByNamespace(ctx, token.DefaultCountLimit)
		if err != nil {
			return nil, err
		}
		return res{m, trc}, nil
	})
	if err != nil {
		return nil, false, err
	}
	r := v.(res)
	return r.m, r.trc, nil
}

// AuthRoleCounts returns the number of AppRole, JWT and LDAP roles bound to
// each Tuck namespace ("" is root). Roles are global objects; the binding is
// their tuck_namespace. Kubernetes roles are not counted (their store cannot
// be enumerated).
func (c *Core) AuthRoleCounts(ctx context.Context) (map[string]int, error) {
	v, err := c.usage.do("authroles", func() (any, error) {
		out := make(map[string]int)
		add := func(ns string) { out[nsKey(ns)]++ }

		if names, err := c.approleStore.ListRoles(ctx); err == nil {
			for i, n := range names {
				if i >= maxRoleScan {
					break
				}
				if r, err := c.approleStore.GetRole(ctx, n); err == nil {
					add(r.TuckNamespace)
				}
			}
		}
		if names, err := c.jwtStore.ListRoles(ctx); err == nil {
			for i, n := range names {
				if i >= maxRoleScan {
					break
				}
				if r, err := c.jwtStore.GetRole(ctx, n); err == nil {
					add(r.TuckNamespace)
				}
			}
		}
		if names, err := c.ldapStore.ListRoles(ctx); err == nil {
			for i, n := range names {
				if i >= maxRoleScan {
					break
				}
				if r, err := c.ldapStore.GetRole(ctx, n); err == nil {
					add(r.TuckNamespace)
				}
			}
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(map[string]int), nil
}

// NamespaceCounts are the raw (unfiltered) object counts of one namespace.
// The API layer decides which of them a caller may see.
type NamespaceCounts struct {
	SecretsKV1 int  `json:"secrets_kv1"`
	SecretsKV2 int  `json:"secrets_kv2"`
	Tokens     int  `json:"tokens"`
	Policies   int  `json:"policies"`
	AuthRoles  int  `json:"auth_roles"`
	Truncated  bool `json:"truncated,omitempty"`
}

// NamespaceCounts counts the objects stored in or bound to ns ("" or "root"
// is the root namespace). Results are cached for UsageCacheTTL.
func (c *Core) NamespaceCounts(ctx context.Context, ns string) (NamespaceCounts, error) {
	ns = nsKey(ns)
	var out NamespaceCounts
	var err error
	if out.SecretsKV1, out.SecretsKV2, err = c.SecretCounts(ctx, ns); err != nil {
		return NamespaceCounts{}, err
	}
	tok, trc, err := c.TokenCounts(ctx)
	if err != nil {
		return NamespaceCounts{}, err
	}
	out.Tokens, out.Truncated = tok[ns], trc
	roles, err := c.AuthRoleCounts(ctx)
	if err != nil {
		return NamespaceCounts{}, err
	}
	out.AuthRoles = roles[ns]
	pol, err := c.usage.do("policies:"+ns, func() (any, error) {
		names, err := c.Policies(ns).List(ctx)
		return len(names), err
	})
	if err != nil {
		return NamespaceCounts{}, err
	}
	out.Policies = pol.(int)
	return out, nil
}
