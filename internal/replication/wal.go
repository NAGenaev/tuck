// Package replication implements the write-ahead log (WAL) used as the
// replication stream between a primary and secondary Tuck cluster.
// WAL entries are immutable, sequentially numbered records that describe
// every barrier write so that secondaries can replay them in order.
package replication

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

const (
	walPrefix = "replication/wal/"
	statKey   = "replication/state"

	// MaxEntrySize is the maximum WAL entry value size (1 MiB).
	MaxEntrySize = 1 << 20

	// ReplicaModeDisabled means replication is not configured.
	ReplicaModeDisabled = "disabled"
	// ReplicaModePrimary is the write-accepting leader.
	ReplicaModePrimary = "primary"
	// ReplicaModeSecondary is a read-only follower.
	ReplicaModeSecondary = "secondary"
)

var (
	// ErrNotPrimary is returned when a write is attempted on a secondary.
	ErrNotPrimary = errors.New("replication: this node is not the primary")
	// ErrSequenceGap is returned if WAL entries are non-contiguous.
	ErrSequenceGap = errors.New("replication: sequence gap in WAL")
)

// Entry is a single WAL record.
type Entry struct {
	Sequence  uint64    `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	Operation string    `json:"operation"` // "put" | "delete"
	Key       string    `json:"key"`
	Value     []byte    `json:"value,omitempty"` // nil for deletes
}

// State describes the replication state of this node.
type State struct {
	Mode        string    `json:"mode"`         // disabled | primary | secondary
	LastSequence uint64   `json:"last_sequence"`
	UpdatedAt   time.Time `json:"updated_at"`
	PrimaryAddr string    `json:"primary_addr,omitempty"` // set on secondary
}

// barrierer is the subset of barrier.Barrier used by the WAL.
type barrierer interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// WAL is the write-ahead log manager.
type WAL struct {
	mu   sync.Mutex
	b    barrierer
	mode string
	seq  uint64 // in-memory cache of last sequence; 0 = unloaded
}

// New returns a WAL backed by the given barrier. The WAL starts in disabled mode.
func New(b barrierer) *WAL { return &WAL{b: b, mode: ReplicaModeDisabled} }

// SetMode switches the replication mode. On transition to primary the WAL
// sequence is loaded from storage. On transition to disabled the in-memory
// sequence is reset.
func (w *WAL) SetMode(ctx context.Context, mode, primaryAddr string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	st := &State{Mode: mode, PrimaryAddr: primaryAddr, UpdatedAt: time.Now().UTC()}
	if mode == ReplicaModePrimary || mode == ReplicaModeSecondary {
		seq, err := w.loadSeq(ctx)
		if err != nil {
			return fmt.Errorf("load WAL sequence: %w", err)
		}
		w.seq = seq
		st.LastSequence = seq
	} else {
		w.seq = 0
	}
	w.mode = mode
	return w.saveState(ctx, st)
}

// Append writes a new WAL entry. Only allowed on the primary.
func (w *WAL) Append(ctx context.Context, op, key string, value []byte) (*Entry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.mode != ReplicaModePrimary {
		return nil, ErrNotPrimary
	}
	w.seq++
	e := &Entry{
		Sequence:  w.seq,
		Timestamp: time.Now().UTC(),
		Operation: op,
		Key:       key,
		Value:     value,
	}
	if err := w.writeEntry(ctx, e); err != nil {
		w.seq-- // roll back on failure
		return nil, err
	}
	return e, nil
}

// ReadFrom returns all WAL entries with Sequence > afterSeq, in order.
func (w *WAL) ReadFrom(ctx context.Context, afterSeq uint64) ([]*Entry, error) {
	keys, err := w.b.List(ctx, walPrefix)
	if err != nil {
		return nil, err
	}
	var out []*Entry
	for _, k := range keys {
		seq, err := seqFromKey(k)
		if err != nil || seq <= afterSeq {
			continue
		}
		raw, err := w.b.Get(ctx, k)
		if err != nil || raw == nil {
			continue
		}
		var e Entry
		if err := json.Unmarshal(raw.Value, &e); err != nil {
			continue
		}
		out = append(out, &e)
	}
	sortEntries(out)
	return out, nil
}

// GetState returns the persisted replication state.
func (w *WAL) GetState(ctx context.Context) (*State, error) {
	raw, err := w.b.Get(ctx, statKey)
	if err != nil || raw == nil {
		return &State{Mode: ReplicaModeDisabled}, nil
	}
	var st State
	if err := json.Unmarshal(raw.Value, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// TrimBefore removes WAL entries with Sequence < minSeq. Used by the primary
// to prune entries that all secondaries have acknowledged.
func (w *WAL) TrimBefore(ctx context.Context, minSeq uint64) error {
	keys, err := w.b.List(ctx, walPrefix)
	if err != nil {
		return err
	}
	for _, k := range keys {
		seq, err := seqFromKey(k)
		if err != nil || seq >= minSeq {
			continue
		}
		_ = w.b.Delete(ctx, k)
	}
	return nil
}

func (w *WAL) writeEntry(ctx context.Context, e *Entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return w.b.Put(ctx, &physical.Entry{Key: walKey(e.Sequence), Value: b})
}

func (w *WAL) loadSeq(ctx context.Context) (uint64, error) {
	keys, err := w.b.List(ctx, walPrefix)
	if err != nil {
		return 0, err
	}
	var max uint64
	for _, k := range keys {
		seq, err := seqFromKey(k)
		if err == nil && seq > max {
			max = seq
		}
	}
	return max, nil
}

func (w *WAL) saveState(ctx context.Context, st *State) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return w.b.Put(ctx, &physical.Entry{Key: statKey, Value: b})
}

// walKey encodes a sequence number as a zero-padded 20-digit decimal key
// so that lexicographic ordering equals numeric ordering.
func walKey(seq uint64) string {
	return walPrefix + fmt.Sprintf("%020d", seq)
}

func seqFromKey(k string) (uint64, error) {
	base := strings.TrimPrefix(k, walPrefix)
	base = strings.TrimPrefix(base, "/") // strip extra prefix if List returned full key
	// The key ends with the 20-digit sequence; parse the last segment.
	parts := strings.Split(base, "/")
	return strconv.ParseUint(parts[len(parts)-1], 10, 64)
}

// sortEntries sorts entries by Sequence ascending (insertion sort — small slices).
func sortEntries(es []*Entry) {
	for i := 1; i < len(es); i++ {
		for j := i; j > 0 && es[j].Sequence < es[j-1].Sequence; j-- {
			es[j], es[j-1] = es[j-1], es[j]
		}
	}
}

// uint64ToBytes converts a uint64 to 8 big-endian bytes (used in tests).
func uint64ToBytes(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, n)
	return b
}

var _ = uint64ToBytes // suppress unused warning
