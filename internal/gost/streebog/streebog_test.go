package streebog

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

// The RFC prints vectors as big-endian numbers; the byte strings that are
// hashed and returned are the reverse of the printed hex.
func rev(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

const (
	m1 = "32313039383736353433323130393837 36353433323130393837363534333231 30393837363534333231303938373635 343332313039383736353433323130"
	m2 = "fbe2e5f0eee3c820fbeafaebef20fffb f0e1e0f0f520e0ed20e8ece0ebe5f0f2 f120fff0eeec20f120faf2fee5e2202c e8f6f3ede220e8e6eee1e8f0f2d1202c e8f0f2e5e220e5d1"

	h1x512 = "486f64c1917879417fef082b3381a4e2 11c324f074654c38823a7b76f830ad00 fa1fbae42b1285c0352f227524bc9ab1 6254288dd6863dccd5b9f54a1ad0541b"
	h1x256 = "00557be5e584fd52a449b16b0251d05d 27f94ab76cbaa6da890b59d8ef1e159d"
	h2x512 = "28fbc9bada033b1460642bdcddb90c3f b3e56c497ccd0f62b8a2ad4935e85f03 7613966de4ee00531ae60f3b5a47f8da e06915d5f2f194996fcabf2622e6881e"
	h2x256 = "508f7e553c06501d749a66fc28c6cac0 b005746d97537fa85d9e40904efed29d"
)

func TestRFC6986(t *testing.T) {
	cases := []struct {
		name  string
		msg   string
		new   func() interface{ Sum([]byte) []byte }
		want  string
		sizeB int
	}{
		{"M1-512", m1, func() interface{ Sum([]byte) []byte } { return New512() }, h1x512, 64},
		{"M1-256", m1, func() interface{ Sum([]byte) []byte } { return New256() }, h1x256, 32},
		{"M2-512", m2, func() interface{ Sum([]byte) []byte } { return New512() }, h2x512, 64},
		{"M2-256", m2, func() interface{ Sum([]byte) []byte } { return New256() }, h2x256, 32},
	}
	for _, c := range cases {
		msg := rev(t, c.msg)
		var got []byte
		if c.sizeB == 64 {
			s := Sum512(msg)
			got = s[:]
		} else {
			s := Sum256(msg)
			got = s[:]
		}
		if want := rev(t, c.want); !bytes.Equal(got, want) {
			t.Errorf("%s: got %x want %x", c.name, got, want)
		}
	}
}

// Well-known byte-order digests of "012345678901234567890123456789012345678901234567890123456789012".
func TestKnownDigest(t *testing.T) {
	msg := []byte("012345678901234567890123456789012345678901234567890123456789012")
	if !bytes.Equal(msg, rev(t, m1)) {
		t.Fatal("M1 reversal mismatch")
	}
	s := Sum256(msg)
	if hex.EncodeToString(s[:]) != "9d151eefd8590b89daa6ba6cb74af9275dd051026bb149a452fd84e5e57b5500" {
		t.Errorf("Streebog-256 = %x", s)
	}
}

func TestChunkedWrites(t *testing.T) {
	for _, size := range []int{0, 1, 63, 64, 65, 127, 128, 129, 200, 1000} {
		data := make([]byte, size)
		rand.Read(data)
		for _, mk := range []func() (h interface {
			Write([]byte) (int, error)
			Sum([]byte) []byte
			Reset()
		}){
			func() interface {
				Write([]byte) (int, error)
				Sum([]byte) []byte
				Reset()
			} {
				return New256()
			},
			func() interface {
				Write([]byte) (int, error)
				Sum([]byte) []byte
				Reset()
			} {
				return New512()
			},
		} {
			whole := mk()
			_, _ = whole.Write(data)
			want := whole.Sum(nil)
			for _, step := range []int{1, 7, 64, 100} {
				h := mk()
				for i := 0; i < len(data); i += step {
					end := i + step
					if end > len(data) {
						end = len(data)
					}
					_, _ = h.Write(data[i:end])
				}
				if got := h.Sum(nil); !bytes.Equal(got, want) {
					t.Fatalf("size %d step %d: chunked digest differs", size, step)
				}
				// Sum must not disturb the state, and Reset must restart it.
				if got := h.Sum(nil); !bytes.Equal(got, want) {
					t.Fatalf("size %d step %d: second Sum differs", size, step)
				}
				h.Reset()
				_, _ = h.Write(data)
				if got := h.Sum(nil); !bytes.Equal(got, want) {
					t.Fatalf("size %d step %d: digest after Reset differs", size, step)
				}
			}
		}
	}
}

func BenchmarkStreebog256_1K(b *testing.B) {
	data := make([]byte, 1024)
	b.SetBytes(1024)
	for i := 0; i < b.N; i++ {
		Sum256(data)
	}
}
