package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/NAGenaev/tuck/internal/audit"
	"github.com/NAGenaev/tuck/internal/namespace"
	"github.com/NAGenaev/tuck/internal/policy"
)

// The audit log endpoints expose the records of EVERY namespace, so on top of
// the usual ACL check (policy on sys/audit/...) they are limited to tokens
// bound to the root namespace. Records only carry paths, token fingerprints
// and client addresses — never secret values or token IDs.

const (
	auditEventsDefaultLimit = 200
	auditEventsMaxLimit     = 1000
	auditStatsDefaultMin    = 60

	// A stream is closed after auditStreamMaxAge; clients reconnect with
	// since_seq and miss nothing that is still in the window. This also bounds
	// graceful shutdown.
	auditStreamMaxAge    = 30 * time.Minute
	auditStreamHeartbeat = 15 * time.Second
	auditStreamWriteTO   = 10 * time.Second
	auditStreamBatch     = 500
	auditStreamMax       = 32 // concurrent live-tail connections per process
)

var auditStreams atomic.Int32

// auditRootAccess authorises a request against a sys/audit/... path and
// requires a root-namespace token. It writes the error response itself.
func (s *Server) auditRootAccess(w http.ResponseWriter, r *http.Request, logical string, cap policy.Capability) bool {
	id := tokenFromCtx(r.Context())
	if err := s.core.EnforceAccess(r.Context(), id, logical, cap); err != nil {
		writeErr(w, err)
		return false
	}
	tok, err := s.core.Authenticate(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return false
	}
	if tok.Namespace != "" && tok.Namespace != namespace.RootNamespace {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return false
	}
	return true
}

var auditOperations = map[string]bool{"": true, "all": true, "read": true, "write": true, "delete": true, "list": true, "login": true, "other": true}

// parseAuditTime accepts an RFC 3339 timestamp, unix seconds, or a duration
// ("15m", "24h") meaning "that long ago".
func parseAuditTime(v string, now time.Time) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t, nil
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
		return time.Unix(n, 0), nil
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q (use RFC 3339, unix seconds or a duration such as 15m)", v)
}

// auditFilterFromQuery builds the shared filter of the events and stream
// endpoints; it writes a 400 and returns false on bad input.
func auditFilterFromQuery(w http.ResponseWriter, r *http.Request) (audit.Filter, bool) {
	q := r.URL.Query()
	now := time.Now()
	bad := func(msg string) (audit.Filter, bool) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return audit.Filter{}, false
	}
	f := audit.Filter{
		Path:      q.Get("path"),
		Operation: q.Get("operation"),
		Status:    q.Get("status"),
		Namespace: q.Get("namespace"),
		Query:     q.Get("q"),
	}
	if len(f.Path) > 512 || len(f.Query) > 512 || len(f.Namespace) > 128 {
		return bad("filter value too long")
	}
	if !auditOperations[f.Operation] {
		return bad("invalid operation filter")
	}
	if !audit.ValidStatusSpec(f.Status) {
		return bad("invalid status filter (use 403, 4xx or error)")
	}
	var err error
	if f.Since, err = parseAuditTime(q.Get("since"), now); err != nil {
		return bad(err.Error())
	}
	if f.Until, err = parseAuditTime(q.Get("until"), now); err != nil {
		return bad(err.Error())
	}
	return f, true
}

// GET /v1/sys/audit/events — recent audit records (newest first) from the
// in-memory window, filtered by path, operation, status, namespace, free text
// and time range.
func (s *Server) getAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !s.auditRootAccess(w, r, "sys/audit/events", policy.CapRead) {
		return
	}
	f, ok := auditFilterFromQuery(w, r)
	if !ok {
		return
	}
	limit := auditEventsDefaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		limit = min(n, auditEventsMaxLimit)
	}
	if v := r.URL.Query().Get("after_seq"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid after_seq"})
			return
		}
		f.AfterSeq = n
	}
	d := s.core.Dispatcher()
	events, matched := d.Ring().Query(f, limit)
	if events == nil {
		events = []audit.Entry{}
	}
	head, seq := d.Head()
	writeJSON(w, http.StatusOK, map[string]any{
		"events":    events,
		"matched":   matched,
		"truncated": matched > len(events),
		"window":    d.Ring().Window(),
		"head_hash": head,
		"head_seq":  seq,
	})
}

// GET /v1/sys/audit/stats — aggregate counters from the audit stream: records
// today, the trailing-minute rate, a per-minute histogram and per-operation
// counts.
func (s *Server) getAuditStats(w http.ResponseWriter, r *http.Request) {
	if !s.auditRootAccess(w, r, "sys/audit/stats", policy.CapRead) {
		return
	}
	minutes := auditStatsDefaultMin
	if v := r.URL.Query().Get("minutes"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid minutes"})
			return
		}
		minutes = min(n, 1440)
	}
	d := s.core.Dispatcher()
	snap := d.Stats().Snapshot(minutes)
	head, seq := d.Head()
	writeJSON(w, http.StatusOK, map[string]any{
		"stats":     snap,
		"window":    d.Ring().Window(),
		"head_hash": head,
		"head_seq":  seq,
	})
}

// POST /v1/sys/audit/verify — verify the hash chain of the in-memory window
// and, when available, of a log file. The file is chosen by name ("primary" =
// the main audit log, or the name of a registered file sink), never by path.
//
// Body (optional): {"source":"memory"|"file"|"all","sink":"<name>"}.
func (s *Server) postAuditVerify(w http.ResponseWriter, r *http.Request) {
	if !s.auditRootAccess(w, r, "sys/audit/verify", policy.CapRead) {
		return
	}
	var body struct {
		Source string `json:"source"`
		Sink   string `json:"sink"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
	}
	switch body.Source {
	case "", "all", "memory", "file":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source must be memory, file or all"})
		return
	}
	d := s.core.Dispatcher()
	out := map[string]any{"verified_at": time.Now().UTC().Format(time.RFC3339)}
	ok := true

	if body.Source != "file" {
		mem := d.VerifyMemory()
		out["memory"] = mem
		ok = ok && mem.OK
	}
	if body.Source != "memory" {
		name := body.Sink
		if name == "" {
			name = "primary"
		}
		out["file_source"] = name
		res, err := d.VerifyFileSink(name)
		switch {
		case errors.Is(err, audit.ErrSinkNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sink not found"})
			return
		case err != nil:
			// No file to check (or it cannot be read): report it without
			// failing the memory result; an explicit file-only request is an error.
			if body.Source == "file" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			out["file_error"] = err.Error()
		default:
			out["file"] = res
			ok = ok && res.OK
		}
	}
	out["ok"] = ok
	head, seq := d.Head()
	out["head_hash"], out["head_seq"] = head, seq
	writeJSON(w, http.StatusOK, out)
}

// GET /v1/sys/audit/events/stream — live tail as Server-Sent Events.
//
// Events: "hello" ({head_seq}), "audit" (one record, id = seq), "gap"
// ({missed_after, oldest_seq}: records were overwritten before they could be
// sent) and ": ping" heartbeats. Without since_seq only new records are sent;
// with since_seq the buffered records after it are replayed first. The same
// filters as /events apply. The stream ends after auditStreamMaxAge or when the
// token stops being valid; EventSource-style clients simply reconnect.
func (s *Server) getAuditEventsStream(w http.ResponseWriter, r *http.Request) {
	if !s.auditRootAccess(w, r, "sys/audit/events", policy.CapRead) {
		return
	}
	f, ok := auditFilterFromQuery(w, r)
	if !ok {
		return
	}
	d := s.core.Dispatcher()
	cursor := d.Ring().Head()
	for _, v := range []string{r.URL.Query().Get("since_seq"), r.Header.Get("Last-Event-ID")} {
		if v == "" {
			continue
		}
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid since_seq"})
			return
		}
		cursor = n
		break
	}
	if auditStreams.Add(1) > auditStreamMax {
		auditStreams.Add(-1)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many live audit streams"})
		return
	}
	defer auditStreams.Add(-1)

	// The server-wide Read/WriteTimeout would cut a long-lived stream.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(format string, args ...any) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(auditStreamWriteTO))
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !write("retry: 3000\nevent: hello\ndata: {\"head_seq\":%d}\n\n", cursor) {
		return
	}

	ring := d.Ring()
	ctx := r.Context()
	id := tokenFromCtx(ctx)
	heartbeat := time.NewTicker(auditStreamHeartbeat)
	defer heartbeat.Stop()
	deadline := time.NewTimer(auditStreamMaxAge)
	defer deadline.Stop()

	for {
		wait := ring.Wait() // taken before reading so no append is missed
		ents, gap := ring.After(cursor, auditStreamBatch)
		if gap {
			oldest := ring.Window().OldestSeq
			if !write("event: gap\ndata: {\"missed_after\":%d,\"oldest_seq\":%d}\n\n", cursor, oldest) {
				return
			}
		}
		for _, e := range ents {
			cursor = e.Seq
			if !f.Match(e) {
				continue
			}
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			if !write("id: %d\nevent: audit\ndata: %s\n\n", e.Seq, data) {
				return
			}
		}
		if len(ents) == auditStreamBatch {
			continue // more backlog
		}
		select {
		case <-wait:
		case <-heartbeat.C:
			// Re-check the token so a revoked token cannot keep tailing.
			if _, err := s.core.Authenticate(ctx, id); err != nil {
				return
			}
			if !write(": ping\n\n") {
				return
			}
		case <-deadline.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

// redactWebhookURL hides credentials and query strings in a webhook URL; both
// commonly carry secrets (basic-auth userinfo, ?token=...).
func redactWebhookURL(raw string) string {
	scheme, rest, found := strings.Cut(raw, "://")
	if !found {
		return "…"
	}
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i] + "?…"
	}
	if i := strings.Index(rest, "@"); i >= 0 && i < strings.IndexAny(rest+"/", "/") {
		rest = "…@" + rest[i+1:]
	}
	return scheme + "://" + rest
}
