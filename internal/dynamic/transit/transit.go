// Package transit implements Tuck's Transit Secrets Engine: encryption as a
// service. Applications submit data for encryption/decryption/signing without
// ever handling raw key material. Keys are versioned; old ciphertext can be
// "rewrapped" to the latest key version after rotation.
//
// Supported key types:
//   - aes256-gcm96  — symmetric AES-256-GCM (encrypt/decrypt/rewrap/hmac)
//   - ecdsa-p256    — ECDSA P-256 (sign/verify/hmac)
//   - ed25519       — Ed25519 (sign/verify/hmac)
//   - rsa-2048      — RSA-PSS 2048-bit (sign/verify/hmac)
//   - rsa-4096      — RSA-PSS 4096-bit (sign/verify/hmac)
//
// Ciphertext and signature format: "vault:v{N}:{base64url-payload}"
// The version prefix enables versioned decryption and signature verification.
package transit

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	gohmac "crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	ErrNotFound          = errors.New("transit: key not found")
	ErrNotEncryptionKey  = errors.New("transit: key type does not support encrypt/decrypt")
	ErrNotSigningKey     = errors.New("transit: key type does not support sign/verify")
	ErrInvalidCiphertext = errors.New("transit: invalid ciphertext/signature format")
	ErrDecryptFailed     = errors.New("transit: decryption failed")
	ErrKeyVersionTooOld  = errors.New("transit: key version below min_decryption_version")
	ErrKeyNotDeletable   = errors.New("transit: key must be marked deletable before deletion")
	ErrUnsupportedType   = errors.New("transit: unsupported key type")
)

const keysPrefix = "transit/keys/"

// keyVersion stores the raw key material for one version of a key.
type keyVersion struct {
	CreatedAt time.Time `json:"created_at"`
	RawKey    string    `json:"raw_key"` // base64url-encoded raw key bytes
}

// keyRecord is the persisted representation of a transit key (all versions).
type keyRecord struct {
	Name          string                `json:"name"`
	Type          string                `json:"type"`
	Versions      map[int]keyVersion    `json:"versions"`
	LatestVersion int                   `json:"latest_version"`
	MinVersion    int                   `json:"min_decryption_version"`
	Deletable     bool                  `json:"deletable"`
	CreatedAt     time.Time             `json:"created_at"`
}

// Key is the public-facing view of a transit key — no key material.
type Key struct {
	Name          string                `json:"name"`
	Type          string                `json:"type"`
	LatestVersion int                   `json:"latest_version"`
	MinVersion    int                   `json:"min_decryption_version"`
	Deletable     bool                  `json:"deletable"`
	CreatedAt     time.Time             `json:"created_at"`
	Versions      map[int]KeyVersionInfo `json:"versions"`
}

// KeyVersionInfo is the public per-version metadata.
type KeyVersionInfo struct {
	CreatedAt time.Time `json:"created_at"`
}

type barrier interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Manager manages transit keys and cryptographic operations.
type Manager struct{ b barrier }

// NewManager creates a Manager backed by b.
func NewManager(b barrier) *Manager { return &Manager{b: b} }

// --- Key lifecycle ---

// CreateKey creates a named key of the given type. Idempotent: if the key
// already exists, it is returned without modification.
func (m *Manager) CreateKey(ctx context.Context, name, keyType string) error {
	if _, err := m.loadKey(ctx, name); err == nil {
		return nil // already exists — idempotent
	}
	if keyType == "" {
		keyType = "aes256-gcm96"
	}
	mat, err := generateKeyMaterial(keyType)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	rec := &keyRecord{
		Name:          name,
		Type:          keyType,
		Versions:      map[int]keyVersion{1: {CreatedAt: now, RawKey: mat}},
		LatestVersion: 1,
		MinVersion:    1,
		CreatedAt:     now,
	}
	return m.saveKey(ctx, rec)
}

// GetKey returns the public metadata for a key (no key material).
func (m *Manager) GetKey(ctx context.Context, name string) (*Key, error) {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return nil, err
	}
	return recordToPublic(rec), nil
}

// DeleteKey removes a key. Returns ErrKeyNotDeletable if Deletable is false.
func (m *Manager) DeleteKey(ctx context.Context, name string) error {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return err
	}
	if !rec.Deletable {
		return ErrKeyNotDeletable
	}
	return m.b.Delete(ctx, keysPrefix+name)
}

// ListKeys returns all key names.
func (m *Manager) ListKeys(ctx context.Context) ([]string, error) {
	keys, err := m.b.List(ctx, keysPrefix)
	if err != nil {
		return nil, err
	}
	for i, k := range keys {
		keys[i] = strings.TrimPrefix(k, keysPrefix)
	}
	return keys, nil
}

// Rotate generates a new key version and sets it as the latest.
// Old versions remain available for decryption/verification down to MinVersion.
func (m *Manager) Rotate(ctx context.Context, name string) error {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return err
	}
	mat, err := generateKeyMaterial(rec.Type)
	if err != nil {
		return err
	}
	newVer := rec.LatestVersion + 1
	rec.Versions[newVer] = keyVersion{CreatedAt: time.Now().UTC(), RawKey: mat}
	rec.LatestVersion = newVer
	return m.saveKey(ctx, rec)
}

// UpdateKey updates MinVersion and/or Deletable flags.
// minVersion=0 means no change. minVersion must be ≤ LatestVersion.
func (m *Manager) UpdateKey(ctx context.Context, name string, minVersion int, deletable bool) error {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return err
	}
	if minVersion > 0 && minVersion <= rec.LatestVersion {
		rec.MinVersion = minVersion
	}
	rec.Deletable = deletable
	return m.saveKey(ctx, rec)
}

// --- Symmetric operations (aes256-gcm96) ---

// Encrypt encrypts plaintext with the latest version of the named key.
// Returns a ciphertext in "vault:v{N}:{base64url}" format.
func (m *Manager) Encrypt(ctx context.Context, name string, plaintext []byte) (string, error) {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return "", err
	}
	if !isEncryptionKey(rec.Type) {
		return "", ErrNotEncryptionKey
	}
	v := rec.LatestVersion
	keyBytes, err := aesKeyBytes(rec, v)
	if err != nil {
		return "", err
	}
	encoded, err := aesEncrypt(keyBytes, plaintext)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("vault:v%d:%s", v, encoded), nil
}

// Decrypt decrypts a "vault:v{N}:{base64url}" ciphertext.
func (m *Manager) Decrypt(ctx context.Context, name string, ciphertext string) ([]byte, error) {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return nil, err
	}
	if !isEncryptionKey(rec.Type) {
		return nil, ErrNotEncryptionKey
	}
	v, encoded, err := parseVaultToken(ciphertext)
	if err != nil {
		return nil, err
	}
	if v < rec.MinVersion {
		return nil, ErrKeyVersionTooOld
	}
	kv, ok := rec.Versions[v]
	if !ok {
		return nil, fmt.Errorf("transit: key version %d not found", v)
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(kv.RawKey)
	if err != nil {
		return nil, err
	}
	return aesDecrypt(keyBytes, encoded)
}

// Rewrap decrypts a ciphertext with its original key version and re-encrypts
// it with the current latest version. Used to migrate ciphertext after rotation.
func (m *Manager) Rewrap(ctx context.Context, name string, ciphertext string) (string, error) {
	plain, err := m.Decrypt(ctx, name, ciphertext)
	if err != nil {
		return "", err
	}
	return m.Encrypt(ctx, name, plain)
}

// --- Asymmetric operations ---

// Sign signs input with the latest version of the named signing key.
// hashAlg controls the digest: "sha2-256" (default), "sha2-384", "sha2-512".
// Ed25519 uses its own internal hash and ignores hashAlg.
// Returns "vault:v{N}:{base64url-signature}".
func (m *Manager) Sign(ctx context.Context, name string, input []byte, hashAlg string) (string, error) {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return "", err
	}
	if !isSigningKey(rec.Type) {
		return "", ErrNotSigningKey
	}
	v := rec.LatestVersion
	priv, err := loadPrivKey(rec, v)
	if err != nil {
		return "", err
	}

	sig, err := signWith(priv, input, hashAlg)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("vault:v%d:%s", v, base64.RawURLEncoding.EncodeToString(sig)), nil
}

// Verify verifies a "vault:v{N}:{base64url-signature}" against input.
// hashAlg must match the value used during signing.
func (m *Manager) Verify(ctx context.Context, name string, input []byte, sigToken string, hashAlg string) (bool, error) {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return false, err
	}
	if !isSigningKey(rec.Type) {
		return false, ErrNotSigningKey
	}
	v, encoded, err := parseVaultToken(sigToken)
	if err != nil {
		return false, ErrInvalidCiphertext
	}
	priv, err := loadPrivKey(rec, v)
	if err != nil {
		return false, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return false, ErrInvalidCiphertext
	}
	return verifyWith(priv, input, sig, hashAlg)
}

// HMAC computes an HMAC of input under the named key's latest version.
// hashAlg: "sha2-256" (default), "sha2-384", "sha2-512".
// Returns "vault:v{N}:{base64url-mac}".
func (m *Manager) HMAC(ctx context.Context, name string, input []byte, hashAlg string) (string, error) {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return "", err
	}
	v := rec.LatestVersion
	kv, ok := rec.Versions[v]
	if !ok {
		return "", fmt.Errorf("transit: key version %d not found", v)
	}
	// Use the full raw key bytes as HMAC key regardless of key type.
	keyBytes, err := base64.RawURLEncoding.DecodeString(kv.RawKey)
	if err != nil {
		return "", err
	}
	mac := computeHMAC(keyBytes, input, hashAlg)
	return fmt.Sprintf("vault:v%d:%s", v, base64.RawURLEncoding.EncodeToString(mac)), nil
}

// --- helpers ---

func generateKeyMaterial(keyType string) (string, error) {
	var raw []byte
	switch keyType {
	case "aes256-gcm96":
		raw = make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return "", fmt.Errorf("transit: generate AES key: %w", err)
		}
	case "ecdsa-p256":
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return "", fmt.Errorf("transit: generate ECDSA key: %w", err)
		}
		raw, err = x509.MarshalECPrivateKey(priv)
		if err != nil {
			return "", err
		}
	case "ed25519":
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return "", fmt.Errorf("transit: generate Ed25519 key: %w", err)
		}
		raw = []byte(priv)
	case "rsa-2048":
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return "", fmt.Errorf("transit: generate RSA-2048 key: %w", err)
		}
		var marshalErr error
		raw, marshalErr = x509.MarshalPKCS8PrivateKey(priv)
		if marshalErr != nil {
			return "", marshalErr
		}
	case "rsa-4096":
		priv, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return "", fmt.Errorf("transit: generate RSA-4096 key: %w", err)
		}
		var marshalErr error
		raw, marshalErr = x509.MarshalPKCS8PrivateKey(priv)
		if marshalErr != nil {
			return "", marshalErr
		}
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedType, keyType)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func loadPrivKey(rec *keyRecord, version int) (crypto.PrivateKey, error) {
	kv, ok := rec.Versions[version]
	if !ok {
		return nil, fmt.Errorf("transit: key version %d not found", version)
	}
	raw, err := base64.RawURLEncoding.DecodeString(kv.RawKey)
	if err != nil {
		return nil, err
	}
	switch rec.Type {
	case "ecdsa-p256":
		return x509.ParseECPrivateKey(raw)
	case "ed25519":
		return ed25519.PrivateKey(raw), nil
	case "rsa-2048", "rsa-4096":
		key, err := x509.ParsePKCS8PrivateKey(raw)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("transit: expected RSA key, got %T", key)
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedType, rec.Type)
	}
}

func aesKeyBytes(rec *keyRecord, version int) ([]byte, error) {
	kv, ok := rec.Versions[version]
	if !ok {
		return nil, fmt.Errorf("transit: key version %d not found", version)
	}
	return base64.RawURLEncoding.DecodeString(kv.RawKey)
}

func aesEncrypt(key, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("transit: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(ct), nil
}

func aesDecrypt(key []byte, encoded string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, ErrInvalidCiphertext
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	return plain, nil
}

func signWith(priv crypto.PrivateKey, input []byte, hashAlg string) ([]byte, error) {
	switch k := priv.(type) {
	case *ecdsa.PrivateKey:
		h := digestInput(hashAlg, input)
		sig, err := ecdsa.SignASN1(rand.Reader, k, h)
		if err != nil {
			return nil, fmt.Errorf("transit: ecdsa sign: %w", err)
		}
		return sig, nil
	case ed25519.PrivateKey:
		// Ed25519 uses its own internal digest — hashAlg is intentionally ignored.
		return ed25519.Sign(k, input), nil
	case *rsa.PrivateKey:
		h := digestInput(hashAlg, input)
		hash := hashAlgToCryptoHash(hashAlg)
		sig, err := rsa.SignPSS(rand.Reader, k, hash, h, nil)
		if err != nil {
			return nil, fmt.Errorf("transit: rsa sign: %w", err)
		}
		return sig, nil
	default:
		return nil, fmt.Errorf("transit: unsupported private key type %T", priv)
	}
}

func verifyWith(priv crypto.PrivateKey, input, sig []byte, hashAlg string) (bool, error) {
	switch k := priv.(type) {
	case *ecdsa.PrivateKey:
		h := digestInput(hashAlg, input)
		return ecdsa.VerifyASN1(&k.PublicKey, h, sig), nil
	case ed25519.PrivateKey:
		pub := k.Public().(ed25519.PublicKey)
		return ed25519.Verify(pub, input, sig), nil
	case *rsa.PrivateKey:
		h := digestInput(hashAlg, input)
		hash := hashAlgToCryptoHash(hashAlg)
		err := rsa.VerifyPSS(&k.PublicKey, hash, h, sig, nil)
		return err == nil, nil
	default:
		return false, fmt.Errorf("transit: unsupported private key type %T", priv)
	}
}

func computeHMAC(keyBytes, input []byte, hashAlg string) []byte {
	switch strings.ToLower(hashAlg) {
	case "sha2-384":
		h := gohmac.New(sha512.New384, keyBytes)
		h.Write(input)
		return h.Sum(nil)
	case "sha2-512":
		h := gohmac.New(sha512.New, keyBytes)
		h.Write(input)
		return h.Sum(nil)
	default: // sha2-256
		h := gohmac.New(sha256.New, keyBytes)
		h.Write(input)
		return h.Sum(nil)
	}
}

func digestInput(hashAlg string, data []byte) []byte {
	switch strings.ToLower(hashAlg) {
	case "sha2-384":
		h := sha512.New384()
		h.Write(data)
		return h.Sum(nil)
	case "sha2-512":
		h := sha512.New()
		h.Write(data)
		return h.Sum(nil)
	default: // sha2-256
		h := sha256.New()
		h.Write(data)
		return h.Sum(nil)
	}
}

func hashAlgToCryptoHash(alg string) crypto.Hash {
	switch strings.ToLower(alg) {
	case "sha2-384":
		return crypto.SHA384
	case "sha2-512":
		return crypto.SHA512
	default:
		return crypto.SHA256
	}
}

// parseVaultToken parses "vault:v{N}:{payload}" and returns (N, payload, err).
func parseVaultToken(s string) (version int, payload string, err error) {
	if !strings.HasPrefix(s, "vault:v") {
		return 0, "", ErrInvalidCiphertext
	}
	rest := s[len("vault:v"):]
	idx := strings.IndexByte(rest, ':')
	if idx < 0 {
		return 0, "", ErrInvalidCiphertext
	}
	version, err = strconv.Atoi(rest[:idx])
	if err != nil || version < 1 {
		return 0, "", ErrInvalidCiphertext
	}
	return version, rest[idx+1:], nil
}

func isEncryptionKey(t string) bool { return t == "aes256-gcm96" }

func isSigningKey(t string) bool {
	switch t {
	case "ecdsa-p256", "ed25519", "rsa-2048", "rsa-4096":
		return true
	}
	return false
}

func recordToPublic(rec *keyRecord) *Key {
	k := &Key{
		Name:          rec.Name,
		Type:          rec.Type,
		LatestVersion: rec.LatestVersion,
		MinVersion:    rec.MinVersion,
		Deletable:     rec.Deletable,
		CreatedAt:     rec.CreatedAt,
		Versions:      make(map[int]KeyVersionInfo, len(rec.Versions)),
	}
	for v, km := range rec.Versions {
		k.Versions[v] = KeyVersionInfo{CreatedAt: km.CreatedAt}
	}
	return k
}

// --- storage helpers ---

func (m *Manager) loadKey(ctx context.Context, name string) (*keyRecord, error) {
	e, err := m.b.Get(ctx, keysPrefix+name)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrNotFound
	}
	var rec keyRecord
	if err := json.Unmarshal(e.Value, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func (m *Manager) saveKey(ctx context.Context, rec *keyRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return m.b.Put(ctx, &physical.Entry{Key: keysPrefix + rec.Name, Value: data})
}
