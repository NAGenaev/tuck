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

// PUT /v1/sys/audit/file/{name} — register or update a file audit sink
func (s *Server) putAuditFile(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/audit/"+name, policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Path       string `json:"path"`
		MaxSizeMB  int64  `json:"max_size_mb"`
		MaxBackups int    `json:"max_backups"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}
	if s.core.Dispatcher().PathInUse(body.Path, name) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "path is already used by the primary audit log or another file sink"})
		return
	}
	if err := s.core.RegisterAuditFile(r.Context(), name, body.Path, body.MaxSizeMB, body.MaxBackups); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /v1/sys/audit/syslog/{name} — register or update a syslog audit sink.
// Answers 501 on platforms without syslog (see capabilities.syslog in the list).
func (s *Server) putAuditSyslog(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/audit/"+name, policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	if !audit.SyslogSupported {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "syslog sinks are not supported on this platform"})
		return
	}
	var body struct {
		Network  string `json:"network"` // "udp" | "tcp" | "" (local syslog socket)
		Addr     string `json:"addr"`    // host:port; required for udp/tcp
		Facility string `json:"facility"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	switch body.Network {
	case "", "udp", "tcp":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "network must be udp, tcp or empty (local syslog)"})
		return
	}
	if body.Network != "" && body.Addr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "addr is required for udp/tcp"})
		return
	}
	if len(body.Addr) > 255 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "addr is too long"})
		return
	}
	if !audit.ValidSyslogFacility(body.Facility) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown syslog facility"})
		return
	}
	if err := s.core.RegisterAuditSyslog(r.Context(), name, body.Network, body.Addr, body.Facility); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /v1/sys/audit/state/{name} — pause or resume a sink: {"enabled": bool}.
// The choice is persisted; records produced while paused are not replayed.
func (s *Server) putAuditSinkState(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/audit/"+name, policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&body); err != nil || body.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled (boolean) is required"})
		return
	}
	if err := s.core.SetAuditSinkEnabled(r.Context(), name, *body.Enabled); err != nil {
		if errors.Is(err, audit.ErrSinkNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sink not found"})
			return
		}
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

// auditSinkInfo is one row of the sink list: the stored config (webhook URLs
// redacted) plus the runtime state of the running sink.
type auditSinkInfo struct {
	audit.SinkConfig
	Errors  int  `json:"errors"`
	Enabled bool `json:"enabled"`
	// Active is false when the sink is configured but not running (for
	// example its file could not be opened at startup).
	Active bool `json:"active"`
	// Health: ok | failing | paused | inactive.
	Health string           `json:"health"`
	Stats  *audit.SinkStats `json:"stats,omitempty"`
}

// LIST /v1/sys/audit/ — list all registered audit sinks with runtime stats
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
	stats := s.core.AuditSinkStats()
	sinks := make([]auditSinkInfo, 0, len(cfgs))
	for _, cfg := range cfgs {
		info := auditSinkInfo{SinkConfig: cfg, Errors: errCounts[cfg.Name], Enabled: !cfg.Disabled}
		if cfg.Type == "webhook" {
			opts := make(map[string]string, len(cfg.Options))
			for k, v := range cfg.Options {
				opts[k] = v
			}
			if u := opts["url"]; u != "" {
				opts["url"] = redactWebhookURL(u)
			}
			info.Options = opts
		}
		st, running := stats[cfg.Name]
		info.Active = running
		switch {
		case !running:
			info.Health = "inactive"
		case st.Paused:
			info.Health = "paused"
		case st.Failing > 0:
			info.Health = "failing"
		default:
			info.Health = "ok"
		}
		if running {
			st := st
			info.Stats = &st
		}
		sinks = append(sinks, info)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sinks": sinks,
		"capabilities": map[string]bool{
			"syslog": audit.SyslogSupported,
			"pause":  true,
		},
	})
}
