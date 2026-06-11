// Package api exposes Tuck's HTTP interface.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/core"
)

const maxBodyBytes = 1 << 20 // 1 MiB

type contextKey int

const tokenCtxKey contextKey = iota

// Server adapts a core.Core to HTTP.
type Server struct {
	core *core.Core
}

// New returns an HTTP server over the given core.
func New(c *core.Core) *Server { return &Server{core: c} }

// Handler builds the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/health", s.health)

	mux.HandleFunc("GET /v1/secret/{path...}", s.requireToken(s.getSecret))
	mux.HandleFunc("PUT /v1/secret/{path...}", s.requireToken(s.putSecret))
	mux.HandleFunc("DELETE /v1/secret/{path...}", s.requireToken(s.deleteSecret))

	mux.HandleFunc("POST /v1/auth/token", s.requireToken(s.createToken))
	mux.HandleFunc("GET /v1/auth/token/{id}", s.requireToken(s.lookupToken))
	mux.HandleFunc("DELETE /v1/auth/token/{id}", s.requireToken(s.revokeToken))

	mux.HandleFunc("PUT /v1/policy/{name}", s.requireToken(s.putPolicy))
	mux.HandleFunc("GET /v1/policy/{name}", s.requireToken(s.getPolicy))
	mux.HandleFunc("DELETE /v1/policy/{name}", s.requireToken(s.deletePolicy))

	return mux
}

// requireToken extracts and validates X-Tuck-Token, then stores the token ID
// in context. Returns 401 on missing/invalid token, 503 if the barrier is sealed.
func (s *Server) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Tuck-Token")
		if id == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing X-Tuck-Token header"})
			return
		}
		if _, err := s.core.Authenticate(r.Context(), id); err != nil {
			if errors.Is(err, barrier.ErrSealed) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sealed"})
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), tokenCtxKey, id)))
	}
}

func tokenFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(tokenCtxKey).(string)
	return id
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sealed": s.core.Sealed()})
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, barrier.ErrSealed):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sealed"})
	case errors.Is(err, core.ErrTokenInvalid):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
	case errors.Is(err, core.ErrUnauthorized):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
