package mount

import (
	"context"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

func newTestConfigStore(t *testing.T) *ConfigStore {
	t.Helper()
	return NewConfigStore(physical.NewInMem())
}

func TestConfigStore_defaultEmpty(t *testing.T) {
	s := newTestConfigStore(t)
	cfg, err := s.Get(context.Background(), "secret/")
	if err != nil {
		t.Fatal(err)
	}
	// Zero Config is the default.
	if cfg.DefaultLeaseTTL != 0 || cfg.MaxLeaseTTL != 0 || cfg.ForceNoCache {
		t.Errorf("unexpected default config: %+v", cfg)
	}
}

func TestConfigStore_putAndGet(t *testing.T) {
	s := newTestConfigStore(t)
	ctx := context.Background()

	cfg := Config{
		DefaultLeaseTTL:        time.Hour,
		MaxLeaseTTL:            24 * time.Hour,
		ForceNoCache:           true,
		AllowedResponseHeaders: []string{"X-My-Header"},
		Description:            "test",
	}
	if err := s.Put(ctx, "secret/", cfg); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "secret/")
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultLeaseTTL != time.Hour {
		t.Errorf("DefaultLeaseTTL = %v, want 1h", got.DefaultLeaseTTL)
	}
	if got.MaxLeaseTTL != 24*time.Hour {
		t.Errorf("MaxLeaseTTL = %v, want 24h", got.MaxLeaseTTL)
	}
	if !got.ForceNoCache {
		t.Error("ForceNoCache must be true")
	}
	if len(got.AllowedResponseHeaders) != 1 || got.AllowedResponseHeaders[0] != "X-My-Header" {
		t.Errorf("AllowedResponseHeaders = %v", got.AllowedResponseHeaders)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt must be set by Put")
	}
}

func TestConfigStore_overwrite(t *testing.T) {
	s := newTestConfigStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "pki/", Config{DefaultLeaseTTL: time.Hour})
	_ = s.Put(ctx, "pki/", Config{DefaultLeaseTTL: 2 * time.Hour})

	got, _ := s.Get(ctx, "pki/")
	if got.DefaultLeaseTTL != 2*time.Hour {
		t.Errorf("DefaultLeaseTTL = %v, want 2h", got.DefaultLeaseTTL)
	}
}

func TestConfigStore_delete(t *testing.T) {
	s := newTestConfigStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "transit/", Config{DefaultLeaseTTL: time.Hour})
	if err := s.Delete(ctx, "transit/"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, "transit/")
	if got.DefaultLeaseTTL != 0 {
		t.Errorf("after delete, DefaultLeaseTTL = %v, want 0", got.DefaultLeaseTTL)
	}
}

func TestConfigStore_list(t *testing.T) {
	s := newTestConfigStore(t)
	ctx := context.Background()

	_ = s.Put(ctx, "secret/", Config{DefaultLeaseTTL: time.Hour})
	_ = s.Put(ctx, "pki/", Config{MaxLeaseTTL: 24 * time.Hour})

	all, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("list count = %d, want 2", len(all))
	}
}
