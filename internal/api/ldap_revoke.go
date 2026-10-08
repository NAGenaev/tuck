package api

import (
	"net/http"

	"github.com/NAGenaev/tuck/internal/policy"
)

// POST /v1/auth/ldap/recheck
//
// Runs the directory re-validation of all live LDAP tokens now instead of
// waiting for the periodic pass: tokens whose owner no longer has a matching
// role are revoked, tokens that lost some policies are narrowed. Root only:
// it touches tokens of every namespace.
func (s *Server) recheckLDAP(w http.ResponseWriter, r *http.Request) {
	if nsFromCtx(r.Context()) != "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/ldap/recheck", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	res, err := s.core.RecheckLDAPTokens(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// POST /v1/auth/ldap/users/{username}/revoke
//
// Revokes every live token issued to the LDAP login (and what those tokens
// created). The immediate answer to "this person must lose access now".
func (s *Server) revokeLDAPUser(w http.ResponseWriter, r *http.Request) {
	if nsFromCtx(r.Context()) != "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}
	user := r.PathValue("username")
	if user == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username required"})
		return
	}
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/ldap/users/"+user+"/revoke", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	n, err := s.core.RevokeLDAPUserTokens(r.Context(), user)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": user, "revoked": n})
}
