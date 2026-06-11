package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	dynTOTP "github.com/NAGenaev/tuck/internal/dynamic/totp"
)

// POST /v1/totp/keys/{name}
// Body: {"issuer":"ACME","account":"user@example.com","algorithm":"sha1","digits":6,"period":30,"secret":"BASE32_OPTIONAL"}
// Response: KeyInfo + secret (returned once).
func (s *Server) totpCreateKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key name required"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req dynTOTP.CreateKeyRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
	}

	result, err := s.core.TOTPCreateKey(r.Context(), name, req)
	if err != nil {
		if errors.Is(err, dynTOTP.ErrInvalidSecret) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// GET /v1/totp/keys/{name}
// Returns key metadata and the otpauth:// URL. Never returns the raw secret.
func (s *Server) totpGetKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	info, err := s.core.TOTPGetKey(r.Context(), name)
	if err != nil {
		if errors.Is(err, dynTOTP.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// DELETE /v1/totp/keys/{name}
func (s *Server) totpDeleteKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.TOTPDeleteKey(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/totp/keys/
func (s *Server) totpListKeys(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.TOTPListKeys(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// GET /v1/totp/code/{name}
// Generates the current TOTP code for the named key.
// Response: {"code":"123456","valid_until":"2026-06-11T12:00:30Z"}
func (s *Server) totpGenerateCode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	result, err := s.core.TOTPGenerateCode(r.Context(), name)
	if err != nil {
		if errors.Is(err, dynTOTP.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// POST /v1/totp/code/{name}
// Validates a TOTP code submitted by the user.
// Body: {"code":"123456"}
// Response: {"valid":true} or {"valid":false}
func (s *Server) totpValidateCode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code required"})
		return
	}

	valid, err := s.core.TOTPValidateCode(r.Context(), name, req.Code)
	if err != nil {
		if errors.Is(err, dynTOTP.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"valid": valid})
}
