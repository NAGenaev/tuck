package api

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/auth/approle"
)

// POST /v1/auth/approle/login
// Body: {"role_id":"...","secret_id":"..."}
func (s *Server) loginAppRole(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		RoleID   string `json:"role_id"`
		SecretID string `json:"secret_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.RoleID == "" || req.SecretID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role_id and secret_id required"})
		return
	}

	remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	tok, err := s.core.LoginAppRole(r.Context(), req.RoleID, req.SecretID, remoteIP)
	if err != nil {
		if errors.Is(err, approle.ErrInvalidCredentials) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired credentials"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      tok.ID,
		"policies":   tok.Policies,
		"expires_at": tok.ExpiresAt,
	})
}

// PUT /v1/auth/approle/role/{name}
// Body: {"policies":["p"],"token_ttl":"1h","secret_id_ttl":"24h","secret_id_num_uses":0}
func (s *Server) putAppRole(w http.ResponseWriter, r *http.Request) {
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
		RoleID          string   `json:"role_id"`
		Policies        []string `json:"policies"`
		TokenTTL        string   `json:"token_ttl"`
		SecretIDTTL     string   `json:"secret_id_ttl"`
		SecretIDNumUses int      `json:"secret_id_num_uses"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if len(wire.Policies) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policies required"})
		return
	}

	role := &approle.Role{
		Name:            name,
		RoleID:          wire.RoleID,
		Policies:        wire.Policies,
		SecretIDNumUses: wire.SecretIDNumUses,
	}
	if wire.TokenTTL != "" {
		d, err := time.ParseDuration(wire.TokenTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid token_ttl: " + err.Error()})
			return
		}
		role.TokenTTL = d
	}
	if wire.SecretIDTTL != "" {
		d, err := time.ParseDuration(wire.SecretIDTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid secret_id_ttl: " + err.Error()})
			return
		}
		role.SecretIDTTL = d
	}

	if err := s.core.PutAppRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// GET /v1/auth/approle/role/{name}
func (s *Server) getAppRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	role, err := s.core.GetAppRole(r.Context(), name)
	if err != nil {
		if errors.Is(err, approle.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/auth/approle/role/{name}
func (s *Server) deleteAppRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.DeleteAppRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/auth/approle/role/
func (s *Server) listAppRoles(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.ListAppRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// POST /v1/auth/approle/role/{name}/secret-id
// Optional body: {"bound_cidrs":["10.0.0.0/8"],"metadata":{"key":"value"}}
func (s *Server) generateSecretID(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var opts approle.SecretIDOptions
	if r.ContentLength > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		var body struct {
			BoundCIDRs []string          `json:"bound_cidrs"`
			Metadata   map[string]string `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}
		opts.BoundCIDRs = body.BoundCIDRs
		opts.Metadata = body.Metadata
	}

	sid, err := s.core.GenerateSecretIDWithOptions(r.Context(), name, opts)
	if err != nil {
		if errors.Is(err, approle.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sid)
}

// GET /v1/auth/approle/role/{name}/secret-id/{id}
func (s *Server) lookupSecretID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sid, err := s.core.LookupSecretID(r.Context(), id)
	if err != nil {
		if errors.Is(err, approle.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "secret_id not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sid)
}

// DELETE /v1/auth/approle/role/{name}/secret-id/{id}
func (s *Server) destroySecretID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.DestroySecretID(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
