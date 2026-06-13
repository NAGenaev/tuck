package api

import (
	"encoding/json"
	"net/http"

	"github.com/NAGenaev/tuck/internal/policy"
	"github.com/NAGenaev/tuck/internal/sysconfig"
)

// getSysConfig handles GET /v1/sys/config (root policy required).
func (s *Server) getSysConfig(w http.ResponseWriter, r *http.Request) {
	tokenID := tokenFromCtx(r.Context())
	if err := s.core.EnforceAccess(r.Context(), tokenID, "sys/config", policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	cfg, err := s.core.GetSysConfig(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// putSysConfig handles PUT /v1/sys/config (root policy required).
func (s *Server) putSysConfig(w http.ResponseWriter, r *http.Request) {
	tokenID := tokenFromCtx(r.Context())
	if err := s.core.EnforceAccess(r.Context(), tokenID, "sys/config", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var cfg sysconfig.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if err := s.core.PutSysConfig(r.Context(), cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.ApplyRateConfig(cfg)
	writeJSON(w, http.StatusOK, cfg)
}
