package api

import (
	"encoding/json"
	"io"
	"net/http"
)

// clusterBackend is the optional interface implemented by Raft backends.
// It is checked via type assertion at request time; single-node deployments
// that don't use Raft simply won't implement it.
type clusterBackend interface {
	IsLeader() bool
	LeaderAddr() string
	AddVoter(id, addr string) error
	RemoveServer(id string) error
	ClusterStatus() any // returns raftbackend.ClusterStatus or similar
}

// cluster returns the cluster backend from Core, if present.
func (s *Server) cluster() (clusterBackend, bool) {
	cb := s.core.ClusterBackend()
	if cb == nil {
		return nil, false
	}
	return cb, true
}

// getClusterStatus handles GET /v1/sys/cluster.
func (s *Server) getClusterStatus(w http.ResponseWriter, r *http.Request) {
	cb, ok := s.cluster()
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "cluster mode not enabled"})
		return
	}
	writeJSON(w, http.StatusOK, cb.ClusterStatus())
}

// postClusterJoin handles POST /v1/sys/cluster/join.
// Must be called on the current leader.
func (s *Server) postClusterJoin(w http.ResponseWriter, r *http.Request) {
	cb, ok := s.cluster()
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "cluster mode not enabled"})
		return
	}
	if !cb.IsLeader() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":       "not leader",
			"leader_addr": cb.LeaderAddr(),
		})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		NodeID   string `json:"node_id"`
		RaftAddr string `json:"raft_addr"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.NodeID == "" || req.RaftAddr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_id and raft_addr are required"})
		return
	}
	if err := cb.AddVoter(req.NodeID, req.RaftAddr); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "joined"})
}

// deleteClusterNode handles DELETE /v1/sys/cluster/node/{id}.
// Must be called on the current leader.
func (s *Server) deleteClusterNode(w http.ResponseWriter, r *http.Request) {
	cb, ok := s.cluster()
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "cluster mode not enabled"})
		return
	}
	if !cb.IsLeader() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":       "not leader",
			"leader_addr": cb.LeaderAddr(),
		})
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node id required"})
		return
	}
	if err := cb.RemoveServer(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
