package replication

import (
	"context"
	"errors"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

func newTestWAL(t *testing.T) *WAL {
	t.Helper()
	return New(physical.NewInMem())
}

func TestSetMode_primary(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	if err := w.SetMode(ctx, ReplicaModePrimary, ""); err != nil {
		t.Fatal(err)
	}
	st, err := w.GetState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != ReplicaModePrimary {
		t.Errorf("mode = %q, want primary", st.Mode)
	}
}

func TestSetMode_secondary(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()

	if err := w.SetMode(ctx, ReplicaModeSecondary, "primary.example.com:8200"); err != nil {
		t.Fatal(err)
	}
	st, _ := w.GetState(ctx)
	if st.Mode != ReplicaModeSecondary {
		t.Errorf("mode = %q, want secondary", st.Mode)
	}
	if st.PrimaryAddr != "primary.example.com:8200" {
		t.Errorf("primary_addr = %q", st.PrimaryAddr)
	}
}

func TestAppend_requiresPrimary(t *testing.T) {
	w := newTestWAL(t)
	_, err := w.Append(context.Background(), "put", "secret/x", []byte("val"))
	if !errors.Is(err, ErrNotPrimary) {
		t.Fatalf("err = %v, want ErrNotPrimary", err)
	}
}

func TestAppend_sequential(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()
	_ = w.SetMode(ctx, ReplicaModePrimary, "")

	for i := 1; i <= 5; i++ {
		e, err := w.Append(ctx, "put", "key", []byte("v"))
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		if int(e.Sequence) != i {
			t.Errorf("entry %d has sequence %d", i, e.Sequence)
		}
	}
}

func TestReadFrom(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()
	_ = w.SetMode(ctx, ReplicaModePrimary, "")

	for i := 0; i < 5; i++ {
		_, _ = w.Append(ctx, "put", "k", nil)
	}

	// ReadFrom(0) → all 5 entries.
	all, err := w.ReadFrom(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("ReadFrom(0) = %d entries, want 5", len(all))
	}
	for i, e := range all {
		if e.Sequence != uint64(i+1) {
			t.Errorf("entry[%d].Sequence = %d, want %d", i, e.Sequence, i+1)
		}
	}

	// ReadFrom(3) → entries 4 and 5.
	partial, err := w.ReadFrom(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(partial) != 2 {
		t.Fatalf("ReadFrom(3) = %d entries, want 2", len(partial))
	}
	if partial[0].Sequence != 4 || partial[1].Sequence != 5 {
		t.Errorf("partial sequences = %d, %d, want 4, 5", partial[0].Sequence, partial[1].Sequence)
	}
}

func TestTrimBefore(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()
	_ = w.SetMode(ctx, ReplicaModePrimary, "")

	for i := 0; i < 5; i++ {
		_, _ = w.Append(ctx, "put", "k", nil)
	}

	// Trim entries < 4 → keep 4 and 5.
	if err := w.TrimBefore(ctx, 4); err != nil {
		t.Fatal(err)
	}
	remaining, _ := w.ReadFrom(ctx, 0)
	if len(remaining) != 2 {
		t.Fatalf("after trim: %d entries remain, want 2", len(remaining))
	}
	if remaining[0].Sequence != 4 {
		t.Errorf("first remaining sequence = %d, want 4", remaining[0].Sequence)
	}
}

func TestGetState_default(t *testing.T) {
	w := newTestWAL(t)
	st, err := w.GetState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != ReplicaModeDisabled {
		t.Errorf("default mode = %q, want disabled", st.Mode)
	}
}

func TestWalKey_ordering(t *testing.T) {
	k1 := walKey(1)
	k99 := walKey(99)
	k100 := walKey(100)
	if k1 >= k99 {
		t.Errorf("walKey(1)=%q should be < walKey(99)=%q", k1, k99)
	}
	if k99 >= k100 {
		t.Errorf("walKey(99)=%q should be < walKey(100)=%q", k99, k100)
	}
}

func TestAppend_deleteOperation(t *testing.T) {
	w := newTestWAL(t)
	ctx := context.Background()
	_ = w.SetMode(ctx, ReplicaModePrimary, "")

	e, err := w.Append(ctx, "delete", "secret/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if e.Operation != "delete" {
		t.Errorf("operation = %q, want delete", e.Operation)
	}
	if e.Value != nil {
		t.Error("delete entry must have nil value")
	}
}
