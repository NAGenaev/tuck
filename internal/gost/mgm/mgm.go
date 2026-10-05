// Package mgm implements the Multilinear Galois Mode AEAD (RFC 9058,
// GOST R 1323565.1.026-2019) for 128-bit block ciphers such as Kuznyechik.
// It is a non-certified software implementation.
package mgm

import (
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"
)

const (
	// NonceSize is the MGM nonce (ICN) size for 128-bit blocks.
	NonceSize = 16
	// TagSize is the authentication tag size.
	TagSize = 16
)

var errOpen = errors.New("mgm: message authentication failed")

type mgm struct {
	b cipher.Block
}

// New returns an MGM AEAD over a 128-bit block cipher.
func New(block cipher.Block) (cipher.AEAD, error) {
	if block.BlockSize() != 16 {
		return nil, errors.New("mgm: only 128-bit block ciphers are supported")
	}
	return &mgm{b: block}, nil
}

func (m *mgm) NonceSize() int { return NonceSize }
func (m *mgm) Overhead() int  { return TagSize }

type u128 struct{ hi, lo uint64 }

func load(b []byte) u128 {
	return u128{binary.BigEndian.Uint64(b[0:8]), binary.BigEndian.Uint64(b[8:16])}
}

func (x u128) put(b []byte) {
	binary.BigEndian.PutUint64(b[0:8], x.hi)
	binary.BigEndian.PutUint64(b[8:16], x.lo)
}

// gfMul multiplies in GF(2^128) with the polynomial x^128+x^7+x^2+x+1,
// treating the byte string as a big-endian integer.
func gfMul(a, b u128) u128 {
	var r u128
	for i := 0; i < 128; i++ {
		var bit uint64
		if i < 64 {
			bit = (b.lo >> uint(i)) & 1
		} else {
			bit = (b.hi >> uint(i-64)) & 1
		}
		mask := -bit
		r.hi ^= a.hi & mask
		r.lo ^= a.lo & mask
		carry := a.hi >> 63
		a.hi = a.hi<<1 | a.lo>>63
		a.lo = a.lo<<1 ^ (carry * 0x87)
	}
	return r
}

func checkNonce(nonce []byte) {
	if len(nonce) != NonceSize {
		panic("mgm: incorrect nonce length given to MGM")
	}
	if nonce[0]&0x80 != 0 {
		panic("mgm: the most significant bit of the nonce must be zero")
	}
}

// tag computes the authentication tag over additional data and ciphertext.
func (m *mgm) tag(nonce, ad, ct []byte) [TagSize]byte {
	var zb [16]byte
	copy(zb[:], nonce)
	zb[0] |= 0x80
	var enc [16]byte
	m.b.Encrypt(enc[:], zb[:])
	z := load(enc[:])

	var sum u128
	var blk, h [16]byte
	absorb := func(data []byte) {
		for len(data) > 0 {
			blk = [16]byte{}
			n := copy(blk[:], data)
			data = data[n:]
			z.put(zb[:])
			m.b.Encrypt(h[:], zb[:])
			p := gfMul(load(h[:]), load(blk[:]))
			sum.hi ^= p.hi
			sum.lo ^= p.lo
			z.hi++ // incr_l: left 64-bit half modulo 2^64
		}
	}
	absorb(ad)
	absorb(ct)

	var lens [16]byte
	binary.BigEndian.PutUint64(lens[0:8], uint64(len(ad))*8)
	binary.BigEndian.PutUint64(lens[8:16], uint64(len(ct))*8)
	z.put(zb[:])
	m.b.Encrypt(h[:], zb[:])
	p := gfMul(load(h[:]), load(lens[:]))
	sum.hi ^= p.hi
	sum.lo ^= p.lo

	sum.put(blk[:])
	var t [TagSize]byte
	m.b.Encrypt(t[:], blk[:])
	return t
}

func (m *mgm) ctr(nonce, dst, src []byte) {
	var yb [16]byte
	copy(yb[:], nonce)
	yb[0] &= 0x7f
	var ks [16]byte
	m.b.Encrypt(ks[:], yb[:])
	y := load(ks[:])
	for len(src) > 0 {
		y.put(yb[:])
		m.b.Encrypt(ks[:], yb[:])
		n := len(src)
		if n > 16 {
			n = 16
		}
		for i := 0; i < n; i++ {
			dst[i] = src[i] ^ ks[i]
		}
		src, dst = src[n:], dst[n:]
		y.lo++ // incr_r: right 64-bit half modulo 2^64
	}
}

func (m *mgm) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	checkNonce(nonce)
	ret, out := sliceForAppend(dst, len(plaintext)+TagSize)
	m.ctr(nonce, out[:len(plaintext)], plaintext)
	t := m.tag(nonce, additionalData, out[:len(plaintext)])
	copy(out[len(plaintext):], t[:])
	return ret
}

func (m *mgm) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(nonce) != NonceSize || nonce[0]&0x80 != 0 {
		return nil, errors.New("mgm: invalid nonce")
	}
	if len(ciphertext) < TagSize {
		return nil, errOpen
	}
	ct, tag := ciphertext[:len(ciphertext)-TagSize], ciphertext[len(ciphertext)-TagSize:]
	want := m.tag(nonce, additionalData, ct)
	if subtle.ConstantTimeCompare(want[:], tag) != 1 {
		return nil, errOpen
	}
	ret, out := sliceForAppend(dst, len(ct))
	m.ctr(nonce, out, ct)
	return ret, nil
}

func sliceForAppend(in []byte, n int) (head, tail []byte) {
	if total := len(in) + n; cap(in) >= total {
		head = in[:total]
	} else {
		head = make([]byte, total)
		copy(head, in)
	}
	tail = head[len(in):]
	return
}
