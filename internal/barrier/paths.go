package barrier

import (
	"context"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Path encryption.
//
// Stores created with metadata version 3 do not keep logical keys in the
// backend in the clear. Each "/"-separated segment is replaced by
//
//	base64url( nonce || AEAD(segment) )
//
// where nonce is a keyed hash of (parent path, segment), so the mapping is
// deterministic (a key always maps to the same stored key, which keeps Get,
// Put and directory listing working) yet reversible (List can show the
// original names). The parent stored path is also the AEAD associated data,
// so a segment cannot be moved to a different directory.
//
// What this hides: key names. What it does not: the tree shape, the number
// of entries, segment lengths, and whether two keys share the same segment
// under the same parent. Keys under the reserved "barrier/" prefix stay in
// the clear because they are needed before unsealing.

const reservedPrefix = "barrier/"

// pathCodec encodes and decodes storage keys.
type pathCodec struct {
	aead cipher.AEAD
	mac  []byte
	top  bool // clear the top nonce bit (MGM)
}

func newPathCodec(s Suite, pathKey []byte) (*pathCodec, error) {
	derive := func(label string) []byte {
		h := hmac.New(sha256.New, pathKey)
		h.Write([]byte(label))
		return h.Sum(nil)
	}
	enc := derive("tuck/path/enc")
	defer zero(enc)
	aead, err := s.New(enc)
	if err != nil {
		return nil, err
	}
	return &pathCodec{aead: aead, mac: derive("tuck/path/mac"), top: s.ClearNonceTopBit}, nil
}

func (c *pathCodec) zero() { zero(c.mac) }

func (c *pathCodec) nonce(parent, seg string) []byte {
	h := hmac.New(sha256.New, c.mac)
	h.Write([]byte(parent))
	h.Write([]byte{0})
	h.Write([]byte(seg))
	n := h.Sum(nil)[:c.aead.NonceSize()]
	if c.top {
		n[0] &= 0x7f
	}
	return n
}

// encode maps a logical key (or directory prefix ending in "/") to its
// stored form.
func (c *pathCodec) encode(key string) string {
	if strings.HasPrefix(key, reservedPrefix) {
		return key
	}
	segs := strings.Split(key, "/")
	parts := make([]string, 0, len(segs))
	for _, seg := range segs {
		if seg == "" {
			parts = append(parts, "")
			continue
		}
		parent := strings.Join(parts, "/")
		n := c.nonce(parent, seg)
		ct := c.aead.Seal(append([]byte(nil), n...), n, []byte(seg), []byte(parent))
		enc := base64.RawURLEncoding.EncodeToString(ct)
		parts = append(parts, enc)
	}
	return strings.Join(parts, "/")
}

var errBadStoredKey = errors.New("malformed encrypted storage key")

// decode reverses encode.
func (c *pathCodec) decode(stored string) (string, error) {
	if strings.HasPrefix(stored, reservedPrefix) {
		return stored, nil
	}
	parts := strings.Split(stored, "/")
	out := make([]string, len(parts))
	ns := c.aead.NonceSize()
	for i, p := range parts {
		if p == "" {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(p)
		if err != nil || len(raw) < ns {
			return "", errBadStoredKey
		}
		parent := strings.Join(parts[:i], "/")
		seg, err := c.aead.Open(nil, raw[:ns], raw[ns:], []byte(parent))
		if err != nil {
			return "", errBadStoredKey
		}
		out[i] = string(seg)
	}
	return strings.Join(out, "/"), nil
}

// phys returns the stored form of a logical key.
func (b *Barrier) phys(key string) string {
	if b.paths == nil {
		return key
	}
	return b.paths.encode(key)
}

// listKeys lists logical keys with the given logical prefix. Caller holds b.mu.
func (b *Barrier) listKeys(ctx context.Context, prefix string) ([]string, error) {
	if b.paths == nil {
		return b.backend.List(ctx, prefix)
	}
	// Only complete segments can be translated into a stored prefix; a
	// trailing partial segment is filtered on the decrypted names.
	dir := prefix[:strings.LastIndexByte(prefix, '/')+1]
	scan := ""
	switch {
	case strings.HasPrefix(prefix, reservedPrefix):
		scan = prefix
	case dir != "":
		scan = b.paths.encode(dir)
	}
	stored, err := b.backend.List(ctx, scan)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(stored))
	for _, s := range stored {
		k, err := b.paths.decode(s)
		if err != nil {
			return nil, fmt.Errorf("list %q: %w", prefix, err)
		}
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}
