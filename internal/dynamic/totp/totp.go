// Package totp implements Tuck's TOTP (Time-based One-Time Password) Secrets
// Engine. Tuck stores TOTP secrets inside the encrypted barrier and generates
// or validates codes on demand — useful for application 2FA and
// service-to-service authentication via short-lived codes.
//
// Workflow:
//
//	# 1. Create a TOTP key (random secret generated automatically)
//	curl -XPOST https://tuck:8200/v1/totp/keys/myapp \
//	  -H "X-Tuck-Token: $TOKEN" \
//	  -d '{"issuer":"ACME Corp","account":"user@example.com"}'
//	# Returns secret + otpauth:// URL — import the URL into any authenticator app.
//
//	# 2. Validate a code from the authenticator
//	curl -XPOST https://tuck:8200/v1/totp/code/myapp \
//	  -H "X-Tuck-Token: $TOKEN" \
//	  -d '{"code":"123456"}'
//
//	# 3. Or generate the current code server-side
//	curl -XGET https://tuck:8200/v1/totp/code/myapp \
//	  -H "X-Tuck-Token: $TOKEN"
package totp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	ErrNotFound      = errors.New("totp: key not found")
	ErrInvalidSecret = errors.New("totp: invalid base32 secret")
)

const keysPrefix = "dynamic/totp/keys/"

// keyRecord is persisted in the barrier (barrier encryption protects the secret).
type keyRecord struct {
	Name      string `json:"name"`
	Issuer    string `json:"issuer"`
	Account   string `json:"account"`
	Algorithm string `json:"algorithm"` // sha1, sha256, sha512
	Digits    int    `json:"digits"`    // 6 or 8
	Period    int    `json:"period"`    // seconds (typically 30)
	Skew      int    `json:"skew"`      // allowed window in periods on each side
	Secret    string `json:"secret"`    // base32-encoded (no padding)
}

// KeyInfo is the public view of a key — never includes the raw secret.
type KeyInfo struct {
	Name      string `json:"name"`
	Issuer    string `json:"issuer"`
	Account   string `json:"account"`
	Algorithm string `json:"algorithm"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
	Skew      int    `json:"skew"`
	// URL is an otpauth:// URI. Pass it to a QR code generator so users can
	// import the key into any standard authenticator app.
	URL string `json:"url"`
}

// CreateKeyRequest configures a new TOTP key.
type CreateKeyRequest struct {
	Issuer    string `json:"issuer"`
	Account   string `json:"account"`
	Algorithm string `json:"algorithm"` // sha1 (default), sha256, sha512
	Digits    int    `json:"digits"`    // 6 (default) or 8
	Period    int    `json:"period"`    // 30s (default)
	Skew      int    `json:"skew"`      // 1 (default)
	// Secret is an optional existing base32-encoded TOTP secret to import.
	// When empty a random 20-byte (160-bit) secret is generated.
	Secret string `json:"secret"`
}

// CreateResult is returned on key creation — includes the secret once.
// The secret is never returned by subsequent GET calls.
type CreateResult struct {
	KeyInfo
	Secret string `json:"secret"`
}

// GenerateResult is returned by GenerateCode.
type GenerateResult struct {
	Code       string    `json:"code"`
	ValidUntil time.Time `json:"valid_until"` // end of the current TOTP window
}

type barrier interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Manager manages TOTP keys stored in the encrypted barrier.
type Manager struct{ b barrier }

// NewManager creates a Manager backed by b.
func NewManager(b barrier) *Manager { return &Manager{b: b} }

// CreateKey creates or overwrites a TOTP key. If req.Secret is empty a random
// 20-byte secret is generated (recommended).
func (m *Manager) CreateKey(ctx context.Context, name string, req CreateKeyRequest) (*CreateResult, error) {
	if req.Algorithm == "" {
		req.Algorithm = "sha1"
	}
	if req.Digits == 0 {
		req.Digits = 6
	}
	if req.Period == 0 {
		req.Period = 30
	}
	if req.Skew == 0 {
		req.Skew = 1
	}
	if req.Issuer == "" {
		req.Issuer = "Tuck"
	}
	if req.Account == "" {
		req.Account = name
	}
	if _, err := hashFunc(req.Algorithm); err != nil {
		return nil, err
	}

	secret := strings.ToUpper(strings.TrimRight(req.Secret, "="))
	if secret == "" {
		raw := make([]byte, 20)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("totp: generate secret: %w", err)
		}
		secret = strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "=")
	} else {
		if _, err := decodeSecret(secret); err != nil {
			return nil, ErrInvalidSecret
		}
	}

	rec := &keyRecord{
		Name:      name,
		Issuer:    req.Issuer,
		Account:   req.Account,
		Algorithm: req.Algorithm,
		Digits:    req.Digits,
		Period:    req.Period,
		Skew:      req.Skew,
		Secret:    secret,
	}
	if err := m.put(ctx, keysPrefix+name, rec); err != nil {
		return nil, err
	}
	return &CreateResult{KeyInfo: recToInfo(rec), Secret: secret}, nil
}

// GetKey returns metadata for an existing key. The secret is never included.
func (m *Manager) GetKey(ctx context.Context, name string) (*KeyInfo, error) {
	var rec keyRecord
	if err := m.get(ctx, keysPrefix+name, &rec); err != nil {
		return nil, err
	}
	info := recToInfo(&rec)
	return &info, nil
}

// DeleteKey removes a key from the store.
func (m *Manager) DeleteKey(ctx context.Context, name string) error {
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

// GenerateCode returns the current TOTP code for the named key.
func (m *Manager) GenerateCode(ctx context.Context, name string) (*GenerateResult, error) {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return nil, err
	}
	raw, err := decodeSecret(rec.Secret)
	if err != nil {
		return nil, err
	}
	h, _ := hashFunc(rec.Algorithm)
	now := time.Now().UTC()
	code := totpCode(raw, now, rec.Digits, rec.Period, h)

	// Compute when this code expires (end of the current period).
	periodDur := time.Duration(rec.Period) * time.Second
	validUntil := now.Truncate(periodDur).Add(periodDur)

	return &GenerateResult{Code: code, ValidUntil: validUntil}, nil
}

// ValidateCode validates a TOTP code against the named key within the allowed
// skew window. Returns true on a match.
func (m *Manager) ValidateCode(ctx context.Context, name, code string) (bool, error) {
	rec, err := m.loadKey(ctx, name)
	if err != nil {
		return false, err
	}
	raw, err := decodeSecret(rec.Secret)
	if err != nil {
		return false, err
	}
	h, _ := hashFunc(rec.Algorithm)
	return validateCode(raw, code, time.Now(), rec.Digits, rec.Period, rec.Skew, h), nil
}

// --- TOTP math (RFC 6238 / RFC 4226) ---

func totpCode(key []byte, t time.Time, digits, period int, h func() hash.Hash) string {
	counter := uint64(t.Unix()) / uint64(period)
	return hotpCode(key, counter, digits, h)
}

// hotpCode computes the HOTP value for the given key, counter, and hash.
// Dynamic truncation per RFC 4226 §5.4.
func hotpCode(key []byte, counter uint64, digits int, h func() hash.Hash) string {
	mac := hmac.New(h, key)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, code%mod)
}

func validateCode(key []byte, code string, t time.Time, digits, period, skew int, h func() hash.Hash) bool {
	base := int64(t.Unix()) / int64(period)
	for i := -skew; i <= skew; i++ {
		c := base + int64(i)
		if c < 0 {
			continue
		}
		if hotpCode(key, uint64(c), digits, h) == code {
			return true
		}
	}
	return false
}

// --- helpers ---

func hashFunc(alg string) (func() hash.Hash, error) {
	switch strings.ToLower(alg) {
	case "sha1":
		return sha1.New, nil
	case "sha256":
		return sha256.New, nil
	case "sha512":
		return sha512.New, nil
	default:
		return nil, fmt.Errorf("totp: unsupported algorithm %q (sha1, sha256, sha512)", alg)
	}
}

func decodeSecret(s string) ([]byte, error) {
	s = strings.ToUpper(s)
	if n := len(s) % 8; n != 0 {
		s += strings.Repeat("=", 8-n)
	}
	raw, err := base32.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("totp: decode secret: %w", err)
	}
	return raw, nil
}

func recToInfo(rec *keyRecord) KeyInfo {
	return KeyInfo{
		Name:      rec.Name,
		Issuer:    rec.Issuer,
		Account:   rec.Account,
		Algorithm: rec.Algorithm,
		Digits:    rec.Digits,
		Period:    rec.Period,
		Skew:      rec.Skew,
		URL:       otpauthURL(rec),
	}
}

func otpauthURL(rec *keyRecord) string {
	label := url.PathEscape(rec.Issuer + ":" + rec.Account)
	q := url.Values{}
	q.Set("secret", rec.Secret)
	q.Set("issuer", rec.Issuer)
	q.Set("algorithm", strings.ToUpper(rec.Algorithm))
	q.Set("digits", fmt.Sprintf("%d", rec.Digits))
	q.Set("period", fmt.Sprintf("%d", rec.Period))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// --- barrier helpers ---

func (m *Manager) loadKey(ctx context.Context, name string) (*keyRecord, error) {
	var rec keyRecord
	if err := m.get(ctx, keysPrefix+name, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func (m *Manager) get(ctx context.Context, key string, dst interface{}) error {
	e, err := m.b.Get(ctx, key)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotFound
	}
	return json.Unmarshal(e.Value, dst)
}

func (m *Manager) put(ctx context.Context, key string, src interface{}) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return m.b.Put(ctx, &physical.Entry{Key: key, Value: data})
}
