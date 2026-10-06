package token

import (
	"context"
	"testing"
	"time"
)

func TestCountByNamespace(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	put := func(ns string, ttl time.Duration) {
		t.Helper()
		tok, err := Generate("c", []string{"default"}, ttl)
		if err != nil {
			t.Fatal(err)
		}
		tok.Namespace = ns
		if err := store.Put(ctx, tok); err != nil {
			t.Fatal(err)
		}
	}
	put("", 0)
	put("root", 0)
	put("team-a", time.Hour)
	put("team-a", 0)
	put("team-b", 0)
	// Expired tokens are not counted.
	tok, _ := Generate("old", nil, time.Hour)
	tok.Namespace = "team-a"
	tok.ExpiresAt = time.Now().Add(-time.Minute)
	if err := store.Put(ctx, tok); err != nil {
		t.Fatal(err)
	}

	counts, trunc, err := store.CountByNamespace(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if trunc || counts[RootKey] != 2 || counts["team-a"] != 2 || counts["team-b"] != 1 {
		t.Fatalf("counts = %v truncated=%v", counts, trunc)
	}

	_, trunc, err = store.CountByNamespace(ctx, 2)
	if err != nil || !trunc {
		t.Fatalf("limit 2: truncated=%v err=%v", trunc, err)
	}
}
