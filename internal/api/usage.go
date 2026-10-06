package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/mount"
	"github.com/NAGenaev/tuck/internal/namespace"
	"github.com/NAGenaev/tuck/internal/policy"
)

// Usage counters. Every counter is read-only, bounded and cached for a few
// seconds in core. A counter is only included when the caller could list the
// objects it counts; the field is then absent (never zero), so a client cannot
// confuse "not permitted" with "empty".

func intp(n int) *int { return &n }

// boundNamespace returns the namespace the calling token is bound to ("" when
// it is a root-namespace token that may look at any namespace).
func (s *Server) boundNamespace(ctx context.Context) (string, error) {
	tok, err := s.core.Authenticate(ctx, tokenFromCtx(ctx))
	if err != nil {
		return "", err
	}
	if tok.Namespace == namespace.RootNamespace {
		return "", nil
	}
	return tok.Namespace, nil
}

// mayViewNamespace reports whether the caller may look at namespace name: a
// token bound to a namespace is confined to its own.
func (s *Server) mayViewNamespace(ctx context.Context, name string) (bool, error) {
	bound, err := s.boundNamespace(ctx)
	if err != nil {
		return false, err
	}
	return bound == "" || bound == name, nil
}

// nsStats are the object counts of one namespace.
type nsStats struct {
	Secrets    *int `json:"secrets,omitempty"` // KV v1 + KV v2
	SecretsKV1 *int `json:"secrets_kv1,omitempty"`
	SecretsKV2 *int `json:"secrets_kv2,omitempty"`
	Tokens     *int `json:"tokens,omitempty"`
	Policies   *int `json:"policies,omitempty"`
	AuthRoles  *int `json:"auth_roles,omitempty"`
	// Truncated is set when a scan limit cut the token count short.
	Truncated bool `json:"truncated,omitempty"`
}

// namespaceStats counts the objects of namespace ns, keeping only what the
// caller may list.
func (s *Server) namespaceStats(ctx context.Context, ns string) (nsStats, error) {
	canList, err := s.core.CapabilityChecker(ctx, tokenFromCtx(ctx), policy.CapList)
	if err != nil {
		return nsStats{}, err
	}
	c, err := s.core.NamespaceCounts(ctx, ns)
	if err != nil {
		return nsStats{}, err
	}
	var out nsStats
	if canList("secret/") {
		out.SecretsKV1, out.SecretsKV2 = intp(c.SecretsKV1), intp(c.SecretsKV2)
		out.Secrets = intp(c.SecretsKV1 + c.SecretsKV2)
	}
	if canList("auth/token") {
		out.Tokens = intp(c.Tokens)
		out.Truncated = c.Truncated
	}
	if canList("auth/policy") {
		out.Policies = intp(c.Policies)
	}
	if canList("auth/approle/role") && canList("auth/jwt/role") && canList("auth/ldap/role") {
		out.AuthRoles = intp(c.AuthRoles)
	}
	return out, nil
}

// GET /v1/sys/namespaces/{name}/stats
func (s *Server) namespaceStatsHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/namespaces/"+name, policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	ok, err := s.mayViewNamespace(r.Context(), name)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		writeErr(w, core.ErrUnauthorized)
		return
	}
	if _, err := s.core.GetNamespace(r.Context(), name); err != nil {
		if errors.Is(err, namespace.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "namespace not found"})
			return
		}
		writeErr(w, err)
		return
	}
	st, err := s.namespaceStats(r.Context(), name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "stats": st})
}

// namespaceView is one namespace in the detailed list.
type namespaceView struct {
	*namespace.Namespace
	Stats *nsStats `json:"stats,omitempty"`
}

// namespaceDetails returns the metadata (and with withStats the counters) of
// the namespaces the caller may look at, sorted by name.
func (s *Server) namespaceDetails(ctx context.Context, names []string, withStats bool) ([]namespaceView, error) {
	bound, err := s.boundNamespace(ctx)
	if err != nil {
		return nil, err
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	out := make([]namespaceView, 0, len(sorted))
	for _, n := range sorted {
		if bound != "" && n != bound {
			continue
		}
		ns, err := s.core.GetNamespace(ctx, n)
		if err != nil {
			continue // removed between list and get
		}
		v := namespaceView{Namespace: ns}
		if withStats {
			st, err := s.namespaceStats(ctx, n)
			if err != nil {
				return nil, err
			}
			v.Stats = &st
		}
		out = append(out, v)
	}
	return out, nil
}

// PUT /v1/sys/namespaces/{name}
// Body: {"description":"...","owner":"...","labels":{"k":"v"}}
// Replaces the namespace metadata; the name and creation time never change.
func (s *Server) updateNamespace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/namespaces/"+name, policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	ok, err := s.mayViewNamespace(r.Context(), name)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		writeErr(w, core.ErrUnauthorized)
		return
	}
	var body namespace.Metadata
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	ns, err := s.core.UpdateNamespaceMetadata(r.Context(), name, body)
	if err != nil {
		switch {
		case errors.Is(err, namespace.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "namespace not found"})
		case errors.Is(err, namespace.ErrInvalidMetadata):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, ns)
}

// GET /v1/sys/usage/secrets
// Number of KV secrets in the caller's namespace. Needs list on secret/, like
// listing the root of the engine does.
func (s *Server) usageSecrets(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "secret/", policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	ns := nsFromCtx(r.Context())
	v1, v2, err := s.core.SecretCounts(r.Context(), ns)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secrets":     v1 + v2,
		"secrets_kv1": v1,
		"secrets_kv2": v2,
	})
}

// mountStats are the counters of one mount. Fields are absent when they do
// not apply to the mount or the caller may not list what they count.
type mountStats struct {
	// EngineBacked is true for the engines built into the server. A mount
	// added through the mount table is a metadata entry only: no engine is
	// attached and nothing is stored under it, so removing it deletes no data.
	EngineBacked bool `json:"engine_backed"`

	// Impact of removing the mount.
	Secrets *int `json:"secrets,omitempty"`
	Leases  *int `json:"leases,omitempty"`
	Tokens  *int `json:"tokens,omitempty"`

	// KV engine detail.
	SecretsKV1        *int       `json:"secrets_kv1,omitempty"`
	SecretsKV2        *int       `json:"secrets_kv2,omitempty"`
	Versions          *int       `json:"versions,omitempty"`
	DeletedVersions   *int       `json:"deleted_versions,omitempty"`
	DestroyedVersions *int       `json:"destroyed_versions,omitempty"`
	HiddenSecrets     *int       `json:"hidden_secrets,omitempty"`
	LastWriteAt       *time.Time `json:"last_write_at,omitempty"`
	Truncated         bool       `json:"truncated,omitempty"`

	// Dynamic engine detail.
	Roles       *int `json:"roles,omitempty"`
	Connections *int `json:"connections,omitempty"`
	Certs       *int `json:"issued_certs,omitempty"`
}

// mountStatsFor computes the counters of entry e for the caller.
func (s *Server) mountStatsFor(ctx context.Context, e *mount.Entry) (*mountStats, error) {
	if !e.Builtin {
		// Table-only mount: nothing is stored under it, nothing depends on it.
		return &mountStats{Secrets: intp(0), Leases: intp(0), Tokens: intp(0)}, nil
	}
	canList, err := s.core.CapabilityChecker(ctx, tokenFromCtx(ctx), policy.CapList)
	if err != nil {
		return nil, err
	}
	st := &mountStats{EngineBacked: true}
	ns := nsFromCtx(ctx)
	switch e.Path {
	case "secret/":
		if !canList("secret/") {
			break
		}
		canRead, err := s.core.CapabilityChecker(ctx, tokenFromCtx(ctx), policy.CapRead)
		if err != nil {
			return nil, err
		}
		v1, _, err := s.core.SecretCounts(ctx, ns)
		if err != nil {
			return nil, err
		}
		ks, err := s.core.KVv2(ns).Stats(ctx, 0, func(p string) bool { return canRead(v2EnforcePath(p)) })
		if err != nil {
			return nil, err
		}
		st.SecretsKV1, st.SecretsKV2 = intp(v1), intp(ks.Secrets)
		st.Secrets = intp(v1 + ks.Secrets)
		st.Versions, st.DeletedVersions, st.DestroyedVersions = intp(ks.Versions), intp(ks.DeletedVersions), intp(ks.DestroyedVersions)
		if ks.Hidden > 0 {
			st.HiddenSecrets = intp(ks.Hidden)
		}
		if !ks.LastWriteAt.IsZero() {
			t := ks.LastWriteAt
			st.LastWriteAt = &t
		}
		st.Truncated = ks.Truncated
	case "database/":
		du, err := s.core.DatabaseUsage(ctx)
		if err != nil {
			return nil, err
		}
		if canList("database/lease") {
			st.Leases = intp(du.ActiveLeases)
		}
		if canList("database/role") {
			st.Roles = intp(du.Roles)
		}
		if canList("database/config") {
			st.Connections = intp(du.Connections)
		}
	case "pki/":
		pu, err := s.core.PKIUsage(ctx)
		if err != nil {
			return nil, err
		}
		if canList("pki/roles") {
			st.Roles = intp(pu.Roles)
		}
		if canList("pki/certs") {
			st.Certs = intp(pu.Issued)
		}
	case "auth/":
		if canList("auth/token") {
			counts, trc, err := s.core.TokenCounts(ctx)
			if err != nil {
				return nil, err
			}
			ns := nsFromCtx(ctx)
			if ns == namespace.RootNamespace {
				ns = ""
			}
			st.Tokens = intp(counts[ns])
			st.Truncated = trc
		}
	}
	return st, nil
}
