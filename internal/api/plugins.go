package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/NAGenaev/tuck/internal/plugin"
	"github.com/NAGenaev/tuck/internal/policy"
)

// GET /v1/sys/plugins/catalog/{type}/{name}
func (s *Server) getPlugin(w http.ResponseWriter, r *http.Request) {
	t := plugin.PluginType(r.PathValue("type"))
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/plugins", policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	e, err := s.core.PluginCatalog().Get(r.Context(), t, name)
	if err != nil {
		if errors.Is(err, plugin.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "plugin not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// POST /v1/sys/plugins/catalog/{type}/{name}
// Body: {"command":"/opt/plugins/my-engine","sha256":"abc...","version":"v1.0.0"}
func (s *Server) registerPlugin(w http.ResponseWriter, r *http.Request) {
	t := plugin.PluginType(r.PathValue("type"))
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/plugins", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Command string   `json:"command"`
		SHA256  string   `json:"sha256"`
		Args    []string `json:"args"`
		Env     []string `json:"env"`
		Version string   `json:"version"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Command == "" || req.SHA256 == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command and sha256 required"})
		return
	}
	e := &plugin.Entry{
		Name:    name,
		Type:    t,
		Command: req.Command,
		SHA256:  req.SHA256,
		Args:    req.Args,
		Env:     req.Env,
		Version: req.Version,
	}
	if err := s.core.PluginCatalog().Register(r.Context(), e); err != nil {
		switch {
		case errors.Is(err, plugin.ErrInvalidType):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, plugin.ErrInvalidName):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// DELETE /v1/sys/plugins/catalog/{type}/{name}
func (s *Server) deletePlugin(w http.ResponseWriter, r *http.Request) {
	t := plugin.PluginType(r.PathValue("type"))
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/plugins", policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.PluginCatalog().Delete(r.Context(), t, name); err != nil {
		if errors.Is(err, plugin.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "plugin not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/sys/plugins/catalog/{type}/  or  LIST /v1/sys/plugins/catalog/
func (s *Server) listPlugins(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/plugins", policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	t := plugin.PluginType(r.PathValue("type"))
	entries, err := s.core.PluginCatalog().List(r.Context(), t)
	if err != nil {
		writeErr(w, err)
		return
	}
	if entries == nil {
		entries = []*plugin.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": entries})
}
