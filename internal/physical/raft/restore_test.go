package raft

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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
