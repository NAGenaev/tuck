// Package kdf implements HMAC_GOSTR3411_2012 and KDF_GOSTR3411_2012_256
// (RFC 7836, R 50.1.113-2016) on top of the Streebog hash.
package kdf

import (
	"crypto/hmac"

	"github.com/NAGenaev/tuck/internal/gost/streebog"
)

// HMAC256 returns HMAC_GOSTR3411_2012_256(key, data).
func HMAC256(key, data []byte) []byte {
	m := hmac.New(streebog.New256, key)
	m.Write(data)
	return m.Sum(nil)
}

// HMAC512 returns HMAC_GOSTR3411_2012_512(key, data).
func HMAC512(key, data []byte) []byte {
	m := hmac.New(streebog.New512, key)
	m.Write(data)
	return m.Sum(nil)
}

// KDF256 returns KDF_GOSTR3411_2012_256(key, label, seed):
// HMAC_256(key, 0x01 || label || 0x00 || seed || 0x01 || 0x00), a 32-byte key.
func KDF256(key, label, seed []byte) []byte {
	msg := make([]byte, 0, len(label)+len(seed)+4)
	msg = append(msg, 0x01)
	msg = append(msg, label...)
	msg = append(msg, 0x00)
	msg = append(msg, seed...)
	msg = append(msg, 0x01, 0x00)
	return HMAC256(key, msg)
}
