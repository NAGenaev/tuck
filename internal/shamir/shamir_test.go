package shamir_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/NAGenaev/tuck/internal/shamir"
)

func TestSplitCombine_2of3(t *testing.T) {
	secret := []byte("hello world test")
	shares, err := shamir.Split(secret, 3, 2)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(shares) != 3 {
		t.Fatalf("expected 3 shares, got %d", len(shares))
	}

	// Try all pairs.
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			got, err := shamir.Combine([][]byte{shares[i], shares[j]})
			if err != nil {
				t.Errorf("Combine([%d,%d]): %v", i, j, err)
				continue
			}
			if !bytes.Equal(got, secret) {
				t.Errorf("Combine([%d,%d]) = %q, want %q", i, j, got, secret)
			}
		}
	}
}

func TestSplitCombine_3of5(t *testing.T) {
	secret := []byte("another secret payload")
	shares, err := shamir.Split(secret, 5, 3)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(shares) != 5 {
		t.Fatalf("expected 5 shares, got %d", len(shares))
	}

	// Test several subsets of size 3.
	subsets := [][3]int{{0, 1, 2}, {0, 2, 4}, {1, 3, 4}, {2, 3, 4}}
	for _, idx := range subsets {
		got, err := shamir.Combine([][]byte{shares[idx[0]], shares[idx[1]], shares[idx[2]]})
		if err != nil {
			t.Errorf("Combine(%v): %v", idx, err)
			continue
		}
		if !bytes.Equal(got, secret) {
			t.Errorf("Combine(%v) = %q, want %q", idx, got, secret)
		}
	}
}

func TestSplitCombine_32ByteRootKey(t *testing.T) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	shares, err := shamir.Split(key, 5, 3)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	got, err := shamir.Combine(shares[:3])
	if err != nil {
		t.Fatalf("Combine: %v", err)
	}
	if !bytes.Equal(got, key) {
		t.Errorf("round-trip mismatch for 32-byte root key")
	}
}

func TestCombine_OrderDoesNotMatter(t *testing.T) {
	secret := []byte("order should not matter")
	shares, err := shamir.Split(secret, 4, 3)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	// Reverse order.
	reversed := [][]byte{shares[2], shares[1], shares[0]}
	got, err := shamir.Combine(reversed)
	if err != nil {
		t.Fatalf("Combine reversed: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Errorf("Combine reversed = %q, want %q", got, secret)
	}
}

func TestSplit_InvalidArgs(t *testing.T) {
	secret := []byte("x")
	cases := []struct {
		n, k int
	}{
		{0, 1},
		{3, 0},
		{2, 3},
		{256, 2},
	}
	for _, c := range cases {
		if _, err := shamir.Split(secret, c.n, c.k); err == nil {
			t.Errorf("Split(n=%d, k=%d) expected error, got nil", c.n, c.k)
		}
	}
}

func TestSplit_EmptySecret(t *testing.T) {
	if _, err := shamir.Split([]byte{}, 3, 2); err == nil {
		t.Error("expected error for empty secret")
	}
}

func TestCombine_DuplicateShares(t *testing.T) {
	secret := []byte("dup test")
	shares, err := shamir.Split(secret, 3, 2)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	// Pass the same share twice.
	if _, err := shamir.Combine([][]byte{shares[0], shares[0]}); err == nil {
		t.Error("expected error for duplicate x-coordinates")
	}
}

func TestCombine_TooFewBytesShare(t *testing.T) {
	if _, err := shamir.Combine([][]byte{{0x01}}); err == nil {
		t.Error("expected error for share with only 1 byte")
	}
}

func TestCombine_NoShares(t *testing.T) {
	if _, err := shamir.Combine(nil); err == nil {
		t.Error("expected error for nil shares")
	}
}

func TestSplitCombine_1of1(t *testing.T) {
	secret := []byte{0xAB, 0xCD}
	shares, err := shamir.Split(secret, 1, 1)
	if err != nil {
		t.Fatalf("Split(1,1): %v", err)
	}
	got, err := shamir.Combine(shares)
	if err != nil {
		t.Fatalf("Combine: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Errorf("1-of-1 round-trip failed: got %x, want %x", got, secret)
	}
}

func TestSplitCombine_AllShares(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		t.Fatal(err)
	}
	shares, err := shamir.Split(secret, 5, 3)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	// Combining all 5 should also work.
	got, err := shamir.Combine(shares)
	if err != nil {
		t.Fatalf("Combine all: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Error("all-shares round-trip failed")
	}
}
