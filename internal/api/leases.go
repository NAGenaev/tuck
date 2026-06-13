package api

import (
	"errors"
	"net/http"

	"github.com/NAGenaev/tuck/internal/lease"
	"github.com/NAGenaev/tuck/internal/policy"
)

// GET /v1/sys/leases/{id...}
func (s *Server) getLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/leases/"+id, policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	info, err := s.core.LeaseManager().Lookup(r.Context(), id)
	if err != nil {
		if errors.Is(err, lease.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// DELETE /v1/sys/leases/{id...}
func (s *Server) revokeLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/leases/"+id, policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.LeaseManager().Revoke(r.Context(), id); err != nil {
		if errors.Is(err, lease.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/sys/leases/
func (s *Server) listLeases(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/leases", policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	leases, err := s.core.LeaseManager().List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if leases == nil {
		leases = []*lease.Info{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"leases": leases})
}
