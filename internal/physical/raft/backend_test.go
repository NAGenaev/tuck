package raft

import (
	"context"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

// openSingle opens a bootstrapped single-node Raft cluster for testing.
func openSingle(t *testing.T) *Backend {
	t.Helper()
	dir := t.TempDir()
	b, err := Open(Config{
		NodeID:             "node1",
		BindAddr:           "127.0.0.1:0",
		DataDir:            dir,
		Bootstrap:          true,
		HeartbeatTimeout:   500 * time.Millisecond,
		ElectionTimeout:    500 * time.Millisecond,
		LeaderLeaseTimeout: 250 * time.Millisecond,
		SnapshotInterval:   5 * time.Second,
		SnapshotThreshold:  1024,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { b.Close() })
	waitLeader(t, b)
	return b
}

// waitLeader spins until this node becomes leader or times out.
func waitLeader(t *testing.T, b *Backend) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b.IsLeader() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for Raft leader election")
}

func TestRaftPutGet(t *testing.T) {
	b := openSingle(t)
	ctx := context.Background()

	if err := b.Put(ctx, entry("hello", "world")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	e, err := b.Get(ctx, "hello")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e == nil {
		t.Fatal("expected entry, got nil")
	}
	if string(e.Value) != "world" {
		t.Fatalf("value mismatch: got %q", e.Value)
	}
}

func TestRaftGetMissing(t *testing.T) {
	b := openSingle(t)
	e, err := b.Get(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e != nil {
		t.Fatalf("expected nil for missing key, got %v", e)
	}
}

func TestRaftDelete(t *testing.T) {
	b := openSingle(t)
	ctx := context.Background()

	if err := b.Put(ctx, entry("key", "val")); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete(ctx, "key"); err != nil {
		t.Fatal(err)
	}
	e, err := b.Get(ctx, "key")
	if err != nil {
		t.Fatal(err)
	}
	if e != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestRaftList(t *testing.T) {
	b := openSingle(t)
	ctx := context.Background()

	for _, k := range []string{"a/1", "a/2", "b/1"} {
		if err := b.Put(ctx, entry(k, "v")); err != nil {
			t.Fatal(err)
		}
	}
	keys, err := b.List(ctx, "a/")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys under a/, got %d: %v", len(keys), keys)
	}
}

func TestRaftOverwrite(t *testing.T) {
	b := openSingle(t)
	ctx := context.Background()

	if err := b.Put(ctx, entry("k", "v1")); err != nil {
		t.Fatal(err)
	}
	if err := b.Put(ctx, entry("k", "v2")); err != nil {
		t.Fatal(err)
	}
	e, err := b.Get(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if string(e.Value) != "v2" {
		t.Fatalf("expected v2, got %q", e.Value)
	}
}

func TestRaftStatus(t *testing.T) {
	b := openSingle(t)
	st := b.Status()
	if !st.IsLeader {
		t.Fatal("single-node cluster should be leader")
	}
	if len(st.Servers) == 0 {
		t.Fatal("expected at least one server in status")
	}
}

func TestRaftSnapshot(t *testing.T) {
	b := openSingle(t)
	ctx := context.Background()

	if err := b.Put(ctx, entry("snap/key", "snap/val")); err != nil {
		t.Fatal(err)
	}

	var buf bytes
	if err := b.Snapshot(ctx, &buf); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("snapshot output is empty")
	}
}

// bytes is a minimal io.Writer for testing.
type bytes struct{ data []byte }

func (b *bytes) Write(p []byte) (int, error) { b.data = append(b.data, p...); return len(p), nil }
func (b *bytes) Len() int                    { return len(b.data) }

func entry(key, val string) *physical.Entry {
	return &physical.Entry{Key: key, Value: []byte(val)}
}
