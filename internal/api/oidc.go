package api

import (
	"errors"
	"io"
	"net/http"

	"github.com/NAGenaev/tuck/internal/audit"
	authjwt "github.com/NAGenaev/tuck/internal/auth/jwt"
	"github.com/NAGenaev/tuck/internal/clientip"
)

const (
	maxOIDCCodeLen  = 4096
	maxOIDCStateLen = 256
)

func (s *Server) oidcAllowed(w http.ResponseWriter, r *http.Request) bool {
	ip := clientip.FromRequest(r)
	if !s.oidcLimiter.Allow(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
		return false
	}
	return true
}

// GET /v1/auth/oidc/status (unauthenticated)
// Tells the login page whether to offer the SSO button. Reveals nothing but
// a boolean; a sealed or unconfigured server reads as disabled.
func (s *Server) oidcStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": s.core.OIDCEnabled(r.Context())})
}

// GET /v1/auth/oidc/auth-url (unauthenticated)
// Starts a browser login and returns {"url": "<IdP authorization URL>", "state": "..."}.
func (s *Server) oidcAuthURL(w http.ResponseWriter, r *http.Request) {
	if !s.oidcAllowed(w, r) {
		return
	}
	authURL, state, err := s.core.OIDCAuthURL(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, authjwt.ErrOIDCNotConfigured):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "OIDC login not configured"})
		case errors.Is(err, authjwt.ErrTooManyPending):
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many pending logins, try again later"})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": authURL, "state": state})
}

// POST /v1/auth/oidc/callback (unauthenticated)
// Body: {"code": "...", "state": "..."}
// Completes the login started by auth-url and returns a Tuck token.
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if !s.oidcAllowed(w, r) {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if !decodeJSON(w, body, &req) {
		return
	}
	if req.Code == "" || req.State == "" || len(req.Code) > maxOIDCCodeLen || len(req.State) > maxOIDCStateLen {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code and state required"})
		return
	}

	tok, err := s.core.LoginOIDC(r.Context(), req.Code, req.State)
	if err != nil {
		switch {
		case errors.Is(err, authjwt.ErrInvalidState):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired state"})
		case errors.Is(err, authjwt.ErrCodeExchange):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "authorization code rejected by identity provider"})
		case errors.Is(err, authjwt.ErrIdPUnavailable):
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "identity provider unavailable"})
		case errors.Is(err, authjwt.ErrInvalidToken):
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired id_token"})
		case errors.Is(err, authjwt.ErrNoRole):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "no matching role for token claims"})
		case errors.Is(err, authjwt.ErrOIDCNotConfigured):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "OIDC login not configured"})
		default:
			writeErr(w, err)
		}
		return
	}
	audit.SetIdentity(r.Context(), tok.DisplayName, tok.EntityID)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      tok.ID,
		"policies":   tok.Policies,
		"expires_at": tok.ExpiresAt,
	})
}
