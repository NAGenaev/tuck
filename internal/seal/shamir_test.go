package seal_test

import (
	"path/filepath"
	"testing"

	"github.com/NAGenaev/tuck/internal/seal"
)

func newTestShamirSeal(t *testing.T, n, k int) *seal.ShamirSeal {
	t.Helper()
	s, err := seal.NewShamir(filepath.Join(t.TempDir(), "shamir.json"), n, k)
	if err != nil {
		t.Fatalf("NewShamir(%d,%d): %v", n, k, err)
	}
	return s
}

// TestShamirSeal_InitProducesShares verifies that Init returns N shares and a
// non-nil root key.
func TestShamirSeal_InitProducesShares(t *testing.T) {
	s := newTestShamirSeal(t, 5, 3)
	result, err := s.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(result.RootKey) != 32 {
		t.Errorf("RootKey length = %d, want 32", len(result.RootKey))
	}
	if len(result.Shares) != 5 {
		t.Errorf("Shares count = %d, want 5", len(result.Shares))
	}
	for i, sh := range result.Shares {
		if sh == "" {
			t.Errorf("share[%d] is empty", i)
		}
	}
}

// TestShamirSeal_UnsealReturnsErrNeedsShards ensures that Unseal() returns the
// sentinel error (never the root key).
func TestShamirSeal_UnsealReturnsErrNeedsShards(t *testing.T) {
	s := newTestShamirSeal(t, 3, 2)
	if _, err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	_, err := s.Unseal()
	if err != seal.ErrNeedsShards {
		t.Errorf("Unseal() error = %v, want ErrNeedsShards", err)
	}
}

// TestShamirSeal_AcceptShard_ProgressAndComplete tests the full unseal cycle.
func TestShamirSeal_AcceptShard_ProgressAndComplete(t *testing.T) {
	s := newTestShamirSeal(t, 5, 3)
	result, err := s.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	shares := result.Shares

	req, recv := s.ShardsProgress()
	if req != 3 || recv != 0 {
		t.Errorf("ShardsProgress() = (%d,%d), want (3,0)", req, recv)
	}

	// Supply first two shards — should not complete yet.
	for i := 0; i < 2; i++ {
		complete, key, err := s.AcceptShard(shares[i])
		if err != nil {
			t.Fatalf("AcceptShard[%d]: %v", i, err)
		}
		if complete {
			t.Fatalf("AcceptShard[%d]: complete=true after only %d shards", i, i+1)
		}
		if key != nil {
			t.Fatalf("AcceptShard[%d]: got non-nil key before threshold", i)
		}
	}

	req, recv = s.ShardsProgress()
	if req != 3 || recv != 2 {
		t.Errorf("ShardsProgress() = (%d,%d), want (3,2)", req, recv)
	}

	// Third shard — should complete.
	complete, recoveredKey, err := s.AcceptShard(shares[2])
	if err != nil {
		t.Fatalf("AcceptShard[2]: %v", err)
	}
	if !complete {
		t.Fatal("AcceptShard[2]: complete=false, want true")
	}
	if len(recoveredKey) != 32 {
		t.Errorf("recovered key length = %d, want 32", len(recoveredKey))
	}

	// The recovered key must match the original root key.
	for i, b := range result.RootKey {
		if recoveredKey[i] != b {
			t.Errorf("recovered key byte[%d] = %02x, want %02x", i, recoveredKey[i], b)
			break
		}
	}
}

// TestShamirSeal_AcceptShard_DuplicateRejected ensures duplicate x-coordinates
// are rejected.
func TestShamirSeal_AcceptShard_DuplicateRejected(t *testing.T) {
	s := newTestShamirSeal(t, 3, 2)
	result, _ := s.Init()

	s.AcceptShard(result.Shares[0]) //nolint:errcheck
	_, _, err := s.AcceptShard(result.Shares[0])
	if err == nil {
		t.Error("expected error for duplicate shard, got nil")
	}
}

// TestShamirSeal_AcceptShard_AfterComplete errors once threshold is met.
func TestShamirSeal_AcceptShard_AfterComplete(t *testing.T) {
	s := newTestShamirSeal(t, 2, 2)
	result, _ := s.Init()

	s.AcceptShard(result.Shares[0]) //nolint:errcheck
	s.AcceptShard(result.Shares[1]) //nolint:errcheck

	_, _, err := s.AcceptShard(result.Shares[0])
	if err != seal.ErrAlreadyUnsealed {
		t.Errorf("expected ErrAlreadyUnsealed, got %v", err)
	}
}

// TestShamirSeal_InvalidN_K checks that NewShamir rejects invalid n/k combos.
func TestShamirSeal_InvalidN_K(t *testing.T) {
	dir := t.TempDir()
	cases := [][2]int{
		{1, 2}, // k > n
		{3, 1}, // k < 2
		{256, 2}, // n > 255
	}
	for _, c := range cases {
		if _, err := seal.NewShamir(filepath.Join(dir, "cfg.json"), c[0], c[1]); err == nil {
			t.Errorf("NewShamir(n=%d,k=%d): expected error, got nil", c[0], c[1])
		}
	}
}

// TestShamirSeal_FromConfig verifies NewShamirFromConfig round-trips N/K.
func TestShamirSeal_FromConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "shamir.json")

	s, err := seal.NewShamir(cfgPath, 5, 3)
	if err != nil {
		t.Fatalf("NewShamir: %v", err)
	}
	if _, err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	s2, err := seal.NewShamirFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("NewShamirFromConfig: %v", err)
	}
	req, _ := s2.ShardsProgress()
	if req != 3 {
		t.Errorf("loaded seal threshold = %d, want 3", req)
	}
}
