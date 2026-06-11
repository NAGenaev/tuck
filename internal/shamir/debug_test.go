package shamir

import "testing"

// TestGFTables verifies that the exp/log lookup tables are correctly
// constructed: all 255 non-zero elements appear in expTable, logTable[1]=0
// (identity), and gfMul(a,1)=a for representative values.
func TestGFTables(t *testing.T) {
	// logTable[1] must be 0 because g^0 = 1.
	if logTable[1] != 0 {
		t.Errorf("logTable[1] = %d, want 0", logTable[1])
	}
	// expTable[0] must be 1 (g^0 = 1).
	if expTable[0] != 1 {
		t.Errorf("expTable[0] = 0x%02x, want 0x01", expTable[0])
	}

	// Every non-zero element must appear exactly once in expTable[0..254].
	seen := make(map[byte]int)
	for i := 0; i < 255; i++ {
		seen[expTable[i]]++
	}
	for v := byte(1); v != 0; v++ {
		if seen[v] != 1 {
			t.Errorf("expTable coverage: element 0x%02x appears %d times (want 1)", v, seen[v])
		}
	}

	// gfMul(a, 1) == a for all non-zero a.
	for _, a := range []byte{0x01, 0x53, 0xab, 0xcd, 0xff} {
		if got := gfMul(a, 1); got != a {
			t.Errorf("gfMul(0x%02x, 1) = 0x%02x, want 0x%02x", a, got, a)
		}
	}
	// gfMul(a, 0) == 0.
	for _, a := range []byte{0x01, 0xab} {
		if got := gfMul(a, 0); got != 0 {
			t.Errorf("gfMul(0x%02x, 0) = 0x%02x, want 0x00", a, got)
		}
	}
}

// TestEvalPolyIdentity checks that evalPoly([s], x) == s for any x (degree-0
// polynomial is always the constant s).
func TestEvalPolyIdentity(t *testing.T) {
	for _, s := range []byte{0x00, 0x01, 0xab, 0xff} {
		for _, x := range []byte{0x01, 0x05, 0xff} {
			if got := evalPoly([]byte{s}, x); got != s {
				t.Errorf("evalPoly([0x%02x], 0x%02x) = 0x%02x, want 0x%02x", s, x, got, s)
			}
		}
	}
}
