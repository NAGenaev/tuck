package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	dynaws "github.com/NAGenaev/tuck/internal/dynamic/aws"
)

// PUT /v1/aws/config
func (s *Server) putAWSConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var cfg dynaws.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if cfg.Region == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "region is required"})
		return
	}
	if err := s.core.PutAWSConfig(r.Context(), &cfg); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/aws/config
func (s *Server) getAWSConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.core.GetAWSConfig(r.Context())
	if err != nil {
		if errors.Is(err, dynaws.ErrNotConfigured) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "aws engine not configured"})
			return
		}
		writeErr(w, err)
		return
	}
	cfg.SecretAccessKey = "" // never return credentials
	writeJSON(w, http.StatusOK, cfg)
}

// DELETE /v1/aws/config
func (s *Server) deleteAWSConfig(w http.ResponseWriter, r *http.Request) {
	if err := s.core.DeleteAWSConfig(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /v1/aws/roles/{name}
// Body: {"credential_type":"assumed_role","role_arns":["arn:..."],"default_ttl":"1h"}
func (s *Server) putAWSRole(w http.ResponseWriter, r *http.Request) {
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
		CredentialType string   `json:"credential_type"`
		PolicyARNs     []string `json:"policy_arns"`
		PolicyDocument string   `json:"policy_document"`
		RoleARNs       []string `json:"role_arns"`
		DefaultTTL     string   `json:"default_ttl"`
		MaxTTL         string   `json:"max_ttl"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if wire.CredentialType != dynaws.CredTypeIAMUser && wire.CredentialType != dynaws.CredTypeAssumedRole {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential_type must be iam_user or assumed_role"})
		return
	}
	role := &dynaws.Role{
		Name:           name,
		CredentialType: wire.CredentialType,
		PolicyARNs:     wire.PolicyARNs,
		PolicyDocument: wire.PolicyDocument,
		RoleARNs:       wire.RoleARNs,
	}
	if wire.DefaultTTL != "" {
		d, err := time.ParseDuration(wire.DefaultTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid default_ttl"})
			return
		}
		role.DefaultTTL = d
	}
	if wire.MaxTTL != "" {
		d, err := time.ParseDuration(wire.MaxTTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid max_ttl"})
			return
		}
		role.MaxTTL = d
	}
	if err := s.core.PutAWSRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/aws/roles/{name}
func (s *Server) getAWSRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	role, err := s.core.GetAWSRole(r.Context(), name)
	if err != nil {
		if errors.Is(err, dynaws.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

// DELETE /v1/aws/roles/{name}
func (s *Server) deleteAWSRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.DeleteAWSRole(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/aws/roles/
func (s *Server) listAWSRoles(w http.ResponseWriter, r *http.Request) {
	names, err := s.core.ListAWSRoles(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": names})
}

// POST /v1/aws/creds/{role}
func (s *Server) generateAWSCreds(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("role")
	res, err := s.core.GenerateAWSCreds(r.Context(), roleName)
	if err != nil {
		switch {
		case errors.Is(err, dynaws.ErrNotConfigured):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "aws engine not configured"})
		case errors.Is(err, dynaws.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
		default:
			writeErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// GET /v1/aws/lease/{id}
func (s *Server) getAWSLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lease, err := s.core.GetAWSLease(r.Context(), id)
	if err != nil {
		if errors.Is(err, dynaws.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lease)
}

// DELETE /v1/aws/lease/{id}
func (s *Server) revokeAWSLease(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.core.RevokeAWSLease(r.Context(), id); err != nil {
		if errors.Is(err, dynaws.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "lease not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/aws/lease/
func (s *Server) listAWSLeases(w http.ResponseWriter, r *http.Request) {
	ids, err := s.core.ListAWSLeases(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": ids})
}
