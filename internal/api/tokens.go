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

type createTokenReq struct {
	DisplayName string   `json:"display_name"`
	Policies    []string `json:"policies"`
	TTL         string   `json:"ttl"` // e.g. "24h", "" = never expires
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req createTokenReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	var ttl time.Duration
	if req.TTL != "" {
		if ttl, err = time.ParseDuration(req.TTL); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl: " + err.Error()})
			return
		}
	}
	if req.Policies == nil {
		req.Policies = []string{}
	}
	tok, err := s.core.CreateToken(r.Context(), req.DisplayName, req.Policies, ttl)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tok)
}

func (s *Server) lookupToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token/"+id, policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	tok, err := s.core.LookupToken(r.Context(), id)
	if err != nil {
		if errors.Is(err, token.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tok)
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token/"+id, policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.RevokeToken(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// renewToken handles POST /v1/auth/token/{id}/renew.
// Optional body: {"ttl": "24h"}. Default renewal TTL is 1h.
func (s *Server) renewToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token/"+id, policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	var req struct {
		TTL string `json:"ttl"`
	}
	body, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	_ = json.Unmarshal(body, &req)

	var ttl time.Duration
	if req.TTL != "" {
		var err error
		if ttl, err = time.ParseDuration(req.TTL); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl: " + err.Error()})
			return
		}
	}
	tok, err := s.core.RenewToken(r.Context(), id, ttl)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tok)
}

// lookupByAccessor handles POST /v1/auth/token/lookup-accessor.
// Body: {"accessor": "tuck_acc_..."}
func (s *Server) lookupByAccessor(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token", policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	var req struct {
		Accessor string `json:"accessor"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Accessor == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "accessor is required"})
		return
	}
	tok, err := s.core.LookupTokenByAccessor(r.Context(), req.Accessor)
	if err != nil {
		if errors.Is(err, token.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tok)
}

// revokeByAccessor handles DELETE /v1/auth/token/revoke-accessor.
// Body: {"accessor": "tuck_acc_..."}
func (s *Server) revokeByAccessor(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token", policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	var req struct {
		Accessor string `json:"accessor"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Accessor == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "accessor is required"})
		return
	}
	if err := s.core.RevokeTokenByAccessor(r.Context(), req.Accessor); err != nil {
		if errors.Is(err, token.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listTokens handles LIST /v1/auth/token/.
// Returns {"keys": [...]} with all token IDs.
func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/token", policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	ids, err := s.core.ListTokens(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": ids})
}
