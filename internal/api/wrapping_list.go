package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/policy"
	"github.com/NAGenaev/tuck/internal/wrapping"
)

// wrapVisibility returns the predicate deciding which capsules the calling
// token may see or revoke.
//
// A caller always reaches the capsules it created itself. A caller whose ACL
// grants adminCap on adminPath (root does, through "**") reaches every capsule.
// A token bound to a namespace — or a request carrying X-Tuck-Namespace — is
// further confined to that namespace, whatever its rights.
func (s *Server) wrapVisibility(r *http.Request, adminPath string, adminCap policy.Capability) (keep func(*wrapping.Capsule) bool, admin bool, err error) {
	ctx := r.Context()
	tok, err := s.core.Authenticate(ctx, tokenFromCtx(ctx))
	if err != nil {
		return nil, false, err
	}
	admin = s.core.EnforceAccess(ctx, tok.ID, adminPath, adminCap) == nil
	ns := nsFromCtx(ctx) // "" = root namespace, unrestricted
	accessor := tok.Accessor
	return func(c *wrapping.Capsule) bool {
		if ns != "" && c.Namespace != ns {
			return false
		}
		if admin {
			return true
		}
		return c.CreatorAccessor != "" && c.CreatorAccessor == accessor
	}, admin, nil
}

// GET /v1/sys/wrapping/list
// Response: {"scope":"own|all","capsules":[{accessor, creation_time, expires_at,
// creation_ttl, size, namespace, creator_accessor, creator_fingerprint,
// creator_name, creation_path}]}
//
// Metadata only: the wrapping token and the payload are never returned, and the
// accessor cannot be used to unwrap. Callers see the capsules they created;
// tokens with list capability on sys/wrapping/list (root) see all of them.
func (s *Server) listWrapped(w http.ResponseWriter, r *http.Request) {
	keep, admin, err := s.wrapVisibility(r, "sys/wrapping/list", policy.CapList)
	if err != nil {
		writeErr(w, err)
		return
	}
	caps, err := s.core.ListWrapped(r.Context(), keep)
	if err != nil {
		writeErr(w, err)
		return
	}
	if caps == nil {
		caps = []wrapping.Capsule{}
	}
	scope := "own"
	if admin {
		scope = "all"
	}
	writeJSON(w, http.StatusOK, map[string]any{"scope": scope, "capsules": caps})
}

// GET /v1/sys/wrapping/stats
// Response: {"wrapped":N,"unwrapped":N,"expired":N,"revoked":N,"active":N,"since":"..."}
//
// Lifetime counters (persisted). Requires read capability on sys/wrapping/stats.
// "active" is the number of live capsules at this moment.
func (s *Server) wrappingStats(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/wrapping/stats", policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	// List first: it sweeps expired capsules, so the counters below include them.
	live, err := s.core.ListWrapped(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	st, err := s.core.WrappingStats(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"wrapped":   st.Wrapped,
		"unwrapped": st.Unwrapped,
		"expired":   st.Expired,
		"revoked":   st.Revoked,
		"active":    len(live),
		"since":     st.Since.Truncate(time.Second),
	})
}

// DELETE /v1/sys/wrapping/revoke-accessor/{accessor}
//
// Destroys a wrapped response without knowing its token. The accessor is part
// of the URL so that it lands in the audit log. The caller must have created
// the capsule, or hold delete capability on sys/wrapping/revoke-accessor.
// A capsule the caller may not touch is reported as 404, like a missing one.
func (s *Server) revokeWrappedByAccessor(w http.ResponseWriter, r *http.Request) {
	accessor := r.PathValue("accessor")
	if accessor == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "accessor is required"})
		return
	}
	allow, _, err := s.wrapVisibility(r, "sys/wrapping/revoke-accessor", policy.CapDelete)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.RevokeWrappedByAccessor(r.Context(), accessor, allow); err != nil {
		if errors.Is(err, wrapping.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "wrapped response not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
