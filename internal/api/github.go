package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	ghauth "github.com/NAGenaev/tuck/internal/auth/github"
	"github.com/NAGenaev/tuck/internal/policy"
)

// POST /v1/auth/github/login
func (s *Server) loginGitHub(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Token == "" || body.Role == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token and role are required"})
		return
	}
	tok, err := s.core.LoginGitHub(r.Context(), body.Token, body.Role)
	if err != nil {
		switch {
		case errors.Is(err, ghauth.ErrRoleNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
		case errors.Is(err, ghauth.ErrNoMatch):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "token claims do not match role"})
		case errors.Is(err, ghauth.ErrInvalidToken):
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired GitHub OIDC token"})
		default:
			writeErr(w, err)
		}
		return
	}
	var ttlSec float64
	var expiresAt string
	if !tok.ExpiresAt.IsZero() {
		ttlSec = time.Until(tok.ExpiresAt).Seconds()
		expiresAt = tok.ExpiresAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":         tok.ID,
		"policies":   tok.Policies,
		"ttl":        ttlSec,
		"expires_at": expiresAt,
	})
}

// PUT /v1/auth/github/role/{name}
func (s *Server) putGitHubRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/github/role/"+name, policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	var role ghauth.Role
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&role); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	role.Name = name
	if err := s.core.PutGitHubRole(r.Context(), &role); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/auth/github/role/{name}
func (s *Server) getGitHubRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/github/role/"+name, policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	role, err := s.core.GetGitHubRole(r.Context(), name)
	if err != nil {
		if errors.Is(err, ghauth.ErrRoleNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/auth/github/role/{name}
func (s *Server) deleteGitHubRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/github/role/"+name, policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.DeleteGitHubRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/auth/github/role/
func (s *Server) listGitHubRoles(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/github/role", policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	names, err := s.core.ListGitHubRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}
