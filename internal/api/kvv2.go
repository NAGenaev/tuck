package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/NAGenaev/tuck/internal/audit"
	"github.com/NAGenaev/tuck/internal/kvv2"
	"github.com/NAGenaev/tuck/internal/policy"
)

func v2EnforcePath(p string) string {
	return "secret/" + path.Clean("/"+p)[1:]
}

// withKVAuthor tags ctx with the identity of the calling token so KV v2 can
// record who wrote a version. Only the display name, accessor, entity id and
// the audit-log fingerprint are stored, never the token ID itself.
func (s *Server) withKVAuthor(ctx context.Context) context.Context {
	id := tokenFromCtx(ctx)
	tok, err := s.core.Authenticate(ctx, id)
	if err != nil {
		return ctx
	}
	name := tok.DisplayName
	if len(name) > 128 {
		name = strings.ToValidUTF8(name[:128], "")
	}
	return kvv2.WithAuthor(ctx, kvv2.Author{
		DisplayName: name,
		Accessor:    tok.Accessor,
		Fingerprint: audit.Fingerprint(id),
		EntityID:    tok.EntityID,
	})
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
	ctx := s.withKVAuthor(r.Context())
	ver, err := s.core.KVv2(nsFromCtx(ctx)).Write(ctx, p, body, cas)
	if err != nil {
		switch {
		case errors.Is(err, kvv2.ErrCASMismatch):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		case errors.Is(err, kvv2.ErrCASRequired):
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "cas_required": true})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": ver, "path": p})
}

// GET /v2/secret/{path...}?version=N  (or ?list=true to list keys)
func (s *Server) v2ReadSecret(w http.ResponseWriter, r *http.Request) {
	if wantsList(r) {
		s.v2ListSecrets(w, r)
		return
	}
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

// GET /v2/secret/metadata/{path...}  (or ?list=true to list keys)
func (s *Server) v2GetMeta(w http.ResponseWriter, r *http.Request) {
	if wantsList(r) {
		s.v2ListMeta(w, r)
		return
	}
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
	def := s.core.KVv2(nsFromCtx(r.Context())).MountDefaults(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     p,
		"metadata": meta,
		// cas_required of the secret OR the mount: what a writer actually faces.
		"cas_required_effective": meta.CASRequired || def.CASRequired,
		"mount_options":          map[string]any{"max_versions": def.MaxVersions, "cas_required": def.CASRequired},
	})
}

// PUT   /v2/secret/metadata/{path...}  fields present are applied; custom_metadata replaces
// PATCH /v2/secret/metadata/{path...}  custom_metadata is merged, a null value deletes a key
// Body: {"max_versions":10,"cas_required":true,"custom_metadata":{"owner":"team-a"}}
func (s *Server) v2UpdateMeta(w http.ResponseWriter, r *http.Request) {
	s.v2WriteMeta(w, r, true)
}

func (s *Server) v2PatchMeta(w http.ResponseWriter, r *http.Request) {
	s.v2WriteMeta(w, r, false)
}

func (s *Server) v2WriteMeta(w http.ResponseWriter, r *http.Request, replace bool) {
	p := r.PathValue("path")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), v2EnforcePath(p), policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		MaxVersions    *int               `json:"max_versions"`
		CASRequired    *bool              `json:"cas_required"`
		CustomMetadata map[string]*string `json:"custom_metadata"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	_, err := s.core.KVv2(nsFromCtx(r.Context())).UpdateMetadata(r.Context(), p, kvv2.MetaUpdate{
		MaxVersions:    body.MaxVersions,
		CASRequired:    body.CASRequired,
		CustomMetadata: body.CustomMetadata,
		Replace:        replace,
	})
	if err != nil {
		if errors.Is(err, kvv2.ErrInvalidMetadata) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
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
	q := r.URL.Query()
	detailed, totals := q.Get("detailed") == "true", q.Get("totals") == "true"
	if detailed || totals {
		s.v2ListDetailed(w, r, prefix, detailed, totals)
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

// v2ListDetailed serves ?detailed=true (per-key current version, time/author of
// the last write, flags) and ?totals=true (counts for the whole subtree). Only
// metadata is read, never secret values. Details and totals only cover
// secrets whose metadata the token may read; the rest is listed by name only
// (restricted) because "list" alone must not reveal versions or authors.
func (s *Server) v2ListDetailed(w http.ResponseWriter, r *http.Request, prefix string, detailed, totals bool) {
	canRead, err := s.core.CapabilityChecker(r.Context(), tokenFromCtx(r.Context()), policy.CapRead)
	if err != nil {
		writeErr(w, err)
		return
	}
	entries, tot, err := s.core.KVv2(nsFromCtx(r.Context())).ListDetailed(r.Context(), prefix, kvv2.ListOptions{
		Totals: totals,
		Allow:  func(p string) bool { return canRead(v2EnforcePath(p)) },
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, e.Name)
	}
	resp := map[string]any{"keys": keys}
	if detailed {
		resp["entries"] = entries
	}
	if totals {
		resp["totals"] = tot
	}
	writeJSON(w, http.StatusOK, resp)
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
