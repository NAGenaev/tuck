package core_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

type flakySeal struct {
	*seal.Dev
	down atomic.Bool
}

func (f *flakySeal) Unseal() ([]byte, error) {
	if f.down.Load() {
		return nil, errors.New("transit unreachable")
	}
	return f.Dev.Unseal()
}

func TestStartAutoUnsealUnavailableStaysSealedAndRecovers(t *testing.T) {
	ctx := context.Background()
	be := physical.NewInMem()
	s := &flakySeal{Dev: seal.NewDev(filepath.Join(t.TempDir(), "rootkey"))}

	first := core.New(be, s)
	if _, err := first.Start(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	first.Seal()

	s.down.Store(true)
	c := core.New(be, s)
	if _, err := c.Start(ctx); !errors.Is(err, core.ErrSealUnavailable) {
		t.Fatalf("Start with seal down: want ErrSealUnavailable, got %v", err)
	}
	if !c.Sealed() {
		t.Fatal("core must stay sealed while the seal is unavailable")
	}
	if ok, err := c.UnsealShard(ctx, ""); err == nil || ok {
		t.Fatalf("unseal while seal is down must fail, got ok=%v err=%v", ok, err)
	}

	s.down.Store(false)
	if ok, err := c.UnsealShard(ctx, ""); err != nil || !ok {
		t.Fatalf("unseal after seal recovered: ok=%v err=%v", ok, err)
	}
	if c.Sealed() {
		t.Fatal("core must be unsealed after recovery")
	}
}
