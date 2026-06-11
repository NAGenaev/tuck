package raft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	hraft "github.com/hashicorp/raft"
	raftbolt "github.com/hashicorp/raft-boltdb/v2"

	"github.com/NAGenaev/tuck/internal/physical"
)

// ErrNotLeader is returned by write operations on a follower node.
var ErrNotLeader = errors.New("raft: not the cluster leader")

const applyTimeout = 10 * time.Second

// Config holds Raft backend configuration.
type Config struct {
	// NodeID is a unique, stable identifier for this node (e.g. pod name or hostname).
	NodeID string
	// BindAddr is the address this node listens on for Raft RPC (e.g. "0.0.0.0:8201").
	BindAddr string
	// AdvertiseAddr is the address other nodes use to reach this node.
	// Defaults to BindAddr when empty.
	AdvertiseAddr string
	// DataDir is the directory for Raft logs, snapshots, and FSM state.
	DataDir string
	// Bootstrap creates a new single-node cluster or bootstraps a multi-node cluster
	// from Peers. Only set this on the very first start of a fresh cluster.
	Bootstrap bool
	// Peers is a list of voter addresses used alongside this node when Bootstrap=true.
	// Use "<nodeID>=<raftAddr>" format (e.g. "node2=10.0.0.2:8201").
	// If empty, a single-node cluster is bootstrapped.
	Peers []Peer
	// HeartbeatTimeout defaults to 1s. Must be >= LeaderLeaseTimeout.
	HeartbeatTimeout time.Duration
	// ElectionTimeout defaults to 1s.
	ElectionTimeout time.Duration
	// LeaderLeaseTimeout defaults to 500ms. Must be <= HeartbeatTimeout.
	LeaderLeaseTimeout time.Duration
	// SnapshotInterval defaults to 2 minutes.
	SnapshotInterval time.Duration
	// SnapshotThreshold defaults to 8192 log entries.
	SnapshotThreshold uint64
}

// Peer is a Raft cluster member used during bootstrap.
type Peer struct {
	ID   string
	Addr string
}

// ServerInfo describes a Raft cluster member.
type ServerInfo struct {
	ID       string `json:"id"`
	Address  string `json:"address"`
	Suffrage string `json:"suffrage"`
}

// ClusterStatus is returned by Backend.Status.
type ClusterStatus struct {
	IsLeader   bool         `json:"is_leader"`
	Leader     string       `json:"leader"`
	LeaderAddr string       `json:"leader_addr"`
	State      string       `json:"state"`
	Servers    []ServerInfo `json:"servers"`
}

// Backend implements physical.Backend using Raft consensus.
// Writes (Put/Delete) are replicated through the Raft log; reads are served
// from the local FSM state. Only the leader can accept writes.
type Backend struct {
	raft     *hraft.Raft
	fsm      *fsm
	log      *raftbolt.BoltStore
	trans    *hraft.NetworkTransport
	nodeID   string
	advertise string
}

// Open creates or rejoins a Raft cluster and returns a ready Backend.
func Open(cfg Config) (*Backend, error) {
	if cfg.NodeID == "" {
		return nil, errors.New("raft: NodeID is required")
	}
	if cfg.BindAddr == "" {
		return nil, errors.New("raft: BindAddr is required")
	}
	if cfg.DataDir == "" {
		return nil, errors.New("raft: DataDir is required")
	}

	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, fmt.Errorf("raft: create data dir: %w", err)
	}

	machine, err := newFSM(filepath.Join(cfg.DataDir, "fsm.db"))
	if err != nil {
		return nil, err
	}

	logStore, err := raftbolt.NewBoltStore(filepath.Join(cfg.DataDir, "raft.db"))
	if err != nil {
		machine.db.Close()
		return nil, fmt.Errorf("raft: open log store: %w", err)
	}

	snapDir := filepath.Join(cfg.DataDir, "snapshots")
	snapStore, err := hraft.NewFileSnapshotStore(snapDir, 3, nil)
	if err != nil {
		logStore.Close()
		machine.db.Close()
		return nil, fmt.Errorf("raft: open snapshot store: %w", err)
	}

	advertise := cfg.AdvertiseAddr
	if advertise == "" {
		advertise = cfg.BindAddr
	}
	advAddr, err := net.ResolveTCPAddr("tcp", advertise)
	if err != nil {
		logStore.Close()
		machine.db.Close()
		return nil, fmt.Errorf("raft: resolve advertise addr %q: %w", advertise, err)
	}
	transport, err := hraft.NewTCPTransport(cfg.BindAddr, advAddr, 5, 10*time.Second, nil)
	if err != nil {
		logStore.Close()
		machine.db.Close()
		return nil, fmt.Errorf("raft: create transport: %w", err)
	}

	rc := hraft.DefaultConfig()
	rc.LocalID = hraft.ServerID(cfg.NodeID)
	rc.LogLevel = "WARN"
	if cfg.HeartbeatTimeout > 0 {
		rc.HeartbeatTimeout = cfg.HeartbeatTimeout
	}
	if cfg.ElectionTimeout > 0 {
		rc.ElectionTimeout = cfg.ElectionTimeout
	}
	if cfg.LeaderLeaseTimeout > 0 {
		rc.LeaderLeaseTimeout = cfg.LeaderLeaseTimeout
	}
	if cfg.SnapshotInterval > 0 {
		rc.SnapshotInterval = cfg.SnapshotInterval
	}
	if cfg.SnapshotThreshold > 0 {
		rc.SnapshotThreshold = cfg.SnapshotThreshold
	}

	r, err := hraft.NewRaft(rc, machine, logStore, logStore, snapStore, transport)
	if err != nil {
		transport.Close()
		logStore.Close()
		machine.db.Close()
		return nil, fmt.Errorf("raft: create raft: %w", err)
	}

	if cfg.Bootstrap {
		servers := []hraft.Server{{
			ID:      hraft.ServerID(cfg.NodeID),
			Address: hraft.ServerAddress(advertise),
		}}
		for _, p := range cfg.Peers {
			servers = append(servers, hraft.Server{
				ID:      hraft.ServerID(p.ID),
				Address: hraft.ServerAddress(p.Addr),
			})
		}
		clusterCfg := hraft.Configuration{Servers: servers}
		if f := r.BootstrapCluster(clusterCfg); f.Error() != nil && f.Error() != hraft.ErrCantBootstrap {
			transport.Close()
			logStore.Close()
			machine.db.Close()
			return nil, fmt.Errorf("raft: bootstrap: %w", f.Error())
		}
	}

	return &Backend{
		raft:      r,
		fsm:       machine,
		log:       logStore,
		trans:     transport,
		nodeID:    cfg.NodeID,
		advertise: advertise,
	}, nil
}

// IsLeader reports whether this node is the current Raft leader.
func (b *Backend) IsLeader() bool { return b.raft.State() == hraft.Leader }

// LeaderAddr returns the Raft RPC address of the current leader, or "" if unknown.
func (b *Backend) LeaderAddr() string {
	addr, _ := b.raft.LeaderWithID()
	return string(addr)
}

// AddVoter adds a new voter to the cluster. Must be called on the leader.
func (b *Backend) AddVoter(id, addr string) error {
	f := b.raft.AddVoter(hraft.ServerID(id), hraft.ServerAddress(addr), 0, applyTimeout)
	return f.Error()
}

// RemoveServer removes a server from the cluster. Must be called on the leader.
func (b *Backend) RemoveServer(id string) error {
	f := b.raft.RemoveServer(hraft.ServerID(id), 0, applyTimeout)
	return f.Error()
}

// ClusterStatus implements core.ClusterBackender — returns Status() as any.
func (b *Backend) ClusterStatus() any { return b.Status() }

// Status returns the current cluster topology and this node's role.
func (b *Backend) Status() ClusterStatus {
	leaderAddr, leaderID := b.raft.LeaderWithID()
	cs := ClusterStatus{
		IsLeader:   b.IsLeader(),
		Leader:     string(leaderID),
		LeaderAddr: string(leaderAddr),
		State:      b.raft.State().String(),
	}
	cfgFuture := b.raft.GetConfiguration()
	if err := cfgFuture.Error(); err == nil {
		cfg := cfgFuture.Configuration()
		for _, srv := range cfg.Servers {
			suf := "Voter"
			if srv.Suffrage == hraft.Nonvoter {
				suf = "Nonvoter"
			} else if srv.Suffrage == hraft.Staging {
				suf = "Staging"
			}
			cs.Servers = append(cs.Servers, ServerInfo{
				ID:       string(srv.ID),
				Address:  string(srv.Address),
				Suffrage: suf,
			})
		}
	}
	return cs
}

// Get returns the entry for key from the local FSM state, or (nil, nil) if absent.
func (b *Backend) Get(_ context.Context, key string) (*physical.Entry, error) {
	return b.fsm.get(key)
}

// Put replicates a put operation through Raft. Returns ErrNotLeader if this
// node is not currently the cluster leader.
func (b *Backend) Put(_ context.Context, entry *physical.Entry) error {
	return b.apply("put", entry.Key, entry.Value)
}

// Delete replicates a delete operation through Raft. Returns ErrNotLeader if
// this node is not the leader.
func (b *Backend) Delete(_ context.Context, key string) error {
	return b.apply("delete", key, nil)
}

// List returns keys with the given prefix from the local FSM state.
func (b *Backend) List(_ context.Context, prefix string) ([]string, error) {
	return b.fsm.list(prefix)
}

// Snapshot writes the FSM state to w as JSON for backup purposes.
func (b *Backend) Snapshot(_ context.Context, w io.Writer) error {
	snap, err := b.fsm.Snapshot()
	if err != nil {
		return fmt.Errorf("raft: snapshot FSM: %w", err)
	}
	sink := &writerSink{w: w}
	return snap.Persist(sink)
}

// Close shuts down the Raft node gracefully.
func (b *Backend) Close() error {
	if f := b.raft.Shutdown(); f.Error() != nil {
		return f.Error()
	}
	b.trans.Close()
	b.log.Close()
	return b.fsm.db.Close()
}

func (b *Backend) apply(op, key string, value []byte) error {
	if !b.IsLeader() {
		return ErrNotLeader
	}
	cmd := command{Op: op, Key: key, Value: value}
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("raft: marshal command: %w", err)
	}
	f := b.raft.Apply(data, applyTimeout)
	if f.Error() != nil {
		return fmt.Errorf("raft: apply: %w", f.Error())
	}
	if resp := f.Response(); resp != nil {
		if err, ok := resp.(error); ok {
			return err
		}
	}
	return nil
}

// writerSink adapts an io.Writer to hraft.SnapshotSink for backup use.
type writerSink struct{ w io.Writer }

func (s *writerSink) Write(p []byte) (int, error) { return s.w.Write(p) }
func (s *writerSink) Close() error                { return nil }
func (s *writerSink) Cancel() error               { return nil }
func (s *writerSink) ID() string                  { return "backup" }
