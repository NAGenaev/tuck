package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/NAGenaev/tuck/internal/namespace"
	"github.com/NAGenaev/tuck/internal/policy"
)

// POST /v1/sys/namespaces
func (s *Server) createNamespace(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/namespaces", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	ns, err := s.core.CreateNamespace(r.Context(), body.Name)
	if err != nil {
		if errors.Is(err, namespace.ErrInvalidName) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ns)
}

// GET /v1/sys/namespaces/{name}
func (s *Server) getNamespace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/namespaces/"+name, policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	ns, err := s.core.GetNamespace(r.Context(), name)
	if err != nil {
		if errors.Is(err, namespace.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "namespace not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ns)
}

// DELETE /v1/sys/namespaces/{name}
func (s *Server) deleteNamespace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/namespaces/"+name, policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.DeleteNamespace(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/sys/namespaces/
func (s *Server) listNamespaces(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/namespaces", policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	names, err := s.core.ListNamespaces(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}
