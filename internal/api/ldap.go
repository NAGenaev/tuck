package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	authlda "github.com/NAGenaev/tuck/internal/auth/ldap"
)

// POST /v1/auth/ldap/login
// Body: {"username":"alice","password":"s3cr3t"}
func (s *Server) loginLDAP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password required"})
		return
	}

	tok, err := s.core.LoginLDAP(r.Context(), req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, authlda.ErrNotConfigured):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LDAP auth not configured"})
		case errors.Is(err, authlda.ErrInvalidCredentials):
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
		case errors.Is(err, authlda.ErrNoRole):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "no matching role for user"})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      tok.ID,
		"policies":   tok.Policies,
		"expires_at": tok.ExpiresAt,
	})
}

// GET /v1/auth/ldap/config
func (s *Server) getLDAPConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.core.GetLDAPConfig(r.Context())
	if err != nil {
		if errors.Is(err, authlda.ErrNotConfigured) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "ldap auth not configured"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// PUT /v1/auth/ldap/config
// Body: {"urls":["ldap://..."],"bind_dn":"...","bind_password":"...","user_dn":"..."}
func (s *Server) putLDAPConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var cfg authlda.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if len(cfg.URLs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "urls is required"})
		return
	}
	if cfg.UserDN == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_dn is required"})
		return
	}
	if err := s.core.PutLDAPConfig(r.Context(), &cfg); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /v1/auth/ldap/role/{name}
// Body: {"groups":["admin"],"policies":["admin-policy"],"ttl":"1h"}
func (s *Server) putLDAPRole(w http.ResponseWriter, r *http.Request) {
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
		Groups   []string `json:"groups"`
		Users    []string `json:"users"`
		Policies []string `json:"policies"`
		TTL      string   `json:"ttl"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	role := &authlda.Role{
		Name:     name,
		Groups:   wire.Groups,
		Users:    wire.Users,
		Policies: wire.Policies,
	}
	if wire.TTL != "" {
		d, err := time.ParseDuration(wire.TTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl"})
			return
		}
		role.TTL = d
	}
	if err := s.core.PutLDAPRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/auth/ldap/role/{name}
func (s *Server) getLDAPRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role name required"})
		return
	}
	role, err := s.core.GetLDAPRole(r.Context(), name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/auth/ldap/role/{name}
func (s *Server) deleteLDAPRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role name required"})
		return
	}
	if err := s.core.DeleteLDAPRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/auth/ldap/role/
func (s *Server) listLDAPRoles(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.ListLDAPRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}
