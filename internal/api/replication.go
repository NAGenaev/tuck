package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/NAGenaev/tuck/internal/policy"
	"github.com/NAGenaev/tuck/internal/replication"
)

// GET /v1/sys/replication/status
func (s *Server) replicationStatus(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/replication", policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	st, err := s.core.WAL().GetState(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// POST /v1/sys/replication/primary/enable
// Body: {} (no args required — sets this node as primary)
func (s *Server) enablePrimary(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/replication", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.WAL().SetMode(r.Context(), replication.ReplicaModePrimary, ""); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /v1/sys/replication/secondary/enable
// Body: {"primary_addr":"https://primary:8200"}
func (s *Server) enableSecondary(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/replication", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		PrimaryAddr string `json:"primary_addr"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.PrimaryAddr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "primary_addr required"})
		return
	}
	if err := s.core.WAL().SetMode(r.Context(), replication.ReplicaModeSecondary, req.PrimaryAddr); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /v1/sys/replication/disable
func (s *Server) disableReplication(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/replication", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.WAL().SetMode(r.Context(), replication.ReplicaModeDisabled, ""); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /v1/sys/replication/wal?after=N
// Returns WAL entries with Sequence > N (default 0).
func (s *Server) walEntries(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/replication", policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	afterSeq := uint64(0)
	if s := r.URL.Query().Get("after"); s != "" {
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid after parameter"})
			return
		}
		afterSeq = n
	}
	entries, err := s.core.WAL().ReadFrom(r.Context(), afterSeq)
	if err != nil {
		writeErr(w, err)
		return
	}
	if entries == nil {
		entries = []*replication.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// POST /v1/sys/replication/wal/trim
// Body: {"min_sequence":N} — removes WAL entries with Sequence < N.
func (s *Server) trimWAL(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/replication", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		MinSequence uint64 `json:"min_sequence"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := s.core.WAL().TrimBefore(r.Context(), req.MinSequence); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
