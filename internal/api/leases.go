package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/lease"
	"github.com/NAGenaev/tuck/internal/policy"
)

// GET /v1/sys/leases/{id...}  (or ?list=true on /v1/sys/leases/ to list all)
func (s *Server) getLease(w http.ResponseWriter, r *http.Request) {
	if wantsList(r) {
		s.listLeases(w, r)
		return
	}
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

// POST /v1/sys/leases/renew
// Body: {"lease_id": "backend/id", "increment": "1h"}
// Response: {"lease_id": "...", "expires_at": "..."}
func (s *Server) renewLease(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/leases/renew", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var wire struct {
		LeaseID   string `json:"lease_id"`
		Increment string `json:"increment"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if wire.LeaseID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lease_id is required"})
		return
	}
	increment := time.Hour
	if wire.Increment != "" {
		d, err := time.ParseDuration(wire.Increment)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid increment"})
			return
		}
		increment = d
	}
	expiresAt, err := s.core.RenewLease(r.Context(), wire.LeaseID, increment)
	if err != nil {
		switch {
		case errors.Is(err, lease.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lease_id":   wire.LeaseID,
		"expires_at": expiresAt,
	})
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
