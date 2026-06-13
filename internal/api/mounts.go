package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/NAGenaev/tuck/internal/mount"
	"github.com/NAGenaev/tuck/internal/policy"
)

// GET /v1/sys/mounts
func (s *Server) listMounts(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/mounts", policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	entries, err := s.core.MountStore().List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if entries == nil {
		entries = []*mount.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"mounts": entries})
}

// POST /v1/sys/mounts/{path...}
// Body: {"type":"kv","description":"my KV store"}
func (s *Server) createMount(w http.ResponseWriter, r *http.Request) {
	mountPath := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/mounts", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type required"})
		return
	}
	entry, err := s.core.MountStore().Register(r.Context(), mountPath, req.Type, req.Description)
	if err != nil {
		if errors.Is(err, mount.ErrAlreadyExists) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// DELETE /v1/sys/mounts/{path...}
func (s *Server) deleteMount(w http.ResponseWriter, r *http.Request) {
	mountPath := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/mounts", policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.MountStore().Delete(r.Context(), mountPath); err != nil {
		switch {
		case errors.Is(err, mount.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		case errors.Is(err, mount.ErrBuiltin):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		default:
			writeErr(w, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
