package api

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

// TestClusterJoinTokenAPI covers the full lifecycle of POST /v1/sys/cluster/join-token:
// minted token's shape, defaults, caps, not-leader handling, and that it is forbidden to
// a non-root/non-sudo caller — including, crucially, to a token minted by this very
// endpoint (no self-escalation).
func TestClusterJoinTokenAPI(t *testing.T) {
	ts, _, root := newRaftTestServer(t)

	// Defaults: no body at all.
	code, m, raw := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/join-token", "", root)
	if code != http.StatusCreated {
		t.Fatalf("default mint: %d %s", code, raw)
	}
	joinTok, _ := m["token"].(string)
	if joinTok == "" || m["accessor"] == nil || m["accessor"] == "" {
		t.Fatalf("missing token/accessor: %s", raw)
	}
	if m["policy"] != "cluster-join" {
		t.Fatalf("policy = %v, want cluster-join", m["policy"])
	}
	if m["ttl_seconds"].(float64) != 600 || m["num_uses"].(float64) != 1 {
		t.Fatalf("defaults: ttl=%v uses=%v, want 600/1", m["ttl_seconds"], m["num_uses"])
	}
	if m["leader_addr"] == nil || m["node_id"] != "node-a" {
		t.Fatalf("self info missing: %s", raw)
	}
	if _, err := time.Parse(time.RFC3339, m["expires_at"].(string)); err != nil {
		t.Fatalf("expires_at not RFC3339: %v (%s)", err, raw)
	}

	// Caps: an oversized request is clamped, not rejected. num_uses=10 (capped) leaves
	// enough budget below to probe several forbidden calls without exhausting the token,
	// so a 403 can't be confused with a 401 "exceeded use limit".
	code, m, raw = clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/join-token",
		`{"ttl_seconds":999999,"num_uses":999}`, root)
	if code != http.StatusCreated || m["ttl_seconds"].(float64) != 3600 || m["num_uses"].(float64) != 10 {
		t.Fatalf("cap enforcement: %d %s", code, raw)
	}
	spareUsesTok := m["token"].(string)

	// Negative values are a hard 400.
	if code, _, _ := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/join-token", `{"ttl_seconds":-1}`, root); code != http.StatusBadRequest {
		t.Fatalf("negative ttl_seconds: %d, want 400", code)
	}

	// The minted token can do what the cluster-join policy promises: read status,
	// and (since this node is the leader) call join itself.
	if code, _, raw := clusterJSON(t, http.MethodGet, ts.URL+"/v1/sys/cluster", "", joinTok); code != http.StatusOK {
		t.Fatalf("join token read status: %d %s", code, raw)
	}
	// ...but not mint another join token (no self-escalation).
	if code, _, raw := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/join-token", "", spareUsesTok); code != http.StatusForbidden {
		t.Fatalf("join token minting again: %d %s, want 403", code, raw)
	}
	// ...nor remove a node (cluster-join grants read+write, not delete).
	if code, _, raw := clusterJSON(t, http.MethodDelete, ts.URL+"/v1/sys/cluster/node/node-a", "", spareUsesTok); code != http.StatusForbidden {
		t.Fatalf("join token removing a node: %d %s, want 403", code, raw)
	}

	// A token with read-only sys/cluster access (the same narrow shape an operator
	// might hand-craft today) is also forbidden — this is a root/sudo-only endpoint.
	ro := limitedToken(t, ts, root, `{"path":"sys/cluster","capabilities":["read","write"]}`)
	if code, _, raw := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/join-token", "", ro); code != http.StatusForbidden {
		t.Fatalf("sys/cluster-only token minting: %d %s, want 403", code, raw)
	}

	// No token at all.
	if code, _, _ := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/join-token", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d, want 401", code)
	}
}

// TestClusterJoinTokenNumUsesEnforced confirms num_uses is enforced by the generic
// token-use-tracking path (requireToken -> Core.TrackUse), not by anything bespoke here.
func TestClusterJoinTokenNumUsesEnforced(t *testing.T) {
	ts, _, root := newRaftTestServer(t)
	code, m, raw := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/join-token", `{"num_uses":1}`, root)
	if code != http.StatusCreated {
		t.Fatalf("mint: %d %s", code, raw)
	}
	joinTok := m["token"].(string)

	// First authenticated use succeeds and exhausts the single allowed use.
	if code, _, raw := clusterJSON(t, http.MethodGet, ts.URL+"/v1/sys/cluster", "", joinTok); code != http.StatusOK {
		t.Fatalf("first use: %d %s", code, raw)
	}
	if code, _, raw := clusterJSON(t, http.MethodGet, ts.URL+"/v1/sys/cluster", "", joinTok); code != http.StatusUnauthorized {
		t.Fatalf("second use after exhaustion: %d %s, want 401", code, raw)
	}
}

// TestClusterJoinTokenNotLeader mirrors TestClusterStepDownErrorMapping's style: a
// follower refuses to mint and reports the leader's address instead.
func TestClusterJoinTokenNotLeader(t *testing.T) {
	fb := &fakeRaft{Backend: physical.NewInMem()}
	ts, _, tok := serveCore(t, core.New(fb, seal.NewDev(filepath.Join(t.TempDir(), "rootkey"))))
	code, m, raw := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/join-token", "", tok)
	if code != http.StatusServiceUnavailable || m["leader_addr"] != "10.0.0.9:8201" {
		t.Fatalf("follower mint: %d %s", code, raw)
	}
}

// TestClusterJoinTokenNotInClusterMode mirrors TestClusterOpsNotInClusterMode.
func TestClusterJoinTokenNotInClusterMode(t *testing.T) {
	ts, _, tok := newTestServer(t)
	if code, _, raw := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/join-token", "", tok); code != http.StatusNotImplemented {
		t.Fatalf("no cluster backend: %d %s, want 501", code, raw)
	}
}
