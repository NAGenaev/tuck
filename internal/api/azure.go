package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/dynamic/azure"
)

// PUT /v1/azure/config
func (s *Server) putAzureConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var cfg azure.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.core.PutAzureConfig(r.Context(), &cfg); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/azure/config
func (s *Server) getAzureConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.core.GetAzureConfig(r.Context())
	if err != nil {
		if errors.Is(err, azure.ErrNotConfigured) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "azure engine not configured"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// DELETE /v1/azure/config
func (s *Server) deleteAzureConfig(w http.ResponseWriter, r *http.Request) {
	if err := s.core.DeleteAzureConfig(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /v1/azure/roles/{name}
// Body: {"application_object_id":"...","application_id":"...","default_ttl":"1h","max_ttl":"12h"}
func (s *Server) putAzureRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role name required"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var wire struct {
		ApplicationObjectID string `json:"application_object_id"`
		ApplicationID       string `json:"application_id"`
		DefaultTTL          string `json:"default_ttl"`
		MaxTTL              string `json:"max_ttl"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if wire.ApplicationObjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "application_object_id is required"})
		return
	}
	if wire.ApplicationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "application_id is required"})
		return
	}
	role := &azure.Role{
		Name:                name,
		ApplicationObjectID: wire.ApplicationObjectID,
		ApplicationID:       wire.ApplicationID,
	}
	if wire.DefaultTTL != "" {
		d, err := time.ParseDuration(wire.DefaultTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid default_ttl"})
			return
		}
		role.DefaultTTL = d
	}
	if wire.MaxTTL != "" {
		d, err := time.ParseDuration(wire.MaxTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid max_ttl"})
			return
		}
		role.MaxTTL = d
	}
	if err := s.core.PutAzureRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/azure/roles/{name}
func (s *Server) getAzureRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	role, err := s.core.GetAzureRole(r.Context(), name)
	if err != nil {
		if errors.Is(err, azure.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/azure/roles/{name}
func (s *Server) deleteAzureRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.DeleteAzureRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/azure/roles/
func (s *Server) listAzureRoles(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.ListAzureRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// POST /v1/azure/creds/{role}
func (s *Server) generateAzureCreds(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("role")
	res, err := s.core.GenerateAzureCreds(r.Context(), roleName)
	if err != nil {
		switch {
		case errors.Is(err, azure.ErrNotConfigured):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "azure engine not configured"})
		case errors.Is(err, azure.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// GET /v1/azure/lease/{id}
func (s *Server) getAzureLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lease, err := s.core.GetAzureLease(r.Context(), id)
	if err != nil {
		if errors.Is(err, azure.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lease)
}

// DELETE /v1/azure/lease/{id}
func (s *Server) revokeAzureLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.RevokeAzureLease(r.Context(), id); err != nil {
		if errors.Is(err, azure.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/azure/lease/
func (s *Server) listAzureLeases(w http.ResponseWriter, r *http.Request) {
	ids, err := s.core.ListAzureLeases(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": ids})
}
