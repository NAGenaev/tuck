package shamir

import (
	"testing"
)

// FuzzCombine tests that Combine never panics on arbitrary byte slices.
// It does not check correctness — only crash-safety of the parser.
func FuzzCombine(f *testing.F) {
	// Seed with valid shares generated from a known secret.
	secret := []byte("super-secret-root-key-32bytes!!!")
	shares, err := Split(secret, 5, 3)
	if err == nil {
		for _, s := range shares {
			f.Add(s)
		}
	}
	// Seed with boundary inputs.
	f.Add([]byte{0x01, 0xff})
	f.Add([]byte{0x00, 0x00})
	f.Add([]byte{})
	f.Add(make([]byte, 34))

	f.Fuzz(func(t *testing.T, share []byte) {
		// Must not panic regardless of input.
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Combine panicked on input %x: %v", share, r)
			}
		}()
		_, _ = Combine([][]byte{share})
	})
}

// FuzzSplitCombineRoundtrip tests that Split followed by Combine recovers the
// original secret for any valid (non-empty, ≤255 byte) secret.
func FuzzSplitCombineRoundtrip(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte{0x00, 0x01, 0xff})
	f.Add(make([]byte, 32))

	f.Fuzz(func(t *testing.T, secret []byte) {
		if len(secret) == 0 {
			return
		}
		shares, err := Split(secret, 3, 2)
		if err != nil {
			return // invalid input for Split — skip
		}
		got, err := Combine(shares[:2])
		if err != nil {
			t.Errorf("Combine failed: %v", err)
			return
		}
		if string(got) != string(secret) {
			t.Errorf("roundtrip mismatch: got %x, want %x", got, secret)
		}
	})
}
