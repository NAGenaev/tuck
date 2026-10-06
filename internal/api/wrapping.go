package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/audit"
	"github.com/NAGenaev/tuck/internal/wrapping"
)

// POST /v1/sys/wrapping/wrap
// Body: {"data": {...}, "ttl": "5m", "creation_path": "optional label"}
// Response: {"token": "tuck_wrap_...", "expires_at": "..."}
func (s *Server) wrapPayload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var wire struct {
		Data json.RawMessage `json:"data"`
		TTL  string         `json:"ttl"`
		// CreationPath is an optional free-text label of where the data came from;
		// it is shown in the wrapped-response list and is never interpreted.
		CreationPath string `json:"creation_path"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if len(wire.Data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "data is required"})
		return
	}

	var ttl time.Duration
	if wire.TTL != "" {
		d, err := time.ParseDuration(wire.TTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl"})
			return
		}
		ttl = d
	}

	meta := wrapping.Meta{Namespace: nsFromCtx(r.Context()), CreationPath: strings.TrimSpace(wire.CreationPath)}
	if tok, aerr := s.core.Authenticate(r.Context(), tokenFromCtx(r.Context())); aerr == nil {
		meta.CreatorAccessor = tok.Accessor
		meta.CreatorName = tok.DisplayName
		meta.CreatorFingerprint = audit.Fingerprint(tok.ID)
	}
	token, expiresAt, err := s.core.WrapPayloadWithMeta(r.Context(), wire.Data, ttl, meta)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": expiresAt,
	})
}

// POST /v1/sys/wrapping/unwrap
// Body: {"token": "tuck_wrap_..."}
// Response: {"data": {...}}
func (s *Server) unwrapPayload(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var wire struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if wire.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}

	data, err := s.core.UnwrapPayload(r.Context(), wire.Token)
	if err != nil {
		switch {
		case errors.Is(err, wrapping.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "wrapping token not found or already used"})
		case errors.Is(err, wrapping.ErrExpired):
			writeJSON(w, http.StatusGone, map[string]string{"error": "wrapping token has expired"})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// POST /v1/sys/wrapping/lookup
// Body: {"token": "tuck_wrap_..."}
// Response: {"creation_time":"...","expires_at":"...","creation_ttl":300}
func (s *Server) lookupWrappingToken(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var wire struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if wire.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}

	info, err := s.core.LookupWrappingToken(r.Context(), wire.Token)
	if err != nil {
		switch {
		case errors.Is(err, wrapping.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "wrapping token not found"})
		case errors.Is(err, wrapping.ErrExpired):
			writeJSON(w, http.StatusGone, map[string]string{"error": "wrapping token has expired"})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// DELETE /v1/sys/wrapping/revoke
// Body: {"token": "tuck_wrap_..."}
func (s *Server) revokeWrappingToken(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var wire struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if wire.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}

	if err := s.core.RevokeWrappingToken(r.Context(), wire.Token); err != nil {
		if errors.Is(err, wrapping.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "wrapping token not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
