// Package streebog implements the GOST R 34.11-2012 "Streebog" hash function
// (RFC 6986) with 256- and 512-bit outputs. It is a non-certified software
// implementation.
package streebog

import (
	"encoding/binary"
	"hash"
	"sync"
)

const (
	// Size256 and Size512 are the digest sizes in bytes.
	Size256 = 32
	Size512 = 64
	// BlockSize is the internal block size in bytes.
	BlockSize = 64
)

var (
	tblOnce sync.Once
	// lps[j][v]: contribution of byte value v at position j of a 64-bit word
	// after S and the linear transformation l.
	lps [8][256]uint64
)

func initTables() {
	for j := 0; j < 8; j++ {
		for v := 0; v < 256; v++ {
			lps[j][v] = linear(uint64(pi[v]) << (8 * uint(j)))
		}
	}
}

// linear multiplies the little-endian 64-bit vector by matrix A: bit i of x
// selects row 63-i.
func linear(x uint64) uint64 {
	var r uint64
	for i := uint(0); i < 64; i++ {
		if x>>i&1 != 0 {
			r ^= matA[63-i]
		}
	}
	return r
}

type vec [8]uint64 // little-endian 64-byte vector

func (a *vec) xor(b *vec) vec {
	var r vec
	for i := range r {
		r[i] = a[i] ^ b[i]
	}
	return r
}

func loadVec(b []byte) vec {
	var v vec
	for i := range v {
		v[i] = binary.LittleEndian.Uint64(b[8*i:])
	}
	return v
}

func (a *vec) put(b []byte) {
	for i := range a {
		binary.LittleEndian.PutUint64(b[8*i:], a[i])
	}
}

// lpsTransform applies L(P(S(a))). Output word w is built from bytes w, 8+w, ...,
// 56+w of the input (P is the 8x8 byte transposition).
func lpsTransform(a *vec) vec {
	var r vec
	for w := 0; w < 8; w++ {
		var x uint64
		for j := 0; j < 8; j++ {
			b := byte(a[j] >> (8 * uint(w)))
			x ^= lps[j][b]
		}
		r[w] = x
	}
	return r
}

func encrypt(k, m vec) vec {
	for i := 0; i < 12; i++ {
		x := m.xor(&k)
		m = lpsTransform(&x)
		c := loadVec(roundC[i][:])
		kc := k.xor(&c)
		k = lpsTransform(&kc)
	}
	return m.xor(&k)
}

func g(n, h, m vec) vec {
	x := h.xor(&n)
	k := lpsTransform(&x)
	e := encrypt(k, m)
	e = e.xor(&h)
	return e.xor(&m)
}

func add512(a *vec, b vec) {
	var carry uint64
	for i := range a {
		s := a[i] + b[i]
		c1 := uint64(0)
		if s < a[i] {
			c1 = 1
		}
		s2 := s + carry
		if s2 < s {
			c1 = 1
		}
		a[i], carry = s2, c1
	}
}

type digest struct {
	size  int
	h     vec
	n     vec
	sigma vec
	buf   [BlockSize]byte
	nbuf  int
}

// New256 returns a Streebog-256 hash.
func New256() hash.Hash { return newDigest(Size256) }

// New512 returns a Streebog-512 hash.
func New512() hash.Hash { return newDigest(Size512) }

func newDigest(size int) *digest {
	tblOnce.Do(initTables)
	d := &digest{size: size}
	d.Reset()
	return d
}

func (d *digest) Size() int      { return d.size }
func (d *digest) BlockSize() int { return BlockSize }

func (d *digest) Reset() {
	d.h = vec{}
	if d.size == Size256 {
		for i := range d.h {
			d.h[i] = 0x0101010101010101
		}
	}
	d.n = vec{}
	d.sigma = vec{}
	d.nbuf = 0
}

func (d *digest) block(b []byte) {
	m := loadVec(b)
	d.h = g(d.n, d.h, m)
	add512(&d.n, vec{512})
	add512(&d.sigma, m)
}

func (d *digest) Write(p []byte) (int, error) {
	total := len(p)
	// Hold back a full block until more data arrives so Sum can pad the
	// final block uniformly (a message that is a multiple of 64 bytes ends
	// with an empty padded block).
	if d.nbuf > 0 {
		n := copy(d.buf[d.nbuf:], p)
		d.nbuf += n
		p = p[n:]
		if d.nbuf == BlockSize && len(p) > 0 {
			d.block(d.buf[:])
			d.nbuf = 0
		}
	}
	for len(p) > BlockSize {
		d.block(p[:BlockSize])
		p = p[BlockSize:]
	}
	if len(p) > 0 {
		d.nbuf += copy(d.buf[d.nbuf:], p)
	}
	return total, nil
}

func (d *digest) Sum(in []byte) []byte {
	c := *d
	if c.nbuf == BlockSize {
		c.block(c.buf[:])
		c.nbuf = 0
	}
	var last [BlockSize]byte
	copy(last[:], c.buf[:c.nbuf])
	last[c.nbuf] = 1
	m := loadVec(last[:])
	c.h = g(c.n, c.h, m)
	add512(&c.n, vec{uint64(c.nbuf) * 8})
	add512(&c.sigma, m)
	c.h = g(vec{}, c.h, c.n)
	c.h = g(vec{}, c.h, c.sigma)
	var out [BlockSize]byte
	c.h.put(out[:])
	if c.size == Size256 {
		return append(in, out[32:]...)
	}
	return append(in, out[:]...)
}

// Sum256 returns the Streebog-256 digest of data.
func Sum256(data []byte) [Size256]byte {
	d := New256()
	d.Write(data)
	var r [Size256]byte
	copy(r[:], d.Sum(nil))
	return r
}

// Sum512 returns the Streebog-512 digest of data.
func Sum512(data []byte) [Size512]byte {
	d := New512()
	d.Write(data)
	var r [Size512]byte
	copy(r[:], d.Sum(nil))
	return r
}
