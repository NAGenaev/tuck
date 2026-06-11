package api

import (
	"io"
	"net/http"
	"path"

	"github.com/NAGenaev/tuck/internal/policy"
)

// secretEnforcePath builds the full logical path used for policy enforcement,
// mirroring the normalisation core.secretKey applies before storage.
func secretEnforcePath(p string) string {
	return "secret/" + path.Clean("/"+p)[1:]
}

func (s *Server) getSecret(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), secretEnforcePath(p), policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
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
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), secretEnforcePath(p), policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
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
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), secretEnforcePath(p), policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.DeleteSecret(r.Context(), p); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
