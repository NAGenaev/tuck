package raft

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

// openCluster starts a bootstrapped three-node cluster and waits for a leader.
func openCluster(t *testing.T) (map[string]*Backend, string) {
	t.Helper()
	ids := []string{"n1", "n2", "n3"}
	addrs := map[string]string{}
	for _, id := range ids {
		addrs[id] = freeAddr(t)
	}
	dirs := map[string]string{}
	for _, id := range ids {
		dirs[id] = t.TempDir() // registered before the Close cleanup, so removed after it
	}
	old := sampleInterval
	sampleInterval = 100 * time.Millisecond
	t.Cleanup(func() { sampleInterval = old })
	nodes := map[string]*Backend{}
	t.Cleanup(func() {
		for _, b := range nodes {
			_ = b.Close()
		}
	})
	for _, id := range ids {
		cfg := nodeConfig(id, addrs[id], dirs[id])
		cfg.Bootstrap = true
		for _, other := range ids {
			if other != id {
				cfg.Peers = append(cfg.Peers, Peer{ID: other, Addr: addrs[other]})
			}
		}
		b, err := Open(cfg)
		if err != nil {
			t.Fatalf("open %s: %v", id, err)
		}
		nodes[id] = b
	}
	var leaderID string
	eventually(t, "initial leader", func() bool { id, l := findLeader(nodes); leaderID = id; return l != nil })
	return nodes, leaderID
}

func TestStepDownTransfersLeadership(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-node: skipped in -short mode")
	}
	nodes, first := openCluster(t)

	// A follower refuses to step down.
	for id, b := range nodes {
		if id != first {
			if err := b.StepDown(""); !errors.Is(err, ErrNotLeader) {
				t.Fatalf("follower StepDown: want ErrNotLeader, got %v", err)
			}
		}
	}
	// Invalid targets are rejected without disturbing the leader.
	if err := nodes[first].StepDown(first); !errors.Is(err, ErrInvalidTransferTarget) {
		t.Fatalf("self target: got %v", err)
	}
	if err := nodes[first].StepDown("nope"); !errors.Is(err, ErrInvalidTransferTarget) {
		t.Fatalf("unknown target: got %v", err)
	}
	if !nodes[first].IsLeader() {
		t.Fatal("leader changed after rejected transfers")
	}

	// Automatic target.
	if err := nodes[first].StepDown(""); err != nil {
		t.Fatalf("StepDown: %v", err)
	}
	var second string
	eventually(t, "new leader", func() bool {
		id, l := findLeader(nodes)
		second = id
		return l != nil && id != first
	})

	// Explicit target: pick a voter that is not the current leader.
	var target string
	for id := range nodes {
		if id != second {
			target = id
			break
		}
	}
	if err := nodes[second].StepDown(target); err != nil {
		t.Fatalf("StepDown(%s): %v", target, err)
	}
	eventually(t, "leader is the requested target", func() bool { return nodes[target].IsLeader() })

	// The cluster still takes writes after two transfers.
	eventually(t, "write after transfers", func() bool {
		return nodes[target].Put(context.Background(), entry("after", "ok")) == nil
	})
}

func TestStatusReplicationMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-node: skipped in -short mode")
	}
	nodes, leaderID := openCluster(t)
	leader := nodes[leaderID]
	for i := 0; i < 5; i++ {
		if err := leader.Put(context.Background(), entry("k", "v")); err != nil {
			t.Fatal(err)
		}
	}

	// The leader sees every follower through heartbeats.
	eventually(t, "leader observes both followers", func() bool {
		st := leader.Status()
		seen := 0
		for _, s := range st.Servers {
			if !s.Self && s.Metrics != nil && s.Metrics.Source == "heartbeat" && s.Metrics.HeartbeatRttMs != nil &&
				s.Metrics.Reachable != nil && *s.Metrics.Reachable && s.Metrics.LagEntries != nil {
				seen++
			}
		}
		return seen == 2
	})
	st := leader.Status()
	if st.NodeID != leaderID || st.Term == 0 || st.CommitIndex == 0 || st.LastLogIndex < st.CommitIndex || st.LastContactMs != nil {
		t.Fatalf("unexpected leader stats: %+v", st)
	}
	var self *ServerInfo
	for i := range st.Servers {
		if st.Servers[i].Self {
			self = &st.Servers[i]
		}
	}
	if self == nil || self.ID != leaderID || self.Metrics == nil || self.Metrics.Source != "local" || *self.Metrics.LagEntries != 0 {
		t.Fatalf("unexpected self entry: %+v", self)
	}

	// A follower reports its own numbers and the age of the last leader contact,
	// but invents nothing about the other peers.
	for id, b := range nodes {
		if id == leaderID {
			continue
		}
		eventually(t, id+" has leader contact", func() bool { return b.Status().LastContactMs != nil })
		fs := b.Status()
		for _, s := range fs.Servers {
			if s.Self != (s.ID == id) {
				t.Fatalf("%s: wrong self flag on %s", id, s.ID)
			}
			if !s.Self && s.Metrics != nil {
				t.Fatalf("%s: follower must not report metrics for %s", id, s.ID)
			}
		}
		if fs.CommitIndex == 0 || fs.AppliedIndex == 0 {
			t.Fatalf("%s: empty indexes: %+v", id, fs)
		}
	}

	// The sampler records real points.
	var h Sample
	eventually(t, "leader history sample with peers", func() bool {
		hist := leader.Status().History
		if len(hist) == 0 {
			return false
		}
		h = hist[len(hist)-1]
		return h.IsLeader && h.CommitIndex > 0 && len(h.Peers) == 2
	})
	if h.T == 0 || h.Term == 0 {
		t.Fatalf("unexpected sample: %+v", h)
	}
}

func TestSnapshotListCreateOpen(t *testing.T) {
	b := openSingle(t)
	ctx := context.Background()

	if snaps, err := b.ListSnapshots(); err != nil || len(snaps) != 0 {
		t.Fatalf("fresh node: snaps=%v err=%v", snaps, err)
	}
	if err := b.Put(ctx, entry("snap/a", "1")); err != nil {
		t.Fatal(err)
	}
	info, err := b.CreateSnapshot()
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if info.ID == "" || info.Index == 0 || info.Term == 0 || info.Size <= 0 || info.CreatedAt == "" || info.Voters != 1 {
		t.Fatalf("incomplete metadata: %+v", info)
	}
	if _, perr := time.Parse(time.RFC3339, info.CreatedAt); perr != nil {
		t.Fatalf("created_at: %v", perr)
	}
	// Nothing changed: the second attempt reports it.
	if _, err := b.CreateSnapshot(); !errors.Is(err, ErrNoNewData) {
		t.Fatalf("second snapshot: want ErrNoNewData, got %v", err)
	}
	if err := b.Put(ctx, entry("snap/b", "2")); err != nil {
		t.Fatal(err)
	}
	info2, err := b.CreateSnapshot()
	if err != nil || info2.Index <= info.Index {
		t.Fatalf("third snapshot: %+v err=%v", info2, err)
	}

	list, err := b.ListSnapshots()
	if err != nil || len(list) != 2 || list[0].ID != info2.ID || list[1].ID != info.ID {
		t.Fatalf("list (newest first): %+v err=%v", list, err)
	}

	// The stored snapshot has the FSM dump format and holds the data.
	meta, rc, err := b.OpenSnapshot(info2.ID)
	if err != nil {
		t.Fatalf("OpenSnapshot: %v", err)
	}
	defer func() { _ = rc.Close() }()
	raw, _ := io.ReadAll(rc)
	var data map[string][]byte
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("snapshot is not an FSM dump: %v", err)
	}
	if string(data["snap/a"]) != "1" || string(data["snap/b"]) != "2" || meta.Size != int64(len(raw)) {
		t.Fatalf("snapshot content mismatch (size meta=%d raw=%d)", meta.Size, len(raw))
	}

	// Only IDs the store lists can be opened: no path traversal.
	for _, bad := range []string{"", "..", "../raft.db", "1-2-3", info.ID + "/.."} {
		if _, _, err := b.OpenSnapshot(bad); !errors.Is(err, ErrSnapshotNotFound) {
			t.Fatalf("OpenSnapshot(%q): want ErrSnapshotNotFound, got %v", bad, err)
		}
	}
}

func TestTLSInfo(t *testing.T) {
	b := openSingle(t)
	if b.Status().TLS != nil {
		t.Fatal("plain TCP transport must report no tls block")
	}
	tc := newTestCA(t, "tuck-test-ca").issue(t, "node-a.raft.test")
	b.tlsCfg = tc
	info := b.Status().TLS
	if info == nil || info.Error != "" {
		t.Fatalf("tls info: %+v", info)
	}
	if info.Subject != "CN=node-a.raft.test" || info.Issuer != "CN=tuck-test-ca" || info.CACerts != 1 ||
		info.Serial == "" || len(info.IPAddresses) != 1 || info.DaysLeft == nil || *info.DaysLeft != 0 {
		t.Fatalf("unexpected tls info: %+v", info)
	}
	if _, err := time.Parse(time.RFC3339, info.NotAfter); err != nil {
		t.Fatalf("not_after: %v", err)
	}
	b.tlsCfg = &TLSConfig{CertFile: tc.CertFile + ".missing", CAFile: tc.CAFile}
	if got := b.Status().TLS; got == nil || got.Error == "" {
		t.Fatalf("missing cert must surface an error, got %+v", got)
	}
}

func TestStepDownSingleNode(t *testing.T) {
	b := openSingle(t)
	if err := b.StepDown(""); !errors.Is(err, ErrNoOtherVoter) {
		t.Fatalf("want ErrNoOtherVoter, got %v", err)
	}
	if err := b.StepDown("n1"); !errors.Is(err, ErrNoOtherVoter) && !errors.Is(err, ErrInvalidTransferTarget) {
		t.Fatalf("unexpected: %v", err)
	}
	if !b.IsLeader() {
		t.Fatal("a refused step-down must not change leadership")
	}
}
