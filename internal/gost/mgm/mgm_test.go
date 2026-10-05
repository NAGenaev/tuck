package mgm

import (
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/NAGenaev/tuck/internal/gost/kuznyechik"
)

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newAEAD(t testing.TB, key []byte) cipher.AEAD {
	t.Helper()
	b, err := kuznyechik.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(b)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

type vec struct{ key, icn, ad, pt, ct, tag string }

var vectors = []vec{
	{ // RFC 9058 A.1.1
		key: "8899AABBCCDDEEFF0011223344556677FEDCBA98765432100123456789ABCDEF",
		icn: "1122334455667700FFEEDDCCBBAA9988",
		ad:  "0202020202020202 0101010101010101 0404040404040404 0303030303030303 EA05050505050505 05",
		pt:  "1122334455667700FFEEDDCCBBAA9988 00112233445566778899AABBCCEEFF0A 112233445566778899AABBCCEEFF0A00 2233445566778899AABBCCEEFF0A0011 AABBCC",
		ct:  "A9757B8147956E9055B8A33DE89F42FC 8075D2212BF9FD5BD3F7069AADC16B39 497AB15915A6BA85936B5D0EA9F6851C C60C14D4D3F883D0AB94420695C76DEB 2C7552",
		tag: "CF5D656F40C34F5C46E8BB0E29FCDB4C",
	},
	{ // RFC 9058 A.1.2
		key: "99AABBCCDDEEFF0011223344556677FE DCBA98765432100123456789ABCDEF88",
		icn: "1122334455667700FFEEDDCCBBAA9988",
		ad:  "01010101010101010101010101010101",
		pt:  "",
		ct:  "",
		tag: "7901E9EA2085CD247ED249695F9F8A85",
	},
}

func TestRFC9058(t *testing.T) {
	for i, v := range vectors {
		a := newAEAD(t, unhex(t, v.key))
		nonce, ad, pt := unhex(t, v.icn), unhex(t, v.ad), unhex(t, v.pt)
		want := append(unhex(t, v.ct), unhex(t, v.tag)...)
		got := a.Seal(nil, nonce, pt, ad)
		if !bytes.Equal(got, want) {
			t.Fatalf("vector %d: Seal\n got %x\nwant %x", i, got, want)
		}
		back, err := a.Open(nil, nonce, got, ad)
		if err != nil || !bytes.Equal(back, pt) {
			t.Fatalf("vector %d: Open: %v %x", i, err, back)
		}
	}
}

func TestTamper(t *testing.T) {
	v := vectors[0]
	a := newAEAD(t, unhex(t, v.key))
	nonce, ad, pt := unhex(t, v.icn), unhex(t, v.ad), unhex(t, v.pt)
	sealed := a.Seal(nil, nonce, pt, ad)
	for i := range sealed {
		bad := append([]byte(nil), sealed...)
		bad[i] ^= 1
		if _, err := a.Open(nil, nonce, bad, ad); err == nil {
			t.Fatalf("tampered byte %d accepted", i)
		}
	}
	badAD := append([]byte(nil), ad...)
	badAD[0] ^= 1
	if _, err := a.Open(nil, nonce, sealed, badAD); err == nil {
		t.Fatal("tampered AD accepted")
	}
	badN := append([]byte(nil), nonce...)
	badN[15] ^= 1
	if _, err := a.Open(nil, badN, sealed, ad); err == nil {
		t.Fatal("wrong nonce accepted")
	}
	if _, err := a.Open(nil, nonce, sealed[:5], ad); err == nil {
		t.Fatal("short ciphertext accepted")
	}
	if _, err := a.Open(nil, nonce, nil, ad); err == nil {
		t.Fatal("empty ciphertext accepted")
	}
	hi := append([]byte(nil), nonce...)
	hi[0] |= 0x80
	if _, err := a.Open(nil, hi, sealed, ad); err == nil {
		t.Fatal("nonce with MSB set accepted")
	}
}

func TestSizesRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	a := newAEAD(t, key)
	for _, n := range []int{0, 1, 15, 16, 17, 31, 32, 33, 255, 256, 1000} {
		for _, m := range []int{0, 1, 16, 17, 33} {
			pt, ad := make([]byte, n), make([]byte, m)
			rand.Read(pt)
			rand.Read(ad)
			nonce := make([]byte, 16)
			rand.Read(nonce)
			nonce[0] &= 0x7f
			prefix := []byte("prefix")
			sealed := a.Seal(append([]byte(nil), prefix...), nonce, pt, ad)
			if !bytes.HasPrefix(sealed, prefix) || len(sealed) != len(prefix)+n+TagSize {
				t.Fatalf("n=%d m=%d: bad Seal layout", n, m)
			}
			back, err := a.Open(nil, nonce, sealed[len(prefix):], ad)
			if err != nil || !bytes.Equal(back, pt) {
				t.Fatalf("n=%d m=%d: %v", n, m, err)
			}
		}
	}
}

func TestInPlace(t *testing.T) {
	a := newAEAD(t, make([]byte, 32))
	nonce := make([]byte, 16)
	pt := []byte("in place encryption of a message")
	buf := append([]byte(nil), pt...)
	sealed := a.Seal(buf[:0], nonce, buf, nil)
	back, err := a.Open(sealed[:0], nonce, sealed, nil)
	if err != nil || !bytes.Equal(back, pt) {
		t.Fatalf("in-place: %v %q", err, back)
	}
}

func TestBadNoncePanics(t *testing.T) {
	a := newAEAD(t, make([]byte, 32))
	for _, n := range [][]byte{make([]byte, 12), append([]byte{0x80}, make([]byte, 15)...)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Seal accepted nonce %x", n)
				}
			}()
			a.Seal(nil, n, nil, nil)
		}()
	}
}

func BenchmarkSeal1K(b *testing.B) {
	a := newAEAD(b, make([]byte, 32))
	nonce := make([]byte, 16)
	pt := make([]byte, 1024)
	b.SetBytes(1024)
	for i := 0; i < b.N; i++ {
		a.Seal(nil, nonce, pt, nil)
	}
}

func BenchmarkOpen1K(b *testing.B) {
	a := newAEAD(b, make([]byte, 32))
	nonce := make([]byte, 16)
	sealed := a.Seal(nil, nonce, make([]byte, 1024), nil)
	b.SetBytes(1024)
	for i := 0; i < b.N; i++ {
		if _, err := a.Open(nil, nonce, sealed, nil); err != nil {
			b.Fatal(err)
		}
	}
}
