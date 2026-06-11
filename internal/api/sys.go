package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/policy"
)

// sealStatusResponse is the JSON shape for GET /v1/sys/seal-status.
type sealStatusResponse struct {
	Sealed          bool   `json:"sealed"`
	Type            string `json:"type"`
	RequiredShards  int    `json:"required_shards,omitempty"`
	ReceivedShards  int    `json:"received_shards,omitempty"`
}

// unsealRequest is the JSON body for POST /v1/sys/unseal.
type unsealRequest struct {
	Key string `json:"key"` // base64url-encoded shard
}

// unsealResponse is the JSON body returned by POST /v1/sys/unseal.
type unsealResponse struct {
	Sealed          bool   `json:"sealed"`
	RequiredShards  int    `json:"required_shards,omitempty"`
	ReceivedShards  int    `json:"received_shards,omitempty"`
	Message         string `json:"message,omitempty"`
}

// getSealStatus handles GET /v1/sys/seal-status (no authentication required).
func (s *Server) getSealStatus(w http.ResponseWriter, r *http.Request) {
	info := s.core.SealStatus()
	resp := sealStatusResponse{
		Sealed: info.Sealed,
		Type:   info.Type,
	}
	if info.Required > 0 {
		resp.RequiredShards = info.Required
		resp.ReceivedShards = info.Received
	}
	writeJSON(w, http.StatusOK, resp)
}

// postUnseal handles POST /v1/sys/unseal (no authentication required).
//
// For Shamir seals: accepts one base64url-encoded shard at a time. Returns the
// current progress until the threshold is met, at which point sealed=false.
//
// For auto-unseal seals (dev, transit): returns 400 — those seals unseal
// automatically at startup.
func (s *Server) postUnseal(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var req unsealRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing field: key"})
		return
	}

	complete, err := s.core.UnsealShard(r.Context(), req.Key)
	if err != nil {
		if errors.Is(err, core.ErrSealNotInteractive) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	info := s.core.SealStatus()
	resp := unsealResponse{
		Sealed:         info.Sealed,
		RequiredShards: info.Required,
		ReceivedShards: info.Received,
	}
	if complete {
		resp.Message = "unseal complete"
	}
	writeJSON(w, http.StatusOK, resp)
}

// getReady handles GET /v1/sys/ready (no authentication required).
//
// Returns 200 when the server is unsealed and ready to serve requests.
// Returns 503 when sealed — use this as the Kubernetes readinessProbe target.
// GET /v1/health can serve as the livenessProbe (always 200 while process runs).
func (s *Server) getReady(w http.ResponseWriter, _ *http.Request) {
	if s.core.Sealed() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ready": false, "sealed": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ready": true, "sealed": false})
}

// postSeal handles POST /v1/sys/seal (requires X-Tuck-Token with root policy).
//
// Re-seals the server immediately, dropping the barrier key from memory. All
// in-flight secret operations will begin returning 503 until the server is
// manually unsealed again.
func (s *Server) postSeal(w http.ResponseWriter, r *http.Request) {
	tokenID := tokenFromCtx(r.Context())
	if err := s.core.EnforceAccess(r.Context(), tokenID, "sys/seal", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	s.core.Seal()
	writeJSON(w, http.StatusOK, map[string]bool{"sealed": true})
}
