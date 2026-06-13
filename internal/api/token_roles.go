package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/policy"
	"github.com/NAGenaev/tuck/internal/token"
)

type putTokenRoleReq struct {
	Policies  []string `json:"policies"`
	TTL       string   `json:"ttl"`        // e.g. "24h"
	MaxTTL    string   `json:"max_ttl"`    // e.g. "168h"
	MaxUses   int      `json:"max_uses"`
	Renewable bool     `json:"renewable"`
	Period    string   `json:"period"`     // e.g. "4h"
}

// PUT /v1/auth/token/roles/{name}
func (s *Server) putTokenRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token/roles/"+name, policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req putTokenRoleReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	role := &token.Role{Name: name, Policies: req.Policies, Renewable: req.Renewable, MaxUses: req.MaxUses}
	if req.TTL != "" {
		d, err := time.ParseDuration(req.TTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl: " + err.Error()})
			return
		}
		role.TTL = d
	}
	if req.MaxTTL != "" {
		d, err := time.ParseDuration(req.MaxTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid max_ttl: " + err.Error()})
			return
		}
		role.MaxTTL = d
	}
	if req.Period != "" {
		d, err := time.ParseDuration(req.Period)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid period: " + err.Error()})
			return
		}
		role.Period = d
	}
	if err := s.core.PutTokenRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/auth/token/roles/{name}
func (s *Server) getTokenRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token/roles/"+name, policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	role, err := s.core.GetTokenRole(r.Context(), name)
	if err != nil {
		if errors.Is(err, token.ErrRoleNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/auth/token/roles/{name}
func (s *Server) deleteTokenRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token/roles/"+name, policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.DeleteTokenRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/auth/token/roles/
func (s *Server) listTokenRoles(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token/roles", policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	names, err := s.core.ListTokenRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// POST /v1/auth/token/roles/{role}/create
func (s *Server) createTokenFromRole(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("role")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token/roles/"+roleName+"/create", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&body)

	tok, err := s.core.CreateTokenFromRole(r.Context(), roleName, body.DisplayName)
	if err != nil {
		if errors.Is(err, token.ErrRoleNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tok)
}
