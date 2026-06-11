package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	dynSSH "github.com/NAGenaev/tuck/internal/dynamic/ssh"
)

// POST /v1/ssh/generate/ca
// Body: {"key_type":"ed25519"}  (key_type optional, defaults to ed25519)
// Returns the CA public key in OpenSSH format for use in TrustedUserCAKeys.
func (s *Server) sshGenerateCA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyType string `json:"key_type"`
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	_ = json.Unmarshal(body, &req)

	pubKey, err := s.core.SSHGenerateCA(r.Context(), req.KeyType)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": pubKey})
}

// POST /v1/ssh/import/ca
// Body: {"private_key":"-----BEGIN PRIVATE KEY-----\n..."}
func (s *Server) sshImportCA(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		PrivateKey string `json:"private_key"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.PrivateKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "private_key (PEM) required"})
		return
	}
	if err := s.core.SSHImportCA(r.Context(), req.PrivateKey); err != nil {
		if errors.Is(err, dynSSH.ErrInvalidPEM) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /v1/ssh/ca/public-key — unauthenticated; for TrustedUserCAKeys on hosts.
func (s *Server) sshGetCAPublicKey(w http.ResponseWriter, r *http.Request) {
	pubKey, err := s.core.SSHGetCAPublicKey(r.Context())
	if err != nil {
		if errors.Is(err, dynSSH.ErrNoCA) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no SSH CA configured"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": pubKey})
}

// PUT /v1/ssh/roles/{name}
// Body: {"allowed_users":["ubuntu"],"default_ttl":"24h","max_ttl":"168h","cert_type":"user"}
func (s *Server) sshPutRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role name required"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var wire struct {
		AllowedUsers      []string          `json:"allowed_users"`
		DefaultExtensions map[string]string `json:"default_extensions"`
		CertType          string            `json:"cert_type"`
		DefaultTTL        string            `json:"default_ttl"`
		MaxTTL            string            `json:"max_ttl"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	role := &dynSSH.Role{
		Name:              name,
		AllowedUsers:      wire.AllowedUsers,
		DefaultExtensions: wire.DefaultExtensions,
		CertType:          wire.CertType,
	}
	if wire.DefaultTTL != "" {
		d, err := time.ParseDuration(wire.DefaultTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid default_ttl: " + err.Error()})
			return
		}
		role.DefaultTTL = d
	}
	if wire.MaxTTL != "" {
		d, err := time.ParseDuration(wire.MaxTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid max_ttl: " + err.Error()})
			return
		}
		role.MaxTTL = d
	}

	if err := s.core.SSHPutRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// GET /v1/ssh/roles/{name}
func (s *Server) sshGetRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	role, err := s.core.SSHGetRole(r.Context(), name)
	if err != nil {
		if errors.Is(err, dynSSH.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/ssh/roles/{name}
func (s *Server) sshDeleteRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.SSHDeleteRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/ssh/roles/
func (s *Server) sshListRoles(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.SSHListRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// POST /v1/ssh/sign/{role}
// Body: {"public_key":"ssh-ed25519 AAAA...","valid_principals":["ubuntu"],"ttl":"24h"}
// Response: {"serial":..., "signed_key":"ssh-ed25519-cert-v01@... ...","valid_after":"...","valid_before":"...","ttl":"24h0m0s"}
func (s *Server) sshSign(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("role")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		PublicKey       string   `json:"public_key"`
		ValidPrincipals []string `json:"valid_principals"`
		TTL             string   `json:"ttl"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.PublicKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "public_key required"})
		return
	}

	var ttl time.Duration
	if req.TTL != "" {
		d, err := time.ParseDuration(req.TTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl: " + err.Error()})
			return
		}
		ttl = d
	}

	signed, err := s.core.SSHSignPublicKey(r.Context(), roleName, req.PublicKey, req.ValidPrincipals, ttl)
	if err != nil {
		switch {
		case errors.Is(err, dynSSH.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
		case errors.Is(err, dynSSH.ErrNoCA):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no SSH CA configured"})
		case errors.Is(err, dynSSH.ErrPrincipalDenied):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, signed)
}
