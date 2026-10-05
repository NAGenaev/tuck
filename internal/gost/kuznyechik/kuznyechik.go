// Package kuznyechik implements GOST R 34.12-2015 "Kuznyechik" (RFC 7801).
// It is a non-certified software implementation; FSB certification requires a certified SKZI.
//
// The implementation is table driven (16 tables of 256 128-bit entries per
// direction). Table lookups indexed by secret data are not constant-time and
// may leak through cache timing on shared hardware.
package kuznyechik

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"sync"
)

const (
	// BlockSize is the Kuznyechik block size in bytes.
	BlockSize = 16
	// KeySize is the Kuznyechik key size in bytes.
	KeySize = 32

	rounds = 10
)

// linCoef are the coefficients of l(a15..a0) in field GF(2)[x]/(x^8+x^7+x^6+x+1),
// listed from the leftmost (a15) to the rightmost (a0) byte (RFC 7801 section 4.2).
var linCoef = [16]byte{148, 32, 133, 16, 194, 192, 1, 251, 1, 192, 194, 16, 133, 32, 148, 1}

type block [2]uint64 // [0] = bytes 0..7, [1] = bytes 8..15, both big-endian

type tables struct {
	enc [16][256]block // enc[i][v] = L(S(v) placed at byte index i)
	dec [16][256]block // dec[i][v] = L^-1(v placed at byte index i)
	c   [32]block      // round constants C_1..C_32
}

var (
	tblOnce sync.Once
	tbl     *tables
)

func getTables() *tables {
	tblOnce.Do(func() {
		t := new(tables)
		for i := 0; i < 16; i++ {
			for v := 0; v < 256; v++ {
				var x [16]byte
				x[i] = pi[v]
				t.enc[i][v] = toBlock(lTransform(x))
				var y [16]byte
				y[i] = byte(v)
				t.dec[i][v] = toBlock(lInverse(y))
			}
		}
		for i := 1; i <= 32; i++ {
			var x [16]byte
			x[15] = byte(i)
			t.c[i-1] = toBlock(lTransform(x))
		}
		tbl = t
	})
	return tbl
}

// gfMul multiplies in GF(2)[x]/(x^8+x^7+x^6+x+1).
func gfMul(a, b byte) byte {
	var r byte
	for i := 0; i < 8; i++ {
		if b&1 != 0 {
			r ^= a
		}
		hi := a & 0x80
		a <<= 1
		if hi != 0 {
			a ^= 0xC3 // 0x1C3 without the x^8 term
		}
		b >>= 1
	}
	return r
}

func linear(b *[16]byte) byte {
	var r byte
	for k := 0; k < 16; k++ {
		r ^= gfMul(linCoef[k], b[k])
	}
	return r
}

// lTransform computes L = R^16 on a byte string (byte 0 is a15).
func lTransform(x [16]byte) [16]byte {
	for n := 0; n < 16; n++ {
		l := linear(&x)
		copy(x[1:], x[:15])
		x[0] = l
	}
	return x
}

// lInverse computes L^-1 = (R^-1)^16.
func lInverse(x [16]byte) [16]byte {
	for n := 0; n < 16; n++ {
		var n [16]byte
		copy(n[:15], x[1:])
		n[15] = x[0]
		n[15] = linear(&n) // l(a14..a0, a15) replaces the rotated-in byte
		x = n
	}
	return x
}

func toBlock(b [16]byte) block {
	return block{binary.BigEndian.Uint64(b[0:8]), binary.BigEndian.Uint64(b[8:16])}
}

func (b block) put(dst []byte) {
	binary.BigEndian.PutUint64(dst[0:8], b[0])
	binary.BigEndian.PutUint64(dst[8:16], b[1])
}

func (b block) xor(o block) block { return block{b[0] ^ o[0], b[1] ^ o[1]} }

// lsx computes L(S(x xor k)) with the precomputed tables.
func (t *tables) lsx(x block) block {
	var r block
	for i := 0; i < 8; i++ {
		v := byte(x[0] >> (56 - 8*uint(i)))
		e := &t.enc[i][v]
		r[0] ^= e[0]
		r[1] ^= e[1]
	}
	for i := 0; i < 8; i++ {
		v := byte(x[1] >> (56 - 8*uint(i)))
		e := &t.enc[8+i][v]
		r[0] ^= e[0]
		r[1] ^= e[1]
	}
	return r
}

// invRound computes S^-1(L^-1(x)).
func (t *tables) invRound(x block) block {
	var r block
	for i := 0; i < 8; i++ {
		v := byte(x[0] >> (56 - 8*uint(i)))
		e := &t.dec[i][v]
		r[0] ^= e[0]
		r[1] ^= e[1]
	}
	for i := 0; i < 8; i++ {
		v := byte(x[1] >> (56 - 8*uint(i)))
		e := &t.dec[8+i][v]
		r[0] ^= e[0]
		r[1] ^= e[1]
	}
	var buf [16]byte
	r.put(buf[:])
	for i := range buf {
		buf[i] = piInv[buf[i]]
	}
	return toBlock(buf)
}

type kuz struct {
	t  *tables
	rk [rounds]block
}

// NewCipher returns a Kuznyechik block cipher for a 32-byte key.
func NewCipher(key []byte) (cipher.Block, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("kuznyechik: invalid key size %d, want %d", len(key), KeySize)
	}
	t := getTables()
	k := &kuz{t: t}
	a1 := toBlock([16]byte(key[0:16]))
	a0 := toBlock([16]byte(key[16:32]))
	k.rk[0], k.rk[1] = a1, a0
	for i := 1; i <= 4; i++ {
		for j := 0; j < 8; j++ {
			c := t.c[8*(i-1)+j]
			a1, a0 = t.lsx(a1.xor(c)).xor(a0), a1
		}
		k.rk[2*i], k.rk[2*i+1] = a1, a0
	}
	return k, nil
}

func (k *kuz) BlockSize() int { return BlockSize }

func (k *kuz) Encrypt(dst, src []byte) {
	if len(src) < BlockSize || len(dst) < BlockSize {
		panic("kuznyechik: input or output not full block")
	}
	x := toBlock([16]byte(src[:16]))
	for r := 0; r < rounds-1; r++ {
		x = k.t.lsx(x.xor(k.rk[r]))
	}
	x.xor(k.rk[rounds-1]).put(dst)
}

func (k *kuz) Decrypt(dst, src []byte) {
	if len(src) < BlockSize || len(dst) < BlockSize {
		panic("kuznyechik: input or output not full block")
	}
	x := toBlock([16]byte(src[:16]))
	for r := rounds - 1; r > 0; r-- {
		x = k.t.invRound(x.xor(k.rk[r]))
	}
	x.xor(k.rk[0]).put(dst)
}
