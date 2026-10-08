// Package raft implements physical.Backend using Raft consensus for HA deployments.
package raft

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	hraft "github.com/hashicorp/raft"
	bolt "go.etcd.io/bbolt"

	"github.com/NAGenaev/tuck/internal/physical"
)

const dataBucket = "data"

// restoreReactWindow is how fresh a restore entry must be for a node to seal
// itself because of it.
const restoreReactWindow = 5 * time.Minute

type command struct {
	Op    string `json:"op"`             // "put" | "delete" | "restore" (Value = JSON map of the whole data set)
	Key   string `json:"key"`
	Value []byte `json:"value,omitempty"`
}

// fsm is the Raft finite state machine backed by bbolt.
type fsm struct {
	db *bolt.DB

	// onRestore, when set, is called (asynchronously) after a "restore" command
	// was applied, so the node can seal itself and re-read its keyring.
	onRestore func()
}

func newFSM(path string) (*fsm, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("open FSM db: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(dataBucket))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init FSM bucket: %w", err)
	}
	return &fsm{db: db}, nil
}

func (f *fsm) Apply(l *hraft.Log) interface{} {
	var cmd command
	if err := json.Unmarshal(l.Data, &cmd); err != nil {
		return fmt.Errorf("unmarshal command: %w", err)
	}
	if cmd.Op == "restore" {
		var data map[string][]byte
		if err := json.Unmarshal(cmd.Value, &data); err != nil {
			return fmt.Errorf("decode restore payload: %w", err)
		}
		if err := f.replaceAll(data); err != nil {
			return err
		}
		// A node that merely replays an old log entry on startup must not seal
		// itself again: only react to a restore that has just happened.
		if f.onRestore != nil && time.Since(l.AppendedAt) < restoreReactWindow {
			go f.onRestore()
		}
		return nil
	}
	return f.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(dataBucket))
		if b == nil {
			return fmt.Errorf("data bucket missing")
		}
		switch cmd.Op {
		case "put":
			return b.Put([]byte(cmd.Key), cmd.Value)
		case "delete":
			return b.Delete([]byte(cmd.Key))
		default:
			return fmt.Errorf("unknown op: %s", cmd.Op)
		}
	})
}

func (f *fsm) Snapshot() (hraft.FSMSnapshot, error) {
	data := make(map[string][]byte)
	if err := f.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(dataBucket))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			kc := make([]byte, len(k))
			vc := make([]byte, len(v))
			copy(kc, k)
			copy(vc, v)
			data[string(kc)] = vc
			return nil
		})
	}); err != nil {
		return nil, fmt.Errorf("snapshot read: %w", err)
	}
	return &fsmSnapshot{data: data}, nil
}

func (f *fsm) Restore(rc io.ReadCloser) error {
	defer func() { _ = rc.Close() }()
	var data map[string][]byte
	if err := json.NewDecoder(rc).Decode(&data); err != nil {
		return fmt.Errorf("decode snapshot: %w", err)
	}
	return f.replaceAll(data)
}

// replaceAll atomically replaces the whole data set with data.
func (f *fsm) replaceAll(data map[string][]byte) error {
	return f.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte(dataBucket)); err != nil && err != bolt.ErrBucketNotFound {
			return err
		}
		b, err := tx.CreateBucket([]byte(dataBucket))
		if err != nil {
			return err
		}
		for k, v := range data {
			if err := b.Put([]byte(k), v); err != nil {
				return err
			}
		}
		return nil
	})
}

// get reads a single key from the local FSM state.
func (f *fsm) get(key string) (*physical.Entry, error) {
	var val []byte
	if err := f.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(dataBucket))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(key))
		if v != nil {
			val = make([]byte, len(v))
			copy(val, v)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if val == nil {
		return nil, nil
	}
	return &physical.Entry{Key: key, Value: val}, nil
}

// list returns keys whose storage key has the given prefix.
func (f *fsm) list(prefix string) ([]string, error) {
	var keys []string
	if err := f.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(dataBucket))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		pre := []byte(prefix)
		for k, _ := c.Seek(pre); k != nil && strings.HasPrefix(string(k), prefix); k, _ = c.Next() {
			kc := make([]byte, len(k))
			copy(kc, k)
			keys = append(keys, string(kc))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return keys, nil
}

// fsmSnapshot is a point-in-time copy of the FSM state used for Raft snapshotting.
type fsmSnapshot struct {
	data map[string][]byte
}

func (s *fsmSnapshot) Persist(sink hraft.SnapshotSink) error {
	if err := json.NewEncoder(sink).Encode(s.data); err != nil {
		_ = sink.Cancel()
		return fmt.Errorf("encode snapshot: %w", err)
	}
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}
