package kdf

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.Join(strings.Fields(s), ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const key = "00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f 10 11 12 13 14 15 16 17 18 19 1a 1b 1c 1d 1e 1f"

// RFC 7836 Appendix B, examples 1, 2 and 9.
func TestRFC7836(t *testing.T) {
	msg := unhex(t, "01 26 bd b8 78 00 af 21 43 41 45 65 63 78 01 00")
	want256 := unhex(t, "a1 aa 5f 7d e4 02 d7 b3 d3 23 f2 99 1c 8d 45 34 01 31 37 01 0a 83 75 4f d0 af 6d 7c d4 92 2e d9")
	if got := HMAC256(unhex(t, key), msg); !bytes.Equal(got, want256) {
		t.Errorf("HMAC256 = %x", got)
	}
	want512 := unhex(t, `a5 9b ab 22 ec ae 19 c6 5f bd e6 e5 f4 e9 f5 d8 54 9d 31 f0 37 f9 df 9b 90 55 00 e1 71 92 3a 77
		3d 5f 15 30 f2 ed 7e 96 4c b2 ee dc 29 e9 ad 2f 3a fe 93 b2 81 4f 79 f5 00 0f fc 03 66 c2 51 e6`)
	if got := HMAC512(unhex(t, key), msg); !bytes.Equal(got, want512) {
		t.Errorf("HMAC512 = %x", got)
	}
	got := KDF256(unhex(t, key), unhex(t, "26 bd b8 78"), unhex(t, "af 21 43 41 45 65 63 78"))
	if !bytes.Equal(got, want256) {
		t.Errorf("KDF256 = %x", got)
	}
}

func TestKDFSeparation(t *testing.T) {
	k := unhex(t, key)
	a := KDF256(k, []byte("a"), []byte("seed"))
	b := KDF256(k, []byte("b"), []byte("seed"))
	c := KDF256(k, []byte("a"), []byte("seed2"))
	if len(a) != 32 || bytes.Equal(a, b) || bytes.Equal(a, c) {
		t.Fatal("KDF output must depend on label and seed")
	}
}
