package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/dynamic/pki"
)

// POST /v1/pki/generate/root
// Body: {"common_name":"...","ttl":"87600h","key_type":"ec","key_bits":256}
// Returns the CA certificate PEM.
func (s *Server) pkiGenerateRoot(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var wire struct {
		CommonName   string   `json:"common_name"`
		TTL          string   `json:"ttl"`
		KeyType      string   `json:"key_type"`
		KeyBits      int      `json:"key_bits"`
		Organization []string `json:"organization"`
		Country      []string `json:"country"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if wire.CommonName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "common_name required"})
		return
	}

	cfg := &pki.CAConfig{
		CommonName:   wire.CommonName,
		KeyType:      wire.KeyType,
		KeyBits:      wire.KeyBits,
		Organization: wire.Organization,
		Country:      wire.Country,
	}
	if wire.TTL != "" {
		d, err := time.ParseDuration(wire.TTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl: " + err.Error()})
			return
		}
		cfg.TTL = d
	}

	certPEM, err := s.core.GeneratePKICA(r.Context(), cfg)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"certificate": certPEM})
}

// POST /v1/pki/import/ca
// Body: {"cert_pem":"...","key_pem":"..."}
func (s *Server) pkiImportCA(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		CertPEM string `json:"cert_pem"`
		KeyPEM  string `json:"key_pem"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.CertPEM == "" || req.KeyPEM == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cert_pem and key_pem required"})
		return
	}
	if err := s.core.ImportPKICA(r.Context(), req.CertPEM, req.KeyPEM); err != nil {
		if errors.Is(err, pki.ErrInvalidPEM) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /v1/pki/ca/pem — unauthenticated; returns the CA cert PEM.
func (s *Server) pkiGetCACert(w http.ResponseWriter, r *http.Request) {
	certPEM, err := s.core.GetPKICACert(r.Context())
	if err != nil {
		if errors.Is(err, pki.ErrNoCA) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no CA configured"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"certificate": certPEM})
}

// GET /v1/pki/crl/pem — unauthenticated; returns the CRL PEM.
func (s *Server) pkiGetCRL(w http.ResponseWriter, r *http.Request) {
	crlPEM, err := s.core.GetPKICRL(r.Context())
	if err != nil {
		if errors.Is(err, pki.ErrNoCA) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no CA configured"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"crl": crlPEM})
}

// PUT /v1/pki/roles/{name}
// Body: {"allowed_domains":["example.com"],"allow_subdomains":true,"key_type":"ec","default_ttl":"72h","server_flag":true}
func (s *Server) pkiPutRole(w http.ResponseWriter, r *http.Request) {
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
		AllowedDomains  []string `json:"allowed_domains"`
		AllowSubdomains bool     `json:"allow_subdomains"`
		AllowIPSANs     bool     `json:"allow_ip_sans"`
		AllowLocalhost  bool     `json:"allow_localhost"`
		KeyType         string   `json:"key_type"`
		KeyBits         int      `json:"key_bits"`
		DefaultTTL      string   `json:"default_ttl"`
		MaxTTL          string   `json:"max_ttl"`
		ServerFlag      bool     `json:"server_flag"`
		ClientFlag      bool     `json:"client_flag"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	role := &pki.Role{
		Name:            name,
		AllowedDomains:  wire.AllowedDomains,
		AllowSubdomains: wire.AllowSubdomains,
		AllowIPSANs:     wire.AllowIPSANs,
		AllowLocalhost:  wire.AllowLocalhost,
		KeyType:         wire.KeyType,
		KeyBits:         wire.KeyBits,
		ServerFlag:      wire.ServerFlag,
		ClientFlag:      wire.ClientFlag,
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

	if err := s.core.PutPKIRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// GET /v1/pki/roles/{name}
func (s *Server) pkiGetRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	role, err := s.core.GetPKIRole(r.Context(), name)
	if err != nil {
		if errors.Is(err, pki.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/pki/roles/{name}
func (s *Server) pkiDeleteRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.DeletePKIRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/pki/roles/
func (s *Server) pkiListRoles(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.ListPKIRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// POST /v1/pki/issue/{role}
// Body: {"common_name":"api.example.com","alt_names":["api.example.com"],"ttl":"72h"}
// Returns: certificate + private_key + issuing_ca + serial + expires_at
func (s *Server) pkiIssueCert(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("role")
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		CommonName string   `json:"common_name"`
		AltNames   []string `json:"alt_names"`
		TTL        string   `json:"ttl"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.CommonName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "common_name required"})
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

	issued, err := s.core.IssuePKICert(r.Context(), roleName, req.CommonName, req.AltNames, ttl)
	if err != nil {
		switch {
		case errors.Is(err, pki.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
		case errors.Is(err, pki.ErrNoCA):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no CA configured"})
		case errors.Is(err, pki.ErrDomainDenied):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, issued)
}

// POST /v1/pki/revoke/{serial}
func (s *Server) pkiRevokeCert(w http.ResponseWriter, r *http.Request) {
	serial := r.PathValue("serial")
	if err := s.core.RevokePKICert(r.Context(), serial); err != nil {
		if errors.Is(err, pki.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "certificate not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// GET /v1/pki/certs/{serial}
func (s *Server) pkiGetCert(w http.ResponseWriter, r *http.Request) {
	serial := r.PathValue("serial")
	cr, err := s.core.GetPKICert(r.Context(), serial)
	if err != nil {
		if errors.Is(err, pki.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "certificate not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cr)
}

// LIST /v1/pki/certs/
func (s *Server) pkiListCerts(w http.ResponseWriter, r *http.Request) {
	serials, err := s.core.ListPKICerts(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": serials})
}
