package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"time"
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
	entry, err := s.core.GetSecretEntry(r.Context(), nsFromCtx(r.Context()), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	if entry == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	resp := map[string]any{"path": p}
	if entry.Metadata != nil {
		resp["metadata"] = entry.Metadata
	}
	if !entry.CreatedAt.IsZero() {
		resp["created_at"] = entry.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !entry.ExpiresAt.IsZero() {
		resp["expires_at"] = entry.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if utf8.Valid(entry.Value) {
		resp["value"] = string(entry.Value)
	} else {
		resp["value"] = base64.StdEncoding.EncodeToString(entry.Value)
		resp["encoding"] = "base64"
	}
	writeJSON(w, http.StatusOK, resp)
}

type putSecretReq struct {
	Value    json.RawMessage   `json:"value"`    // raw bytes or base64; required
	TTL      string            `json:"ttl"`      // e.g. "24h"; empty = no expiry
	Metadata map[string]string `json:"metadata"` // optional labels
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

	// Detect whether body is a JSON object with a "value" key (new API) or
	// raw bytes (legacy API). Try JSON first.
	var req putSecretReq
	if jsonErr := json.Unmarshal(body, &req); jsonErr == nil && req.Value != nil {
		// New structured API.
		var value []byte
		// req.Value may be a JSON string or a base64-encoded string.
		var strVal string
		if jsonErr2 := json.Unmarshal(req.Value, &strVal); jsonErr2 == nil {
			value = []byte(strVal)
		} else {
			// Treat the raw JSON value bytes as the payload.
			value = []byte(req.Value)
		}

		var ttl time.Duration
		if req.TTL != "" {
			if ttl, err = time.ParseDuration(req.TTL); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl: " + err.Error()})
				return
			}
		}
		if err := s.core.PutSecretWithMeta(r.Context(), nsFromCtx(r.Context()), p, value, req.Metadata, ttl); err != nil {
			writeErr(w, err)
			return
		}
	} else {
		// Legacy raw-bytes API: store body directly with no TTL or metadata.
		if err := s.core.PutSecret(r.Context(), nsFromCtx(r.Context()), p, body); err != nil {
			writeErr(w, err)
			return
		}
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
