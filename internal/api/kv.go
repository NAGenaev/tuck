package api

import (
	"encoding/base64"
	"io"
	"net/http"
	"path"
	"unicode/utf8"

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
	val, ok, err := s.core.GetSecret(r.Context(), nsFromCtx(r.Context()), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	// API-1: binary-safe response — base64-encode when the value is not valid UTF-8.
	if utf8.Valid(val) {
		writeJSON(w, http.StatusOK, map[string]string{"path": p, "value": string(val)})
	} else {
		writeJSON(w, http.StatusOK, map[string]any{
			"path":     p,
			"value":    base64.StdEncoding.EncodeToString(val),
			"encoding": "base64",
		})
	}
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
	if err := s.core.PutSecret(r.Context(), nsFromCtx(r.Context()), p, body); err != nil {
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
	if err := s.core.DeleteSecret(r.Context(), nsFromCtx(r.Context()), p); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listSecrets handles LIST /v1/secret/{path...}.
// Returns {"keys": [...]} with all secret paths under the given prefix.
func (s *Server) listSecrets(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("path")
	enforcePath := "secret/"
	if prefix != "" {
		enforcePath = secretEnforcePath(prefix)
	}
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), enforcePath, policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	keys, err := s.core.ListSecrets(r.Context(), nsFromCtx(r.Context()), prefix)
	if err != nil {
		writeErr(w, err)
		return
	}
	if keys == nil {
		keys = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}
