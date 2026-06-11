// Package api exposes Tuck's HTTP interface. Milestone 0 implements a minimal
// KV: PUT/GET/DELETE on /v1/secret/<path>, plus a seal-status health endpoint.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/core"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// Server adapts a core.Core to HTTP.
type Server struct {
	core *core.Core
}

// New returns an HTTP server over the given core.
func New(c *core.Core) *Server { return &Server{core: c} }

// Handler builds the route table. Uses net/http pattern routing (Go 1.22+),
// so there is no web framework in the dependency tree.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("GET /v1/secret/{path...}", s.getSecret)
	mux.HandleFunc("PUT /v1/secret/{path...}", s.putSecret)
	mux.HandleFunc("DELETE /v1/secret/{path...}", s.deleteSecret)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sealed": s.core.Sealed()})
}

func (s *Server) getSecret(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	val, ok, err := s.core.GetSecret(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": p, "value": string(val)})
}

func (s *Server) putSecret(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	if err := s.core.PutSecret(r.Context(), p, body); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteSecret(w http.ResponseWriter, r *http.Request) {
	if err := s.core.DeleteSecret(r.Context(), r.PathValue("path")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, barrier.ErrSealed) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sealed"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
