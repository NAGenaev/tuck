package sysconfig

import (
	"context"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

// memBarrier is an in-memory barrier for testing.
type memBarrier struct{ entries map[string][]byte }

func newMem() *memBarrier { return &memBarrier{entries: map[string][]byte{}} }

func (m *memBarrier) Get(_ context.Context, key string) (*physical.Entry, error) {
	v, ok := m.entries[key]
	if !ok {
		return nil, nil
	}
	return &physical.Entry{Key: key, Value: v}, nil
}

func (m *memBarrier) Put(_ context.Context, e *physical.Entry) error {
	m.entries[e.Key] = e.Value
	return nil
}

func TestPutGet(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	cfg := Config{IPRateLimitRPS: 100, IPRateLimitBurst: 200, MaxBodyBytes: 1 << 20}
	if err := s.Put(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.IPRateLimitRPS != 100 || got.IPRateLimitBurst != 200 || got.MaxBodyBytes != 1<<20 {
		t.Fatalf("unexpected config: %+v", got)
	}
}

func TestGetEmpty(t *testing.T) {
	s := New(newMem())
	cfg, err := s.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg != (Config{}) {
		t.Fatalf("expected zero config, got %+v", cfg)
	}
}

func TestPutOverwrite(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	_ = s.Put(ctx, Config{IPRateLimitRPS: 10})
	_ = s.Put(ctx, Config{IPRateLimitRPS: 50, TokenRateLimitRPS: 25})

	got, err := s.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.IPRateLimitRPS != 50 || got.TokenRateLimitRPS != 25 {
		t.Fatalf("unexpected config after overwrite: %+v", got)
	}
}
