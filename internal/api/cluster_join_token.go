package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/policy"
	"github.com/NAGenaev/tuck/internal/token"
)

const (
	defaultJoinTokenTTL     = 10 * time.Minute
	maxJoinTokenTTL         = time.Hour
	defaultJoinTokenNumUses = 1
	maxJoinTokenNumUses     = 10
)

// clusterJoinTokenReq is the optional body of POST /v1/sys/cluster/join-token.
// Zero/omitted fields fall back to the defaults; out-of-range values are
// silently capped rather than rejected, since the caps exist only to stop an
// accidental standing credential, not to police operator intent.
type clusterJoinTokenReq struct {
	TTLSeconds int `json:"ttl_seconds"`
	NumUses    int `json:"num_uses"`
}

// clusterJoinTokenResp is shown once — like any other freshly minted token,
// the raw value is never retrievable again via the API.
type clusterJoinTokenResp struct {
	Token      string    `json:"token"`
	Accessor   string    `json:"accessor"`
	Policy     string    `json:"policy"`
	ExpiresAt  time.Time `json:"expires_at"`
	TTLSeconds int       `json:"ttl_seconds"`
	NumUses    int       `json:"num_uses"`
	// LeaderAddr is this node's Raft RPC address (it only answers when it is
	// the leader). AdvertiseAddr/NodeID repeat the same self-reported fields
	// GET /v1/sys/cluster exposes, included here so the UI can build the full
	// join command from this single response. Tuck has no separate "advertised
	// HTTP address" concept: operators join via the HTTP address they are
	// already talking to (see cmd/tuck's --cluster-join flag).
	LeaderAddr    string `json:"leader_addr"`
	NodeID        string `json:"node_id,omitempty"`
	AdvertiseAddr string `json:"advertise_addr,omitempty"`
}

// postClusterJoinToken handles POST /v1/sys/cluster/join-token.
//
// Mints a short-lived, few-use token scoped to the built-in cluster-join
// policy (read+write on sys/cluster only — not delete, step-down, or
// snapshot, and not this path itself) so operators can bootstrap a new node
// without being handed the root token or having to hand-craft a policy. Root
// and sudo-equivalent tokens only: a token minted by this endpoint cannot call
// it again, because policy.ClusterJoinPolicy does not grant
// "sys/cluster/join-token". Must be called on the current leader, like every
// other cluster write. Audited like every request by the audit middleware
// (method, path, status, token fingerprint, client IP).
func (s *Server) postClusterJoinToken(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/cluster/join-token", policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
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

	var req clusterJoinTokenReq
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
	if req.TTLSeconds < 0 || req.NumUses < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ttl_seconds and num_uses must be >= 0"})
		return
	}

	ttl := defaultJoinTokenTTL
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	if ttl > maxJoinTokenTTL {
		ttl = maxJoinTokenTTL
	}
	numUses := defaultJoinTokenNumUses
	if req.NumUses > 0 {
		numUses = req.NumUses
	}
	if numUses > maxJoinTokenNumUses {
		numUses = maxJoinTokenNumUses
	}

	// Parented to the caller (same default as POST /v1/auth/token): revoking
	// the admin's own token cascades to every join token it minted. There is
	// no orphan option here — these are meant to be used within minutes, not
	// to outlive their creator.
	tok, err := s.core.CreateToken(r.Context(), "cluster-join-token", []string{token.ClusterJoinPolicyName}, ttl,
		core.WithMaxUses(numUses), core.WithParent(tokenFromCtx(r.Context())))
	if err != nil {
		writeErr(w, err)
		return
	}

	var self struct {
		NodeID        string `json:"node_id"`
		AdvertiseAddr string `json:"advertise_addr"`
	}
	if raw, jerr := json.Marshal(cb.ClusterStatus()); jerr == nil {
		_ = json.Unmarshal(raw, &self)
	}

	writeJSON(w, http.StatusCreated, clusterJoinTokenResp{
		Token:         tok.ID,
		Accessor:      tok.Accessor,
		Policy:        token.ClusterJoinPolicyName,
		ExpiresAt:     tok.ExpiresAt,
		TTLSeconds:    int(ttl.Seconds()),
		NumUses:       numUses,
		LeaderAddr:    cb.LeaderAddr(),
		NodeID:        self.NodeID,
		AdvertiseAddr: self.AdvertiseAddr,
	})
}
