package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/NAGenaev/tuck/internal/audit"
	"github.com/NAGenaev/tuck/internal/policy"
)

// POST /v1/sys/audit/webhook — register or update a webhook audit sink
func (s *Server) putAuditWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/audit/"+name, policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		URL        string `json:"url"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}
	if body.TimeoutSec <= 0 {
		body.TimeoutSec = 5
	}
	if err := s.core.RegisterAuditWebhook(r.Context(), name, body.URL, body.TimeoutSec); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /v1/sys/audit/{name} — remove a named audit sink
func (s *Server) deleteAuditSink(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/audit/"+name, policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.DeregisterAuditSink(r.Context(), name); err != nil {
		if errors.Is(err, audit.ErrSinkNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sink not found"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LIST /v1/sys/audit/ — list all registered audit sinks with error counts
func (s *Server) listAuditSinks(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/audit", policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	cfgs, errCounts, err := s.core.ListAuditSinks(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	type sinkInfo struct {
		audit.SinkConfig
		Errors int `json:"errors"`
	}
	sinks := make([]sinkInfo, 0, len(cfgs))
	for _, cfg := range cfgs {
		sinks = append(sinks, sinkInfo{SinkConfig: cfg, Errors: errCounts[cfg.Name]})
	}
	if sinks == nil {
		sinks = []sinkInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sinks": sinks})
}
