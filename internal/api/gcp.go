package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/dynamic/gcp"
)

// PUT /v1/gcp/config
func (s *Server) putGCPConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var cfg gcp.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.core.PutGCPConfig(r.Context(), &cfg); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/gcp/config
func (s *Server) getGCPConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.core.GetGCPConfig(r.Context())
	if err != nil {
		if errors.Is(err, gcp.ErrNotConfigured) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "gcp engine not configured"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// DELETE /v1/gcp/config
func (s *Server) deleteGCPConfig(w http.ResponseWriter, r *http.Request) {
	if err := s.core.DeleteGCPConfig(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /v1/gcp/roles/{name}
// Body: {"credential_type":"service_account_key","service_account_email":"svc@proj.iam...","default_ttl":"1h"}
func (s *Server) putGCPRole(w http.ResponseWriter, r *http.Request) {
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
		CredentialType      string   `json:"credential_type"`
		ServiceAccountEmail string   `json:"service_account_email"`
		KeyAlgorithm        string   `json:"key_algorithm"`
		Scopes              []string `json:"scopes"`
		DefaultTTL          string   `json:"default_ttl"`
		MaxTTL              string   `json:"max_ttl"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if wire.CredentialType != gcp.CredTypeServiceAccountKey && wire.CredentialType != gcp.CredTypeAccessToken {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential_type must be service_account_key or access_token"})
		return
	}
	if wire.ServiceAccountEmail == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "service_account_email is required"})
		return
	}
	role := &gcp.Role{
		Name:                name,
		CredentialType:      wire.CredentialType,
		ServiceAccountEmail: wire.ServiceAccountEmail,
		KeyAlgorithm:        wire.KeyAlgorithm,
		Scopes:              wire.Scopes,
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
	if err := s.core.PutGCPRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/gcp/roles/{name}
func (s *Server) getGCPRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	role, err := s.core.GetGCPRole(r.Context(), name)
	if err != nil {
		if errors.Is(err, gcp.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/gcp/roles/{name}
func (s *Server) deleteGCPRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.DeleteGCPRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/gcp/roles/
func (s *Server) listGCPRoles(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.ListGCPRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// POST /v1/gcp/creds/{role}
func (s *Server) generateGCPCreds(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("role")
	res, err := s.core.GenerateGCPCreds(r.Context(), roleName)
	if err != nil {
		switch {
		case errors.Is(err, gcp.ErrNotConfigured):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "gcp engine not configured"})
		case errors.Is(err, gcp.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// GET /v1/gcp/lease/{id}
func (s *Server) getGCPLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lease, err := s.core.GetGCPLease(r.Context(), id)
	if err != nil {
		if errors.Is(err, gcp.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lease)
}

// DELETE /v1/gcp/lease/{id}
func (s *Server) revokeGCPLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.RevokeGCPLease(r.Context(), id); err != nil {
		if errors.Is(err, gcp.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/gcp/lease/
func (s *Server) listGCPLeases(w http.ResponseWriter, r *http.Request) {
	ids, err := s.core.ListGCPLeases(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": ids})
}
