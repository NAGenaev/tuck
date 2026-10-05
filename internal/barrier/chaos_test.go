package barrier

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

var errDiskFull = errors.New("chaos: no space left on device")

// fullDisk fails every write once its budget of successful writes is spent.
type fullDisk struct {
	physical.Backend
	budget atomic.Int64 // < 0 = unlimited
}

func (f *fullDisk) Put(ctx context.Context, e *physical.Entry) error {
	if n := f.budget.Load(); n >= 0 {
		if n == 0 {
			return errDiskFull
		}
		f.budget.Add(-1)
	}
	return f.Backend.Put(ctx, e)
}

// A disk that fills up in the middle of a data-key rotation must not lose or
// corrupt anything: every entry stays readable, also after a restart, and the
// rotation can be completed once space is available.
func TestChaosDiskFullDuringRotation(t *testing.T) {
	forEachSuite(t, func(t *testing.T, name string) {
		ctx := context.Background()
		fd := &fullDisk{Backend: physical.NewInMem()}
		fd.budget.Store(-1)
		root := randKey(t)
		b := New(fd)
		if err := b.InitializeWithSuite(ctx, root, name); err != nil {
			t.Fatal(err)
		}
		if err := b.Unseal(ctx, root); err != nil {
			t.Fatal(err)
		}
		const n = 30
		for i := 0; i < n; i++ {
			if err := b.Put(ctx, &physical.Entry{Key: fmt.Sprintf("secret/k%d", i), Value: []byte(fmt.Sprintf("v%d", i))}); err != nil {
				t.Fatal(err)
			}
		}

		// keyring write + 10 entries, then the disk is full.
		fd.budget.Store(11)
		if _, _, err := b.RotateDataKey(ctx, true); !errors.Is(err, errDiskFull) {
			t.Fatalf("rotation on a full disk: %v", err)
		}
		check := func(label string, bb *Barrier) {
			t.Helper()
			for i := 0; i < n; i++ {
				e, err := bb.Get(ctx, fmt.Sprintf("secret/k%d", i))
				if err != nil || e == nil || string(e.Value) != fmt.Sprintf("v%d", i) {
					t.Fatalf("%s: k%d: %v %v", label, i, e, err)
				}
			}
		}
		check("same process", b)

		fresh := New(fd)
		if err := fresh.Unseal(ctx, root); err != nil {
			t.Fatalf("unseal after failed rotation: %v", err)
		}
		check("after restart", fresh)

		// Writes fail cleanly while the disk is full and do not damage old data.
		if err := b.Put(ctx, &physical.Entry{Key: "secret/new", Value: []byte("x")}); !errors.Is(err, errDiskFull) {
			t.Fatalf("put on a full disk: %v", err)
		}
		check("after failed put", b)

		// Space is freed: finish the rotation.
		fd.budget.Store(-1)
		if _, _, err := fresh.RotateDataKey(ctx, true); err != nil {
			t.Fatalf("rotation after freeing space: %v", err)
		}
		check("after completed rotation", fresh)
		again := New(fd)
		if err := again.Unseal(ctx, root); err != nil {
			t.Fatal(err)
		}
		check("restart after completed rotation", again)
	})
}
