package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/NAGenaev/tuck/internal/dynamic/transit"
)

// POST /v1/transit/keys/{name}
// Body: {"type":"aes256-gcm96"} — type is optional, defaults to aes256-gcm96.
func (s *Server) transitCreateKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key name required"})
		return
	}
	var req struct {
		Type string `json:"type"`
	}
	// Body is optional — ignore parse errors.
	body, _ := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	_ = json.Unmarshal(body, &req)

	if err := s.core.TransitCreateKey(r.Context(), name, req.Type); err != nil {
		if errors.Is(err, transit.ErrUnsupportedType) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeErr(w, err)
		return
	}
	k, err := s.core.TransitGetKey(r.Context(), name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

// GET /v1/transit/keys/{name}
func (s *Server) transitGetKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	k, err := s.core.TransitGetKey(r.Context(), name)
	if err != nil {
		if errors.Is(err, transit.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

// DELETE /v1/transit/keys/{name}
func (s *Server) transitDeleteKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.TransitDeleteKey(r.Context(), name); err != nil {
		switch {
		case errors.Is(err, transit.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
		case errors.Is(err, transit.ErrKeyNotDeletable):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		default:
			writeErr(w, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/transit/keys/
func (s *Server) transitListKeys(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.TransitListKeys(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// POST /v1/transit/keys/{name}/rotate
func (s *Server) transitRotate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.TransitRotate(r.Context(), name); err != nil {
		if errors.Is(err, transit.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
			return
		}
		writeErr(w, err)
		return
	}
	k, err := s.core.TransitGetKey(r.Context(), name)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

// POST /v1/transit/keys/{name}/config
// Body: {"min_decryption_version":2,"deletable":true}
func (s *Server) transitUpdateKey(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		MinVersion int  `json:"min_decryption_version"`
		Deletable  bool `json:"deletable"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.core.TransitUpdateKey(r.Context(), name, req.MinVersion, req.Deletable); err != nil {
		if errors.Is(err, transit.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /v1/transit/encrypt/{name}
// Body: {"plaintext":"<base64url-encoded-data>"}
// Response: {"ciphertext":"vault:v1:..."}
func (s *Server) transitEncrypt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Plaintext string `json:"plaintext"` // base64url
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Plaintext == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plaintext (base64url) required"})
		return
	}
	plain, err := base64.RawURLEncoding.DecodeString(req.Plaintext)
	if err != nil {
		// Try standard base64 as fallback.
		plain, err = base64.StdEncoding.DecodeString(req.Plaintext)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plaintext must be base64url-encoded"})
			return
		}
	}
	ct, err := s.core.TransitEncrypt(r.Context(), name, plain)
	if err != nil {
		transitErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ciphertext": ct})
}

// POST /v1/transit/decrypt/{name}
// Body: {"ciphertext":"vault:v1:..."}
// Response: {"plaintext":"<base64url-encoded-data>"}
func (s *Server) transitDecrypt(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Ciphertext == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ciphertext required"})
		return
	}
	plain, err := s.core.TransitDecrypt(r.Context(), name, req.Ciphertext)
	if err != nil {
		transitErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"plaintext": base64.RawURLEncoding.EncodeToString(plain),
	})
}

// POST /v1/transit/rewrap/{name}
// Body: {"ciphertext":"vault:v1:..."}
// Response: {"ciphertext":"vault:v2:..."} — re-encrypted with latest key version.
func (s *Server) transitRewrap(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Ciphertext == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ciphertext required"})
		return
	}
	newCT, err := s.core.TransitRewrap(r.Context(), name, req.Ciphertext)
	if err != nil {
		transitErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ciphertext": newCT})
}

// POST /v1/transit/sign/{name}[/{hash_algorithm}]
// Body: {"input":"<base64url>","hash_algorithm":"sha2-256"}
// Response: {"signature":"vault:v1:..."}
func (s *Server) transitSign(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Input         string `json:"input"`           // base64url
		HashAlgorithm string `json:"hash_algorithm"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Input == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input (base64url) required"})
		return
	}
	input, err := base64.RawURLEncoding.DecodeString(req.Input)
	if err != nil {
		input, err = base64.StdEncoding.DecodeString(req.Input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input must be base64url-encoded"})
			return
		}
	}
	sig, err := s.core.TransitSign(r.Context(), name, input, req.HashAlgorithm)
	if err != nil {
		transitErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"signature": sig})
}

// POST /v1/transit/verify/{name}[/{hash_algorithm}]
// Body: {"input":"<base64url>","signature":"vault:v1:...","hash_algorithm":"sha2-256"}
// Response: {"valid":true}
func (s *Server) transitVerify(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Input         string `json:"input"`
		Signature     string `json:"signature"`
		HashAlgorithm string `json:"hash_algorithm"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Input == "" || req.Signature == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input and signature required"})
		return
	}
	input, err := base64.RawURLEncoding.DecodeString(req.Input)
	if err != nil {
		input, err = base64.StdEncoding.DecodeString(req.Input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input must be base64url-encoded"})
			return
		}
	}
	valid, err := s.core.TransitVerify(r.Context(), name, input, req.Signature, req.HashAlgorithm)
	if err != nil {
		transitErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": valid})
}

// POST /v1/transit/hmac/{name}[/{algorithm}]
// Body: {"input":"<base64url>","algorithm":"sha2-256"}
// Response: {"hmac":"vault:v1:..."}
func (s *Server) transitHMAC(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Input     string `json:"input"`
		Algorithm string `json:"algorithm"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Input == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input (base64url) required"})
		return
	}
	input, err := base64.RawURLEncoding.DecodeString(req.Input)
	if err != nil {
		input, err = base64.StdEncoding.DecodeString(req.Input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input must be base64url-encoded"})
			return
		}
	}
	mac, err := s.core.TransitHMAC(r.Context(), name, input, req.Algorithm)
	if err != nil {
		transitErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"hmac": mac})
}

// transitErr maps transit errors to HTTP status codes.
func transitErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, transit.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "key not found"})
	case errors.Is(err, transit.ErrNotEncryptionKey):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, transit.ErrNotSigningKey):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, transit.ErrInvalidCiphertext):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ciphertext or signature format"})
	case errors.Is(err, transit.ErrDecryptFailed):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decryption failed"})
	case errors.Is(err, transit.ErrKeyVersionTooOld):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeErr(w, err)
	}
}
