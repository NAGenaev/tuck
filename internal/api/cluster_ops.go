package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	physraft "github.com/NAGenaev/tuck/internal/physical/raft"
	"github.com/NAGenaev/tuck/internal/policy"
)

// clusterOps is implemented by the Raft backend: leadership transfer and the
// node-local snapshot store. It is checked by type assertion so that other
// cluster backends keep working without these operations.
type clusterOps interface {
	StepDown(targetID string) error
	ListSnapshots() ([]physraft.SnapshotInfo, error)
	CreateSnapshot() (physraft.SnapshotInfo, error)
	OpenSnapshot(id string) (physraft.SnapshotInfo, io.ReadCloser, error)
	Restore(r io.Reader, maxBytes int64) (int, error)
}

// clusterOpsFor authorises the request and returns the Raft operations, writing
// the error response itself when it cannot.
func (s *Server) clusterOpsFor(w http.ResponseWriter, r *http.Request, path string, cap policy.Capability) (clusterBackend, clusterOps, bool) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), path, cap); err != nil {
		writeErr(w, err)
		return nil, nil, false
	}
	cb, ok := s.cluster()
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "cluster mode not enabled"})
		return nil, nil, false
	}
	ops, ok := cb.(clusterOps)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "cluster backend does not support this operation"})
		return nil, nil, false
	}
	return cb, ops, true
}

// postClusterStepDown handles POST /v1/sys/cluster/step-down.
//
// Hands leadership to another voter (the one named by target_id, otherwise the
// most up-to-date follower) and returns when the transfer has completed. Must
// be called on the current leader. Audited like every request by the audit
// middleware (method, path, status, token fingerprint, client IP).
func (s *Server) postClusterStepDown(w http.ResponseWriter, r *http.Request) {
	cb, ops, ok := s.clusterOpsFor(w, r, "sys/cluster/step-down", policy.CapWrite)
	if !ok {
		return
	}
	var req struct {
		TargetID string `json:"target_id"`
	}
	if r.ContentLength != 0 {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
			return
		}
		if len(body) > 0 && !decodeJSON(w, body, &req) {
			return
		}
	}
	if !cb.IsLeader() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":       "not leader",
			"leader_addr": cb.LeaderAddr(),
		})
		return
	}
	if err := ops.StepDown(req.TargetID); err != nil {
		switch {
		case errors.Is(err, physraft.ErrNotLeader):
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "not leader", "leader_addr": cb.LeaderAddr()})
		case errors.Is(err, physraft.ErrInvalidTransferTarget):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, physraft.ErrNoOtherVoter):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusConflict, map[string]string{"error": "leadership transfer failed: " + err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "transferred", "target_id": req.TargetID})
}

// getClusterSnapshots handles GET /v1/sys/cluster/snapshot: the snapshots kept
// in this node's snapshot directory (node-local, newest first).
func (s *Server) getClusterSnapshots(w http.ResponseWriter, r *http.Request) {
	_, ops, ok := s.clusterOpsFor(w, r, "sys/cluster/snapshot", policy.CapRead)
	if !ok {
		return
	}
	list, err := ops.ListSnapshots()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": list})
}

// postClusterSnapshot handles POST /v1/sys/cluster/snapshot: takes a snapshot
// now and stores it in this node's snapshot directory. It works on any node.
func (s *Server) postClusterSnapshot(w http.ResponseWriter, r *http.Request) {
	_, ops, ok := s.clusterOpsFor(w, r, "sys/cluster/snapshot", policy.CapWrite)
	if !ok {
		return
	}
	info, err := ops.CreateSnapshot()
	switch {
	case errors.Is(err, physraft.ErrNoNewData):
		var latest *physraft.SnapshotInfo
		if list, lerr := ops.ListSnapshots(); lerr == nil && len(list) > 0 {
			latest = &list[0]
		}
		writeJSON(w, http.StatusOK, map[string]any{"created": false, "reason": "no new data since the last snapshot", "snapshot": latest})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "snapshot failed: " + err.Error()})
	default:
		writeJSON(w, http.StatusCreated, map[string]any{"created": true, "snapshot": info})
	}
}

// maxRestoreBytes bounds an uploaded snapshot.
const maxRestoreBytes = 256 << 20

// postClusterRestore handles POST /v1/sys/cluster/restore?confirm=true.
//
// The body is a snapshot downloaded earlier from GET /v1/sys/snapshot or
// /v1/sys/cluster/snapshot/{id}. The whole cluster's data is replaced by it in
// one replicated step, and every node then seals itself: the snapshot carries
// the key ring of the cluster it was taken from, so the nodes must be
// unsealed again with that cluster's key shares. Tokens and secrets written
// after the snapshot are lost. Root namespace only, leader only.
func (s *Server) postClusterRestore(w http.ResponseWriter, r *http.Request) {
	if nsFromCtx(r.Context()) != "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}
	cb, ops, ok := s.clusterOpsFor(w, r, "sys/cluster/restore", policy.CapWrite)
	if !ok {
		return
	}
	if r.URL.Query().Get("confirm") != "true" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "restore replaces ALL data of the cluster; repeat with ?confirm=true"})
		return
	}
	if !cb.IsLeader() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "not leader", "leader_addr": cb.LeaderAddr()})
		return
	}
	n, err := ops.Restore(http.MaxBytesReader(w, r.Body, maxRestoreBytes), maxRestoreBytes)
	switch {
	case errors.Is(err, physraft.ErrNotLeader):
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "not leader", "leader_addr": cb.LeaderAddr()})
	case errors.Is(err, physraft.ErrEmptySnapshot):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case err != nil:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusOK, map[string]any{
			"restored":  n,
			"sealed":    true,
			"next_step": "unseal every node with the key shares of the cluster the snapshot came from",
		})
	}
}

// getClusterSnapshot handles GET /v1/sys/cluster/snapshot/{id}: downloads one
// stored snapshot. It carries the same encrypted data as GET /v1/sys/snapshot
// and requires the same permission.
func (s *Server) getClusterSnapshot(w http.ResponseWriter, r *http.Request) {
	_, ops, ok := s.clusterOpsFor(w, r, "sys/snapshot", policy.CapRead)
	if !ok {
		return
	}
	info, rc, err := ops.OpenSnapshot(r.PathValue("id"))
	if errors.Is(err, physraft.ErrSnapshotNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="tuck-snapshot-`+strconv.FormatUint(info.Index, 10)+`.db"`)
	if info.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}
