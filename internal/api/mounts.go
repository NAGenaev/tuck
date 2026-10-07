package api

import (
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
	resp := map[string]any{"mounts": entries}
	if r.URL.Query().Get("stats") == "true" {
		// Additive: counters keyed by mount path, limited to what the caller may list.
		stats := make(map[string]*mountStats, len(entries))
		for _, e := range entries {
			st, err := s.mountStatsFor(r.Context(), e)
			if err != nil {
				writeErr(w, err)
				return
			}
			stats[e.Path] = st
		}
		resp["stats"] = stats
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /v1/sys/mounts/{path...}
// Body: {"type":"kv","description":"my KV store","config":{"max_versions":5,"cas_required":true}}
// The optional "config" block is only accepted for the KV v2 mount
// (core.KV2MountPath, "secret/" today — see validateKV2Options); it lets a
// caller create the mount already tuned instead of needing a follow-up
// POST /v1/sys/mounts-tune/{path...} call. Applied atomically: on any error
// (bad config, path collision, ...) nothing is created or persisted.
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
		Config      *struct {
			MaxVersions *int  `json:"max_versions"`
			CASRequired *bool `json:"cas_required"`
		} `json:"config"`
	}
	if !decodeJSON(w, body, &req) {
		return
	}
	if req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type required"})
		return
	}
	var maxVersions *int
	var casRequired *bool
	if req.Config != nil {
		maxVersions, casRequired = req.Config.MaxVersions, req.Config.CASRequired
		if errMsg := validateKV2Options(mountPath, maxVersions, casRequired); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
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
	if maxVersions != nil || casRequired != nil {
		cfg, err := s.core.MountConfigStore().Get(r.Context(), mountPath)
		if err == nil {
			cfg = applyKV2Options(cfg, maxVersions, casRequired)
			err = s.core.MountConfigStore().Put(r.Context(), mountPath, cfg)
		}
		if err != nil {
			// Keep creation atomic: do not leave a mount behind whose
			// requested config could not be persisted.
			_ = s.core.MountStore().Delete(r.Context(), mountPath)
			writeErr(w, err)
			return
		}
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
