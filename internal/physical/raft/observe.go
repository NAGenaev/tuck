package raft

import (
	"strconv"
	"sync"
	"time"

	hraft "github.com/hashicorp/raft"
)

const historySize = 60

// sampleInterval is how often the history ring takes a sample.
var sampleInterval = 5 * time.Second

// ServerMetrics is the replication state of one server as seen by the node
// that answered the request. Nothing here is estimated: the numbers are what
// the Raft library or the heartbeat responses reported.
type ServerMetrics struct {
	// Source is "local" for the answering node itself and "heartbeat" for a
	// follower observed by the leader.
	Source string `json:"source"`
	Term   uint64 `json:"term"`
	// LastLogIndex is the last log entry the server reported having stored.
	LastLogIndex uint64 `json:"last_log_index"`
	// LagEntries is how many entries behind the leader's log the follower was
	// at its last heartbeat (0 for the leader). Only set when Source=heartbeat.
	LagEntries *uint64 `json:"lag_entries,omitempty"`
	// LastContactMs is the age of the last successful exchange with the server.
	LastContactMs *int64 `json:"last_contact_ms,omitempty"`
	// HeartbeatRttMs is the round trip of the last heartbeat to the server.
	HeartbeatRttMs *float64 `json:"heartbeat_rtt_ms,omitempty"`
	// Reachable is false when the leader has no successful heartbeat within
	// twice the heartbeat timeout, or the last attempt failed.
	Reachable *bool `json:"reachable,omitempty"`
	// Failures is the number of consecutive failed heartbeats.
	Failures uint64 `json:"failures,omitempty"`
	// CommitIndex / AppliedIndex are only known for the answering node.
	CommitIndex  *uint64 `json:"commit_index,omitempty"`
	AppliedIndex *uint64 `json:"applied_index,omitempty"`
}

// PeerSample is one peer in a history Sample.
type PeerSample struct {
	RttMs *float64 `json:"rtt_ms,omitempty"`
	Lag   uint64   `json:"lag"`
}

// Sample is one point of the history ring, taken every few seconds.
type Sample struct {
	// T is the Unix time in milliseconds.
	T            int64  `json:"t"`
	Term         uint64 `json:"term"`
	CommitIndex  uint64 `json:"commit_index"`
	AppliedIndex uint64 `json:"applied_index"`
	LastLogIndex uint64 `json:"last_log_index"`
	IsLeader     bool   `json:"is_leader"`
	// Peers is filled only while this node is the leader.
	Peers map[string]PeerSample `json:"peers,omitempty"`
}

type peerObs struct {
	lastContact time.Time
	rtt         time.Duration
	hasRTT      bool
	lastLog     uint64
	term        uint64
	failures    uint64
}

// observer records what the leader learns about followers from heartbeats and
// keeps the ring of samples.
type observer struct {
	mu    sync.Mutex
	peers map[hraft.ServerID]*peerObs
	ring  []Sample
}

func newObserver() *observer {
	return &observer{peers: map[hraft.ServerID]*peerObs{}}
}

func (o *observer) record(id hraft.ServerID, heartbeat bool, rtt time.Duration, resp *hraft.AppendEntriesResponse, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	p := o.peers[id]
	if p == nil {
		p = &peerObs{}
		o.peers[id] = p
	}
	if err != nil {
		p.failures++
		return
	}
	p.failures = 0
	p.lastContact = time.Now()
	p.lastLog = resp.LastLog
	p.term = resp.Term
	if heartbeat {
		p.rtt, p.hasRTT = rtt, true
	}
}

func (o *observer) peer(id hraft.ServerID) (peerObs, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	p := o.peers[id]
	if p == nil {
		return peerObs{}, false
	}
	return *p, true
}

func (o *observer) push(s Sample) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.ring = append(o.ring, s)
	if len(o.ring) > historySize {
		o.ring = append([]Sample(nil), o.ring[len(o.ring)-historySize:]...)
	}
}

func (o *observer) history() []Sample {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]Sample(nil), o.ring...)
}

// observedTransport wraps the network transport to observe AppendEntries
// responses (heartbeats included) on the leader. Every other method, and the
// optional WithClose / WithPreVote interfaces, are promoted unchanged.
type observedTransport struct {
	*hraft.NetworkTransport
	obs *observer
}

func (t *observedTransport) AppendEntries(id hraft.ServerID, target hraft.ServerAddress, args *hraft.AppendEntriesRequest, resp *hraft.AppendEntriesResponse) error {
	start := time.Now()
	err := t.NetworkTransport.AppendEntries(id, target, args, resp)
	t.obs.record(id, len(args.Entries) == 0, time.Since(start), resp, err)
	return err
}

type localStats struct {
	term, commit, applied, lastLog, lastLogTerm uint64
	snapIndex, snapTerm, fsmPending             uint64
	lastContact                                 time.Time
}

func (b *Backend) localStats() localStats {
	m := b.raft.Stats()
	u := func(k string) uint64 {
		v, _ := strconv.ParseUint(m[k], 10, 64)
		return v
	}
	return localStats{
		term:        u("term"),
		commit:      u("commit_index"),
		applied:     u("applied_index"),
		lastLog:     u("last_log_index"),
		lastLogTerm: u("last_log_term"),
		snapIndex:   u("last_snapshot_index"),
		snapTerm:    u("last_snapshot_term"),
		fsmPending:  u("fsm_pending"),
		lastContact: b.raft.LastContact(),
	}
}

// servers lists the configuration with the metrics this node can really know.
func (b *Backend) servers(isLeader bool, st localStats) []ServerInfo {
	f := b.raft.GetConfiguration()
	if err := f.Error(); err != nil {
		return nil
	}
	var out []ServerInfo
	for _, srv := range f.Configuration().Servers {
		suf := "Voter"
		switch srv.Suffrage {
		case hraft.Nonvoter:
			suf = "Nonvoter"
		case hraft.Staging:
			suf = "Staging"
		}
		info := ServerInfo{ID: string(srv.ID), Address: string(srv.Address), Suffrage: suf}
		switch {
		case string(srv.ID) == b.nodeID:
			info.Self = true
			commit, applied := st.commit, st.applied
			m := &ServerMetrics{
				Source:       "local",
				Term:         st.term,
				LastLogIndex: st.lastLog,
				CommitIndex:  &commit,
				AppliedIndex: &applied,
			}
			if isLeader {
				zero := uint64(0)
				m.LagEntries = &zero
			} else if !st.lastContact.IsZero() {
				ms := time.Since(st.lastContact).Milliseconds()
				m.LastContactMs = &ms
			}
			info.Metrics = m
		case isLeader:
			if p, ok := b.obs.peer(srv.ID); ok && !p.lastContact.IsZero() {
				info.Metrics = b.peerMetrics(p, st.lastLog)
			}
		}
		out = append(out, info)
	}
	return out
}

func (b *Backend) peerMetrics(p peerObs, leaderLast uint64) *ServerMetrics {
	age := time.Since(p.lastContact)
	ms := age.Milliseconds()
	var lag uint64
	if leaderLast > p.lastLog {
		lag = leaderLast - p.lastLog
	}
	reach := p.failures == 0 && age < 2*b.heartbeat
	m := &ServerMetrics{
		Source:        "heartbeat",
		Term:          p.term,
		LastLogIndex:  p.lastLog,
		LagEntries:    &lag,
		LastContactMs: &ms,
		Reachable:     &reach,
		Failures:      p.failures,
	}
	if p.hasRTT {
		rtt := float64(p.rtt.Microseconds()) / 1000
		m.HeartbeatRttMs = &rtt
	}
	return m
}

func (b *Backend) sampleLoop() {
	t := time.NewTicker(sampleInterval)
	defer t.Stop()
	b.takeSample()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			b.takeSample()
		}
	}
}

func (b *Backend) takeSample() {
	st := b.localStats()
	leader := b.IsLeader()
	s := Sample{
		T:            time.Now().UnixMilli(),
		Term:         st.term,
		CommitIndex:  st.commit,
		AppliedIndex: st.applied,
		LastLogIndex: st.lastLog,
		IsLeader:     leader,
	}
	if leader {
		s.Peers = map[string]PeerSample{}
		for _, srv := range b.servers(true, st) {
			if srv.Self || srv.Metrics == nil || srv.Metrics.LagEntries == nil {
				continue
			}
			s.Peers[srv.ID] = PeerSample{Lag: *srv.Metrics.LagEntries, RttMs: srv.Metrics.HeartbeatRttMs}
		}
	}
	b.obs.push(s)
}
