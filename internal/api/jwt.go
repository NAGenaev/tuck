package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	authjwt "github.com/NAGenaev/tuck/internal/auth/jwt"
)

// POST /v1/auth/jwt/login
// Body: {"jwt": "<token>"}
// Returns a Tuck token if the JWT is valid and matches a configured role.
func (s *Server) loginJWT(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		JWT string `json:"jwt"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.JWT == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "jwt field required"})
		return
	}

	tok, err := s.core.LoginJWT(r.Context(), req.JWT)
	if err != nil {
		switch {
		case errors.Is(err, authjwt.ErrInvalidToken):
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired JWT"})
		case errors.Is(err, authjwt.ErrNoRole):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "no matching role for token claims"})
		case errors.Is(err, authjwt.ErrConfigNotFound):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "JWT auth not configured"})
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

// PUT /v1/auth/jwt/config
// Body: {"jwks_uri":"...","issuer":"...","audience":"...","default_ttl":"1h"}
func (s *Server) putJWTConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	// Parse as a wire struct with string TTL for ergonomics.
	var wire struct {
		JWKSURI    string `json:"jwks_uri"`
		Issuer     string `json:"issuer"`
		Audience   string `json:"audience"`
		DefaultTTL string `json:"default_ttl"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if wire.JWKSURI == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "jwks_uri is required"})
		return
	}

	cfg := &authjwt.Config{
		JWKSURI:  wire.JWKSURI,
		Issuer:   wire.Issuer,
		Audience: wire.Audience,
	}
	if wire.DefaultTTL != "" {
		d, err := time.ParseDuration(wire.DefaultTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid default_ttl: " + err.Error()})
			return
		}
		cfg.DefaultTTL = d
	}

	if err := s.core.ConfigureJWT(r.Context(), cfg); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /v1/auth/jwt/config
func (s *Server) getJWTConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.core.GetJWTConfig(r.Context())
	if err != nil {
		if errors.Is(err, authjwt.ErrConfigNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "JWT auth not configured"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// PUT /v1/auth/jwt/role/{name}
// Body: {"bound_subject":"...","bound_claims":{...},"policies":["p1"],"ttl":"1h"}
func (s *Server) putJWTRole(w http.ResponseWriter, r *http.Request) {
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
		BoundSubject   string            `json:"bound_subject"`
		BoundClaims    map[string]string `json:"bound_claims"`
		BoundAudiences []string          `json:"bound_audiences"`
		Policies       []string          `json:"policies"`
		TTL            string            `json:"ttl"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if len(wire.Policies) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policies required"})
		return
	}

	role := &authjwt.Role{
		Name:           name,
		BoundSubject:   wire.BoundSubject,
		BoundClaims:    wire.BoundClaims,
		BoundAudiences: wire.BoundAudiences,
		Policies:       wire.Policies,
	}
	if wire.TTL != "" {
		d, err := time.ParseDuration(wire.TTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl: " + err.Error()})
			return
		}
		role.TTL = d
	}

	if err := s.core.PutJWTRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// GET /v1/auth/jwt/role/{name}
func (s *Server) getJWTRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	role, err := s.core.GetJWTRole(r.Context(), name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/auth/jwt/role/{name}
func (s *Server) deleteJWTRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.DeleteJWTRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/auth/jwt/role/
func (s *Server) listJWTRoles(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.ListJWTRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	// Strip storage prefix from keys.
	for i, n := range names {
		names[i] = strings.TrimPrefix(n, "auth/jwt/roles/")
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}
