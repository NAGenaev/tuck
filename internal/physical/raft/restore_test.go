package raft

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	hraft "github.com/hashicorp/raft"
)

func TestRaftRestoreReplacesAllData(t *testing.T) {
	b := openSingle(t)
	ctx := context.Background()
	for k, v := range map[string]string{"core/a": "1", "core/b": "2", "kv/x": "old"} {
		if err := b.Put(ctx, entry(k, v)); err != nil {
			t.Fatal(err)
		}
	}
	var snap bytes
	if err := b.Snapshot(ctx, &snap); err != nil {
		t.Fatal(err)
	}

	// Drift away from the snapshot: change, delete and add.
	_ = b.Put(ctx, entry("kv/x", "new"))
	_ = b.Delete(ctx, "core/a")
	_ = b.Put(ctx, entry("kv/extra", "z"))

	hooked := make(chan struct{}, 2)
	b.OnRestore(func() { hooked <- struct{}{} })

	n, err := b.Restore(strings.NewReader(string(snap.data)), 1<<20)
	if err != nil || n != 3 {
		t.Fatalf("Restore: n=%d err=%v", n, err)
	}
	for k, want := range map[string]string{"core/a": "1", "core/b": "2", "kv/x": "old"} {
		e, err := b.Get(ctx, k)
		if err != nil || e == nil || string(e.Value) != want {
			t.Errorf("%s = %v, want %q", k, e, want)
		}
	}
	if e, _ := b.Get(ctx, "kv/extra"); e != nil {
		t.Error("an entry written after the snapshot survived the restore")
	}
	select {
	case <-hooked:
	case <-time.After(2 * time.Second):
		t.Error("OnRestore was not called")
	}
}

func TestRaftRestoreRejectsBadInput(t *testing.T) {
	b := openSingle(t)
	ctx := context.Background()
	_ = b.Put(ctx, entry("keep", "me"))

	if _, err := b.Restore(strings.NewReader(`not json`), 1<<20); err == nil {
		t.Error("garbage accepted")
	}
	if _, err := b.Restore(strings.NewReader(`{}`), 1<<20); !errors.Is(err, ErrEmptySnapshot) {
		t.Errorf("empty snapshot: %v", err)
	}
	if _, err := b.Restore(strings.NewReader(`{"a":"MQ=="}`), 4); err == nil {
		t.Error("oversize snapshot accepted")
	}
	if e, _ := b.Get(ctx, "keep"); e == nil || string(e.Value) != "me" {
		t.Error("a rejected restore changed the data")
	}
}

// The FSM applies a batch of committed entries in one transaction; entries
// stay in order and a bad entry affects only itself.
func TestFSMApplyBatch(t *testing.T) {
	f, err := newFSM(t.TempDir() + "/fsm.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.db.Close() }()
	log := func(op, k, v string) *hraft.Log {
		raw, _ := json.Marshal(command{Op: op, Key: k, Value: []byte(v)})
		return &hraft.Log{Data: raw}
	}
	res := f.ApplyBatch([]*hraft.Log{
		log("put", "a", "1"),
		log("put", "b", "2"),
		{Data: []byte("not json")},
		log("bogus", "x", ""),
		log("delete", "a", ""),
		log("put", "c", "3"),
	})
	if len(res) != 6 {
		t.Fatalf("results: %d", len(res))
	}
	for i, wantErr := range []bool{false, false, true, true, false, false} {
		if (res[i] != nil) != wantErr {
			t.Errorf("entry %d: result %v, want error=%v", i, res[i], wantErr)
		}
	}
	if e, _ := f.get("a"); e != nil {
		t.Error("a was deleted later in the batch")
	}
	for k, want := range map[string]string{"b": "2", "c": "3"} {
		if e, _ := f.get(k); e == nil || string(e.Value) != want {
			t.Errorf("%s = %v", k, e)
		}
	}
}
