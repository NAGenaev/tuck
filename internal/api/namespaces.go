package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

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
		namespace.Metadata
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	ns, err := s.core.CreateNamespaceWithMetadata(r.Context(), body.Name, body.Metadata)
	if err != nil {
		if errors.Is(err, namespace.ErrInvalidName) || errors.Is(err, namespace.ErrInvalidMetadata) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ns)
}

// GET /v1/sys/namespaces/{name...}
// The name itself may contain "/" (organizational naming convention only —
// see namespace.ValidateName); {name...} therefore captures the whole
// remainder of the path, so the former separate "GET .../{name}/stats" route
// would be ambiguous with it and is folded in here instead: a name ending in
// "/stats" is first tried as the stats sub-resource of the namespace named by
// the rest of the path, falling back to a plain get of the literal name (in
// the unlikely case a namespace segment is itself literally called "stats")
// only when that namespace does not exist.
func (s *Server) getNamespace(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if rest, ok := strings.CutSuffix(name, "/stats"); ok {
		if handled := s.namespaceStatsHandler(w, r, rest); handled {
			return
		}
	}
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

// DELETE /v1/sys/namespaces/{name...}
// Deletes exactly the one flat namespace entry named by the full path
// (slashes and all); it never cascades to any other namespace, including
// ones that merely share a "/"-separated prefix segment.
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
	resp := map[string]any{"keys": names}
	// Additive: per-namespace metadata, and with ?stats=true the object counts
	// (limited to what the caller may list).
	details, err := s.namespaceDetails(r.Context(), names, r.URL.Query().Get("stats") == "true")
	if err != nil {
		writeErr(w, err)
		return
	}
	resp["namespaces"] = details
	writeJSON(w, http.StatusOK, resp)
}
