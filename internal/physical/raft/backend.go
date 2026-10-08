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
	"sync"
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
	// TLS, if set, turns on mutual TLS for Raft RPC.
	TLS *TLSConfig
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
	// Self marks the node that answered the request.
	Self bool `json:"self,omitempty"`
	// Metrics is the replication view of this server as seen from the node that
	// answered the request: exact for that node itself, observed through
	// heartbeats for followers when it is the leader, absent otherwise.
	Metrics *ServerMetrics `json:"metrics,omitempty"`
}

// ClusterStatus is returned by Backend.Status.
type ClusterStatus struct {
	IsLeader   bool         `json:"is_leader"`
	Leader     string       `json:"leader"`
	LeaderAddr string       `json:"leader_addr"`
	State      string       `json:"state"`
	Servers    []ServerInfo `json:"servers"`

	// Additive fields: local Raft statistics of the node that answered.
	NodeID            string `json:"node_id,omitempty"`
	AdvertiseAddr     string `json:"advertise_addr,omitempty"`
	Term              uint64 `json:"term"`
	CommitIndex       uint64 `json:"commit_index"`
	AppliedIndex      uint64 `json:"applied_index"`
	LastLogIndex      uint64 `json:"last_log_index"`
	LastLogTerm       uint64 `json:"last_log_term"`
	LastSnapshotIndex uint64 `json:"last_snapshot_index"`
	LastSnapshotTerm  uint64 `json:"last_snapshot_term"`
	FSMPending        uint64 `json:"fsm_pending"`
	// LastContactMs is the time since this node last heard from the leader;
	// null on the leader itself or before the first contact.
	LastContactMs      *int64 `json:"last_contact_ms"`
	HeartbeatTimeoutMs int64  `json:"heartbeat_timeout_ms"`
	// TLS describes this node's Raft transport certificate; nil without mTLS.
	TLS *TLSInfo `json:"tls,omitempty"`
	// History is a ring of the most recent real samples taken by this node.
	History []Sample `json:"history,omitempty"`
}

// Backend implements physical.Backend using Raft consensus.
// Writes (Put/Delete) are replicated through the Raft log; reads are served
// from the local FSM state. Only the leader can accept writes.
type Backend struct {
	raft      *hraft.Raft
	fsm       *fsm
	log       *raftbolt.BoltStore
	trans     *hraft.NetworkTransport
	nodeID    string
	advertise string

	snaps     hraft.SnapshotStore
	obs       *observer
	tlsCfg    *TLSConfig
	heartbeat time.Duration
	stopOnce  sync.Once
	stop      chan struct{}
	tlsMu     sync.Mutex
	tlsCache  *tlsCacheEntry
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
		_ = machine.db.Close()
		return nil, fmt.Errorf("raft: open log store: %w", err)
	}

	snapDir := filepath.Join(cfg.DataDir, "snapshots")
	snapStore, err := hraft.NewFileSnapshotStore(snapDir, 3, nil)
	if err != nil {
		_ = logStore.Close()
		_ = machine.db.Close()
		return nil, fmt.Errorf("raft: open snapshot store: %w", err)
	}

	advertise := cfg.AdvertiseAddr
	if advertise == "" {
		advertise = cfg.BindAddr
	}
	advAddr, err := net.ResolveTCPAddr("tcp", advertise)
	if err != nil {
		_ = logStore.Close()
		_ = machine.db.Close()
		return nil, fmt.Errorf("raft: resolve advertise addr %q: %w", advertise, err)
	}
	var transport *hraft.NetworkTransport
	if cfg.TLS != nil {
		var stream *tlsStream
		if stream, err = newTLSStream(cfg.BindAddr, advAddr, cfg.TLS); err == nil {
			transport = hraft.NewNetworkTransport(stream, 5, 10*time.Second, nil)
		}
	} else {
		transport, err = hraft.NewTCPTransport(cfg.BindAddr, advAddr, 5, 10*time.Second, nil)
	}
	if err != nil {
		_ = logStore.Close()
		_ = machine.db.Close()
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

	obs := newObserver()
	r, err := hraft.NewRaft(rc, machine, logStore, logStore, snapStore, &observedTransport{NetworkTransport: transport, obs: obs})
	if err != nil {
		_ = transport.Close()
		_ = logStore.Close()
		_ = machine.db.Close()
		return nil, fmt.Errorf("raft: create raft: %w", err)
	}

	if cfg.Bootstrap {
		servers := []hraft.Server{{
			ID:      hraft.ServerID(cfg.NodeID),
			Address: hraft.ServerAddress(advertise),
		}}
		for _, p := range cfg.Peers {
			if p.ID == cfg.NodeID {
				continue // the same peer list may be shared by every node
			}
			servers = append(servers, hraft.Server{
				ID:      hraft.ServerID(p.ID),
				Address: hraft.ServerAddress(p.Addr),
			})
		}
		clusterCfg := hraft.Configuration{Servers: servers}
		if f := r.BootstrapCluster(clusterCfg); f.Error() != nil && f.Error() != hraft.ErrCantBootstrap {
			_ = transport.Close()
			_ = logStore.Close()
			_ = machine.db.Close()
			return nil, fmt.Errorf("raft: bootstrap: %w", f.Error())
		}
	}

	b := &Backend{
		raft:      r,
		fsm:       machine,
		log:       logStore,
		trans:     transport,
		nodeID:    cfg.NodeID,
		advertise: advertise,
		snaps:     snapStore,
		obs:       obs,
		tlsCfg:    cfg.TLS,
		heartbeat: rc.HeartbeatTimeout,
		stop:      make(chan struct{}),
	}
	go b.sampleLoop()
	return b, nil
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

// Status returns the current cluster topology, this node's role and its local
// Raft statistics.
func (b *Backend) Status() ClusterStatus {
	leaderAddr, leaderID := b.raft.LeaderWithID()
	cs := ClusterStatus{
		IsLeader:           b.IsLeader(),
		Leader:             string(leaderID),
		LeaderAddr:         string(leaderAddr),
		State:              b.raft.State().String(),
		NodeID:             b.nodeID,
		AdvertiseAddr:      b.advertise,
		HeartbeatTimeoutMs: b.heartbeat.Milliseconds(),
	}
	st := b.localStats()
	cs.Term, cs.CommitIndex, cs.AppliedIndex = st.term, st.commit, st.applied
	cs.LastLogIndex, cs.LastLogTerm = st.lastLog, st.lastLogTerm
	cs.LastSnapshotIndex, cs.LastSnapshotTerm = st.snapIndex, st.snapTerm
	cs.FSMPending = st.fsmPending
	if !cs.IsLeader && !st.lastContact.IsZero() {
		ms := time.Since(st.lastContact).Milliseconds()
		cs.LastContactMs = &ms
	}
	cs.Servers = b.servers(cs.IsLeader, st)
	cs.TLS = b.tlsInfo()
	cs.History = b.obs.history()
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

// ErrEmptySnapshot is returned by Restore for a snapshot without any entry.
var ErrEmptySnapshot = errors.New("raft: snapshot is empty")

// OnRestore registers fn to be called on every node after a restore command
// was applied. Tuck seals the node there: the restored data carries the key
// ring of the cluster the snapshot came from, so it must be unsealed again
// with that cluster's key shares.
func (b *Backend) OnRestore(fn func()) { b.fsm.onRestore.Store(&fn) }

// Restore replaces the data of the whole cluster with the snapshot read from r
// (the stream written by Snapshot / downloaded from /v1/sys/snapshot). It runs
// as one replicated command, so every voter switches atomically. Leader only.
// It returns the number of restored entries.
func (b *Backend) Restore(r io.Reader, maxBytes int64) (int, error) {
	if !b.IsLeader() {
		return 0, ErrNotLeader
	}
	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return 0, fmt.Errorf("raft: read snapshot: %w", err)
	}
	if int64(len(raw)) > maxBytes {
		return 0, fmt.Errorf("raft: snapshot larger than %d bytes", maxBytes)
	}
	var data map[string][]byte
	if err := json.Unmarshal(raw, &data); err != nil {
		return 0, fmt.Errorf("raft: not a Tuck snapshot: %w", err)
	}
	if err := validateRestoreData(data); err != nil {
		return 0, err
	}
	payload, err := json.Marshal(command{Op: "restore", Value: raw})
	if err != nil {
		return 0, fmt.Errorf("raft: marshal restore: %w", err)
	}
	f := b.raft.Apply(payload, 60*time.Second)
	if err := f.Error(); err != nil {
		return 0, fmt.Errorf("raft: apply restore: %w", err)
	}
	if resp := f.Response(); resp != nil {
		if err, ok := resp.(error); ok {
			return 0, err
		}
	}
	return len(data), nil
}

// Close shuts down the Raft node gracefully.
func (b *Backend) Close() error {
	b.stopOnce.Do(func() { close(b.stop) })
	if f := b.raft.Shutdown(); f.Error() != nil {
		return f.Error()
	}
	_ = b.trans.Close()
	_ = b.log.Close()
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
