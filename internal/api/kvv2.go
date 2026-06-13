package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strconv"
	"unicode/utf8"

	"github.com/NAGenaev/tuck/internal/policy"
)

func v2EnforcePath(p string) string {
	return "secret/" + path.Clean("/"+p)[1:]
}

// PUT /v2/secret/{path...}
// Body: raw bytes. Optional query param ?cas=N for check-and-set.
func (s *Server) v2WriteSecret(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), v2EnforcePath(p), policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var cas *int
	if casStr := r.URL.Query().Get("cas"); casStr != "" {
		n, err := strconv.Atoi(casStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cas parameter"})
			return
		}
		cas = &n
	}
	ver, err := s.core.KVv2(nsFromCtx(r.Context())).Write(r.Context(), p, body, cas)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": ver, "path": p})
}

// GET /v2/secret/{path...}?version=N
func (s *Server) v2ReadSecret(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), v2EnforcePath(p), policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	version := 0
	if vStr := r.URL.Query().Get("version"); vStr != "" {
		n, err := strconv.Atoi(vStr)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid version parameter"})
			return
		}
		version = n
	}
	val, vm, err := s.core.KVv2(nsFromCtx(r.Context())).Read(r.Context(), p, version)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if vm == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	resp := map[string]any{
		"path":     p,
		"version":  vm.Version,
		"metadata": vm,
	}
	if val == nil {
		resp["deleted"] = true
	} else if utf8.Valid(val) {
		resp["value"] = string(val)
	} else {
		resp["value"] = base64.StdEncoding.EncodeToString(val)
		resp["encoding"] = "base64"
	}
	writeJSON(w, http.StatusOK, resp)
}

// DELETE /v2/secret/{path...}?versions=1,2,3  (soft-delete)
func (s *Server) v2DeleteSecret(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), v2EnforcePath(p), policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	versions := parseVersionList(r.URL.Query().Get("versions"))
	if len(versions) == 0 {
		// Default: soft-delete current version.
		meta, err := s.core.KVv2(nsFromCtx(r.Context())).GetMeta(r.Context(), p)
		if err != nil || meta == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		versions = []int{meta.CurrentVersion}
	}
	if err := s.core.KVv2(nsFromCtx(r.Context())).SoftDelete(r.Context(), p, versions); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /v2/secret/undelete/{path...}
// Body: {"versions":[1,2]}
func (s *Server) v2Undelete(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), v2EnforcePath(p), policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	versions, err := parseVersionBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.core.KVv2(nsFromCtx(r.Context())).Undelete(r.Context(), p, versions); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /v2/secret/destroy/{path...}
// Body: {"versions":[1,2]}
func (s *Server) v2Destroy(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), v2EnforcePath(p), policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	versions, err := parseVersionBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.core.KVv2(nsFromCtx(r.Context())).Destroy(r.Context(), p, versions); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v2/secret/metadata/{path...}
func (s *Server) v2GetMeta(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), v2EnforcePath(p), policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	meta, err := s.core.KVv2(nsFromCtx(r.Context())).GetMeta(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	if meta == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": p, "metadata": meta})
}

// PUT /v2/secret/metadata/{path...}
// Body: {"max_versions":10}
func (s *Server) v2UpdateMeta(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), v2EnforcePath(p), policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		MaxVersions int `json:"max_versions"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := s.core.KVv2(nsFromCtx(r.Context())).UpdateMeta(r.Context(), p, body.MaxVersions); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /v2/secret/metadata/{path...}  — destroy all versions + metadata
func (s *Server) v2DeleteMeta(w http.ResponseWriter, r *http.Request) {
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), v2EnforcePath(p), policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.KVv2(nsFromCtx(r.Context())).DeleteAll(r.Context(), p); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v2/secret/metadata/{path...}
func (s *Server) v2ListMeta(w http.ResponseWriter, r *http.Request) {
	prefix := r.PathValue("path")
	enforcePath := "secret/"
	if prefix != "" {
		enforcePath = v2EnforcePath(prefix)
	}
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), enforcePath, policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	keys, err := s.core.KVv2(nsFromCtx(r.Context())).List(r.Context(), prefix)
	if err != nil {
		writeErr(w, err)
		return
	}
	if keys == nil {
		keys = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func parseVersionBody(r *http.Request) ([]int, error) {
	var body struct {
		Versions []int `json:"versions"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&body); err != nil {
		return nil, err
	}
	return body.Versions, nil
}

func parseVersionList(s string) []int {
	if s == "" {
		return nil
	}
	var out []int
	for _, part := range splitComma(s) {
		n, err := strconv.Atoi(part)
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// v2ListSecrets handles LIST /v2/secret/{path...} — returns keys without version info.
func (s *Server) v2ListSecrets(w http.ResponseWriter, r *http.Request) {
	s.v2ListMeta(w, r)
}
