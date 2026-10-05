package core

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

func TestStartWithGOSTBarrierCipher(t *testing.T) {
	ctx := context.Background()
	be := physical.NewInMem()
	c := New(be, seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	if err := c.SetBarrierCipher("gost-kuznyechik-mgm"); err != nil {
		t.Fatal(err)
	}
	res, err := c.Start(ctx)
	if err != nil || res == nil || res.RootToken == nil {
		t.Fatalf("start: %v %v", res, err)
	}
	if got := c.BarrierCipher(); got != "gost-kuznyechik-mgm" {
		t.Fatalf("cipher = %q", got)
	}
	if err := New(be, seal.NewDev(filepath.Join(t.TempDir(), "rootkey"))).SetBarrierCipher("rot13"); err == nil {
		t.Fatal("unknown cipher must be rejected")
	}
}
