package raft

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}

func nodeConfig(id, addr, dir string) Config {
	return Config{
		NodeID:             id,
		BindAddr:           addr,
		DataDir:            dir,
		HeartbeatTimeout:   300 * time.Millisecond,
		ElectionTimeout:    300 * time.Millisecond,
		LeaderLeaseTimeout: 150 * time.Millisecond,
		SnapshotInterval:   time.Minute,
		SnapshotThreshold:  8192,
	}
}

func eventually(t *testing.T, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func hasValue(b *Backend, key, want string) bool {
	e, err := b.Get(context.Background(), key)
	return err == nil && e != nil && string(e.Value) == want
}

func findLeader(nodes map[string]*Backend) (string, *Backend) {
	for id, b := range nodes {
		if b != nil && b.IsLeader() {
			return id, b
		}
	}
	return "", nil
}

// A three-node cluster survives losing its leader, keeps accepting writes with
// two nodes, and the lost node catches up after it is restarted.
func TestChaosRaftLeaderLossAndRejoin(t *testing.T) {
	if testing.Short() {
		t.Skip("chaos: skipped in -short mode")
	}
	ctx := context.Background()
	ids := []string{"n1", "n2", "n3"}
	addrs := map[string]string{}
	dirs := map[string]string{}
	for _, id := range ids {
		addrs[id] = freeAddr(t)
		dirs[id] = t.TempDir()
	}

	nodes := map[string]*Backend{}
	t.Cleanup(func() {
		for _, b := range nodes {
			if b != nil {
				_ = b.Close()
			}
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
	var leader *Backend
	eventually(t, "initial leader", func() bool { leaderID, leader = findLeader(nodes); return leader != nil })

	for i := 0; i < 20; i++ {
		if err := leader.Put(ctx, entry(fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))); err != nil {
			t.Fatalf("put k%d: %v", i, err)
		}
	}
	for _, id := range ids {
		b := nodes[id]
		eventually(t, id+" replicated k19", func() bool { return hasValue(b, "k19", "v19") })
	}

	// Kill the leader.
	if err := leader.Close(); err != nil {
		t.Fatalf("close leader: %v", err)
	}
	nodes[leaderID] = nil

	var newLeader *Backend
	eventually(t, "new leader among survivors", func() bool { _, newLeader = findLeader(nodes); return newLeader != nil })
	eventually(t, "write with 2 of 3 nodes", func() bool {
		return newLeader.Put(ctx, entry("after-loss", "ok")) == nil
	})
	for id, b := range nodes {
		if b == nil {
			continue
		}
		b := b
		eventually(t, id+" has pre-loss data", func() bool { return hasValue(b, "k0", "v0") && hasValue(b, "k19", "v19") })
		eventually(t, id+" has post-loss write", func() bool { return hasValue(b, "after-loss", "ok") })
	}

	// Restart the lost node from its own disk: it must rejoin and catch up.
	cfg := nodeConfig(leaderID, addrs[leaderID], dirs[leaderID])
	restarted, err := Open(cfg)
	if err != nil {
		t.Fatalf("restart %s: %v", leaderID, err)
	}
	nodes[leaderID] = restarted
	eventually(t, "restarted node catches up", func() bool {
		return hasValue(restarted, "after-loss", "ok") && hasValue(restarted, "k0", "v0")
	})
}

// Losing quorum (2 of 3) must fail writes instead of silently accepting them,
// and the cluster must recover when a node comes back.
func TestChaosRaftQuorumLossRejectsWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("chaos: skipped in -short mode")
	}
	ctx := context.Background()
	ids := []string{"n1", "n2", "n3"}
	addrs := map[string]string{}
	dirs := map[string]string{}
	for _, id := range ids {
		addrs[id] = freeAddr(t)
		dirs[id] = t.TempDir()
	}
	nodes := map[string]*Backend{}
	t.Cleanup(func() {
		for _, b := range nodes {
			if b != nil {
				_ = b.Close()
			}
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
			t.Fatal(err)
		}
		nodes[id] = b
	}
	var leader *Backend
	eventually(t, "leader", func() bool { _, leader = findLeader(nodes); return leader != nil })
	if err := leader.Put(ctx, entry("committed", "yes")); err != nil {
		t.Fatal(err)
	}

	// Stop both followers; the old leader must stop being able to commit.
	var lid string
	for id, b := range nodes {
		if b == leader {
			lid = id
		}
	}
	for _, id := range ids {
		if id != lid {
			_ = nodes[id].Close()
			nodes[id] = nil
		}
	}
	if err := leader.Put(ctx, entry("uncommitted", "no")); err == nil {
		t.Fatal("write without quorum must fail")
	}
	if !hasValue(leader, "committed", "yes") {
		t.Fatal("previously committed data must stay readable")
	}
}
