package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/NAGenaev/tuck/internal/cubbyhole"
)

// GET /v1/cubbyhole/{path...}
func (s *Server) cubbyholeGet(w http.ResponseWriter, r *http.Request) {
	tokenID := tokenFromCtx(r.Context())
	path := r.PathValue("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}
	data, err := s.core.CubbyholeGet(r.Context(), tokenID, path)
	if err != nil {
		if errors.Is(err, cubbyhole.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// PUT /v1/cubbyhole/{path...}
// Body: any JSON object
func (s *Server) cubbyholePut(w http.ResponseWriter, r *http.Request) {
	tokenID := tokenFromCtx(r.Context())
	path := r.PathValue("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.core.CubbyholePut(r.Context(), tokenID, path, data); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /v1/cubbyhole/{path...}
func (s *Server) cubbyholeDelete(w http.ResponseWriter, r *http.Request) {
	tokenID := tokenFromCtx(r.Context())
	path := r.PathValue("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}
	if err := s.core.CubbyholeDelete(r.Context(), tokenID, path); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/cubbyhole/{path...}
func (s *Server) cubbyholeList(w http.ResponseWriter, r *http.Request) {
	tokenID := tokenFromCtx(r.Context())
	prefix := r.PathValue("path")
	keys, err := s.core.CubbyholeList(r.Context(), tokenID, prefix)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}
