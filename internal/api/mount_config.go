package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/mount"
	"github.com/NAGenaev/tuck/internal/policy"
)

// GET /v1/sys/mounts/{path...}/tune
func (s *Server) getMountConfig(w http.ResponseWriter, r *http.Request) {
	mountPath := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/mounts", policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	// Ensure the mount exists.
	if _, err := s.core.MountStore().Get(r.Context(), mountPath); err != nil {
		if errors.Is(err, mount.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "mount not found"})
			return
		}
		writeErr(w, err)
		return
	}
	cfg, err := s.core.MountConfigStore().Get(r.Context(), mountPath)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// POST /v1/sys/mounts/{path...}/tune
// Body: {"default_lease_ttl":"1h","max_lease_ttl":"24h","force_no_cache":false}
func (s *Server) putMountConfig(w http.ResponseWriter, r *http.Request) {
	mountPath := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/mounts", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	// Ensure the mount exists.
	if _, err := s.core.MountStore().Get(r.Context(), mountPath); err != nil {
		if errors.Is(err, mount.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "mount not found"})
			return
		}
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var wire struct {
		DefaultLeaseTTL           string   `json:"default_lease_ttl"`
		MaxLeaseTTL               string   `json:"max_lease_ttl"`
		ForceNoCache              bool     `json:"force_no_cache"`
		AllowedResponseHeaders    []string `json:"allowed_response_headers"`
		PassthroughRequestHeaders []string `json:"passthrough_request_headers"`
		Description               string   `json:"description"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	// Load existing config so we can merge.
	cfg, err := s.core.MountConfigStore().Get(r.Context(), mountPath)
	if err != nil {
		writeErr(w, err)
		return
	}

	if wire.DefaultLeaseTTL != "" {
		d, err := time.ParseDuration(wire.DefaultLeaseTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid default_lease_ttl: " + err.Error()})
			return
		}
		cfg.DefaultLeaseTTL = d
	}
	if wire.MaxLeaseTTL != "" {
		d, err := time.ParseDuration(wire.MaxLeaseTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid max_lease_ttl: " + err.Error()})
			return
		}
		cfg.MaxLeaseTTL = d
	}
	cfg.ForceNoCache = wire.ForceNoCache
	if wire.AllowedResponseHeaders != nil {
		cfg.AllowedResponseHeaders = wire.AllowedResponseHeaders
	}
	if wire.PassthroughRequestHeaders != nil {
		cfg.PassthroughRequestHeaders = wire.PassthroughRequestHeaders
	}
	if wire.Description != "" {
		cfg.Description = wire.Description
	}

	if cfg.MaxLeaseTTL > 0 && cfg.DefaultLeaseTTL > cfg.MaxLeaseTTL {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "default_lease_ttl cannot exceed max_lease_ttl"})
		return
	}

	if err := s.core.MountConfigStore().Put(r.Context(), mountPath, cfg); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
