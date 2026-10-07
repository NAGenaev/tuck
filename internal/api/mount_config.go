package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/kvv2"
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
	entry, err := s.core.MountStore().Get(r.Context(), mountPath)
	if err != nil {
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
	if r.URL.Query().Get("stats") == "true" {
		// Additive: usage counters next to the tuning fields.
		st, err := s.mountStatsFor(r.Context(), entry)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			mount.Config
			Stats *mountStats `json:"stats"`
		}{cfg, st})
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
		// KV v2 options (secret/ mount only); absent = unchanged.
		MaxVersions *int  `json:"max_versions"`
		CASRequired *bool `json:"cas_required"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if errMsg := validateKV2Options(mountPath, wire.MaxVersions, wire.CASRequired); errMsg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
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
	cfg = applyKV2Options(cfg, wire.MaxVersions, wire.CASRequired)

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

// validateKV2Options checks the KV v2 mount-tuning options shared by the tune
// and create-mount endpoints. It returns a non-empty error message if
// maxVersions/casRequired are set but the target path is not the KV v2 mount
// (core.KV2MountPath, i.e. "secret/" — the only path whose routes are wired to
// the KV v2 engine in this codebase), or if maxVersions is out of range.
// Returns "" when there is nothing to reject (including when both are nil).
func validateKV2Options(mountPath string, maxVersions *int, casRequired *bool) string {
	if maxVersions == nil && casRequired == nil {
		return ""
	}
	if strings.Trim(mountPath, "/") != strings.Trim(core.KV2MountPath, "/") {
		return "max_versions and cas_required are KV v2 options and can only be set on the secret/ mount"
	}
	if maxVersions != nil && (*maxVersions < 0 || *maxVersions > kvv2.MaxVersionsLimit) {
		return "max_versions must be between 0 and " + strconv.Itoa(kvv2.MaxVersionsLimit)
	}
	return ""
}

// applyKV2Options overlays the given overrides (nil = unchanged) onto cfg and
// returns the result. It never fails; callers must validate first.
func applyKV2Options(cfg mount.Config, maxVersions *int, casRequired *bool) mount.Config {
	if maxVersions != nil {
		cfg.MaxVersions = *maxVersions
	}
	if casRequired != nil {
		cfg.CASRequired = *casRequired
	}
	return cfg
}
