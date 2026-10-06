package kvv2

import (
	"context"
	"strings"
	"testing"
)

func TestStats(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()

	for _, p := range []string{"a", "b/c", "b/d"} {
		if _, err := s.Write(ctx, p, []byte("v1"), nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Write(ctx, "a", []byte("v2"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write(ctx, "a", []byte("v3"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SoftDelete(ctx, "a", []int{1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Destroy(ctx, "a", []int{2}); err != nil {
		t.Fatal(err)
	}

	st, err := s.Stats(ctx, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Secrets != 3 || st.Versions != 5 || st.DeletedVersions != 1 || st.DestroyedVersions != 1 {
		t.Fatalf("stats = %+v", st)
	}
	if st.LastWriteAt.IsZero() || st.Truncated {
		t.Fatalf("stats = %+v", st)
	}

	n, err := s.CountSecrets(ctx)
	if err != nil || n != 3 {
		t.Fatalf("count = %d, %v", n, err)
	}

	// Permission filter: only the b/ subtree is readable; the secret count
	// stays exact but "a" contributes no version figures.
	only := func(p string) bool { return strings.HasPrefix(p, "b/") }
	st, err = s.Stats(ctx, 0, only)
	if err != nil {
		t.Fatal(err)
	}
	if st.Secrets != 3 || st.Hidden != 1 || st.Versions != 2 || st.DeletedVersions != 0 {
		t.Fatalf("filtered stats = %+v", st)
	}

	// Scan limit: the secret count stays exact, versions are partial.
	st, err = s.Stats(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Secrets != 3 || !st.Truncated {
		t.Fatalf("limited stats = %+v", st)
	}
}

func TestStatsEmpty(t *testing.T) {
	st, err := New(newMem()).Stats(context.Background(), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Secrets != 0 || st.Versions != 0 || !st.LastWriteAt.IsZero() {
		t.Fatalf("stats = %+v", st)
	}
}
