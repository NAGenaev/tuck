package barrier

import (
	"context"
	"sort"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

// TestViewListRelative guards that View.List returns keys relative to the
// view, like Get/Put/Delete take them, so stores behave the same in a
// namespace view as in the root.
func TestViewListRelative(t *testing.T) {
	ctx := context.Background()
	b := New(physical.NewInMem())
	rootKey := randKey(t)
	if err := b.Initialize(ctx, rootKey); err != nil {
		t.Fatal(err)
	}
	if err := b.Unseal(ctx, rootKey); err != nil {
		t.Fatal(err)
	}
	v := b.View("ns/team/")
	for _, k := range []string{"kv2/meta/a", "kv2/meta/b/c"} {
		if err := v.Put(ctx, &physical.Entry{Key: k, Value: []byte("x")}); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Put(ctx, &physical.Entry{Key: "kv2/meta/root", Value: []byte("x")}); err != nil {
		t.Fatal(err)
	}

	got, err := v.List(ctx, "kv2/meta/")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"kv2/meta/a", "kv2/meta/b/c"}
	if len(got) != len(want) {
		t.Fatalf("view list = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("view list = %v, want %v", got, want)
		}
	}

	// The root view is unchanged: it lists absolute keys under the prefix.
	rootGot, err := b.View("").List(ctx, "kv2/meta/")
	if err != nil {
		t.Fatal(err)
	}
	if len(rootGot) != 1 || rootGot[0] != "kv2/meta/root" {
		t.Fatalf("root list = %v", rootGot)
	}
}
