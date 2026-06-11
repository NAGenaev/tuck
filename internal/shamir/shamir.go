// Package shamir implements Shamir's Secret Sharing over GF(256).
//
// No external dependencies — only stdlib. The finite field GF(256) uses the
// AES irreducible polynomial x^8 + x^4 + x^3 + x + 1 (0x11b). Multiplication
// is computed via log/exp look-up tables built from primitive root g=3.
package shamir

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// ---- GF(256) look-up tables ----
//
// Field polynomial: x^8 + x^4 + x^3 + x + 1 = 0x11b (AES field).
// Primitive root g = 3 generates all 255 non-zero elements.

var (
	logTable [256]byte // logTable[a] = log_{g=3}(a); logTable[0] is unused
	expTable [512]byte // expTable[i] = 3^i mod poly; doubled to avoid mod-255 wrapping
)

func init() {
	// Build exp/log tables using g=3 as the primitive root of GF(2^8) with
	// irreducible polynomial 0x11b (x^8 + x^4 + x^3 + x + 1).
	//
	// To multiply by 3 = (x+1) in GF(2^8):
	//   mul3(a) = mul2(a) XOR a
	// where mul2(a) = (a<<1) XOR (0x1b if a&0x80 != 0 else 0).
	x := 1
	for i := 0; i < 255; i++ {
		expTable[i] = byte(x)
		expTable[i+255] = byte(x)
		logTable[x] = byte(i)
		// x = x * 3 in GF(2^8)
		doubled := x << 1
		if x&0x80 != 0 {
			doubled ^= 0x1b
		}
		x = (doubled ^ x) & 0xff
	}
}

// gfMul multiplies two GF(256) elements.
func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return expTable[int(logTable[a])+int(logTable[b])]
}

// gfDiv divides a by b in GF(256). b must not be 0.
func gfDiv(a, b byte) byte {
	if b == 0 {
		panic("shamir: division by zero in GF(256)")
	}
	if a == 0 {
		return 0
	}
	// a/b = g^(log(a) - log(b)) = g^(log(a) + 255 - log(b)) mod 255
	return expTable[(int(logTable[a])+255-int(logTable[b]))%255]
}

// ---- Polynomial evaluation ----

// evalPoly evaluates the polynomial at x using Horner's method.
// coeffs[0] is the constant term (degree 0).
func evalPoly(coeffs []byte, x byte) byte {
	if len(coeffs) == 0 {
		return 0
	}
	result := coeffs[len(coeffs)-1]
	for i := len(coeffs) - 2; i >= 0; i-- {
		result = coeffs[i] ^ gfMul(result, x)
	}
	return result
}

// ---- Public API ----

// Split splits secret into n shares such that any k shares reconstruct it.
//
// Each returned share is (1 + len(secret)) bytes:
//   - share[0]  — x-coordinate (1..n)
//   - share[1:] — polynomial evaluation for each secret byte
func Split(secret []byte, n, k int) ([][]byte, error) {
	if len(secret) == 0 {
		return nil, errors.New("shamir: secret must not be empty")
	}
	if n < 1 || n > 255 {
		return nil, errors.New("shamir: n must be in [1, 255]")
	}
	if k < 1 {
		return nil, errors.New("shamir: k must be at least 1")
	}
	if k > n {
		return nil, fmt.Errorf("shamir: k (%d) must not exceed n (%d)", k, n)
	}

	shares := make([][]byte, n)
	for i := 0; i < n; i++ {
		shares[i] = make([]byte, 1+len(secret))
		shares[i][0] = byte(i + 1)
	}

	coeffBuf := make([]byte, k-1)
	for byteIdx, s := range secret {
		coeffs := make([]byte, k)
		coeffs[0] = s

		if k > 1 {
			// Fill random coefficients a_1..a_{k-1}.
			if _, err := io.ReadFull(rand.Reader, coeffBuf); err != nil {
				return nil, fmt.Errorf("shamir: rand: %w", err)
			}
			copy(coeffs[1:], coeffBuf)

			// Ensure leading coefficient is non-zero so degree is exactly k-1.
			for coeffs[k-1] == 0 {
				b := [1]byte{}
				if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
					return nil, fmt.Errorf("shamir: rand: %w", err)
				}
				coeffs[k-1] = b[0]
			}
		}

		for i := range shares {
			shares[i][1+byteIdx] = evalPoly(coeffs, byte(i+1))
		}
	}
	return shares, nil
}

// Combine reconstructs the secret from shares via Lagrange interpolation at
// x=0 in GF(256). At least k shares must be provided; extra shares beyond
// what was used in Split are fine — all are used. Share order does not matter.
func Combine(shares [][]byte) ([]byte, error) {
	if len(shares) == 0 {
		return nil, errors.New("shamir: no shares provided")
	}
	shareLen := len(shares[0])
	if shareLen < 2 {
		return nil, errors.New("shamir: share too short (must be at least 2 bytes)")
	}
	for i, sh := range shares {
		if len(sh) != shareLen {
			return nil, fmt.Errorf("shamir: share %d has inconsistent length %d (expected %d)",
				i, len(sh), shareLen)
		}
	}

	// Check for duplicate x-coordinates.
	seen := make(map[byte]bool, len(shares))
	for i, sh := range shares {
		if seen[sh[0]] {
			return nil, fmt.Errorf("shamir: duplicate x-coordinate %d in share %d", sh[0], i)
		}
		seen[sh[0]] = true
	}

	secretLen := shareLen - 1
	secret := make([]byte, secretLen)

	xs := make([]byte, len(shares))
	for i, sh := range shares {
		xs[i] = sh[0]
	}

	// Lagrange interpolation at x=0 for each byte position.
	for byteIdx := 0; byteIdx < secretLen; byteIdx++ {
		var value byte
		for j, xj := range xs {
			yj := shares[j][1+byteIdx]
			// Compute the basis polynomial l_j(0):
			//   l_j(0) = product_{m != j} ( (0 - x_m) / (x_j - x_m) )
			// In GF(2^8) subtraction == addition == XOR, so (0 - x) = x.
			num := byte(1)
			den := byte(1)
			for m, xm := range xs {
				if m == j {
					continue
				}
				num = gfMul(num, xm)
				den = gfMul(den, xj^xm)
			}
			value ^= gfMul(yj, gfDiv(num, den))
		}
		secret[byteIdx] = value
	}
	return secret, nil
}
