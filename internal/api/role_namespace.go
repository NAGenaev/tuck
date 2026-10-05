package api

import (
	"net/http"

	"github.com/NAGenaev/tuck/internal/namespace"
)

// roleNamespace validates the tuck_namespace of an auth role being written and
// returns the value to store. The role table is global, so a caller operating
// inside a namespace may only create roles for that same namespace (and gets it
// by default); the login request itself never chooses the namespace.
func (s *Server) roleNamespace(w http.ResponseWriter, r *http.Request, requested string) (string, bool) {
	if requested == namespace.RootNamespace {
		requested = ""
	}
	if caller := nsFromCtx(r.Context()); caller != "" {
		if requested == "" {
			requested = caller
		} else if requested != caller {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
			return "", false
		}
	}
	if requested == "" {
		return "", true
	}
	if _, err := s.core.GetNamespace(r.Context(), requested); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "namespace not found"})
		return "", false
	}
	return requested, true
}
