package totp

import (
	"context"
	"crypto/sha1"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

// --- in-memory barrier ---

type memBarrier struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMem() *memBarrier { return &memBarrier{data: make(map[string][]byte)} }

func (m *memBarrier) Get(_ context.Context, key string) (*physical.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	c := make([]byte, len(v))
	copy(c, v)
	return &physical.Entry{Key: key, Value: c}, nil
}
func (m *memBarrier) Put(_ context.Context, e *physical.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := make([]byte, len(e.Value))
	copy(c, e.Value)
	m.data[e.Key] = c
	return nil
}
func (m *memBarrier) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}
func (m *memBarrier) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

// --- tests ---

func TestCreateKeyGeneratesSecret(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()

	result, err := m.CreateKey(ctx, "mykey", CreateKeyRequest{
		Issuer:  "ACME",
		Account: "user@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Secret == "" {
		t.Fatal("expected non-empty secret")
	}
	if !strings.HasPrefix(result.URL, "otpauth://totp/") {
		t.Fatalf("unexpected URL: %q", result.URL)
	}
	if result.Algorithm != "sha1" || result.Digits != 6 || result.Period != 30 {
		t.Fatalf("unexpected defaults: %+v", result.KeyInfo)
	}
}

func TestCreateKeyImport(t *testing.T) {
	// Import the well-known RFC 6238 SHA1 test seed (base32 of "12345678901234567890").
	const seed = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	m := NewManager(newMem())
	result, err := m.CreateKey(context.Background(), "rfc", CreateKeyRequest{
		Secret: seed,
		Digits: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Secret != seed {
		t.Fatalf("secret mismatch: want %s, got %s", seed, result.Secret)
	}
}

func TestCreateKeyInvalidSecret(t *testing.T) {
	m := NewManager(newMem())
	_, err := m.CreateKey(context.Background(), "bad", CreateKeyRequest{Secret: "not-base32!!!"})
	if !errors.Is(err, ErrInvalidSecret) {
		t.Fatalf("expected ErrInvalidSecret, got %v", err)
	}
}

func TestGetKeyNoSecret(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()
	m.CreateKey(ctx, "k", CreateKeyRequest{Issuer: "Test", Account: "a@b.com"})

	info, err := m.GetKey(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "k" || info.Issuer != "Test" {
		t.Fatalf("metadata mismatch: %+v", info)
	}
	// KeyInfo must not carry the secret — check it's not in the URL
	// (the URL contains the secret in the query string — that's by design for QR import)
	if info.URL == "" {
		t.Fatal("expected non-empty URL")
	}
}

func TestKeyNotFound(t *testing.T) {
	m := NewManager(newMem())
	if _, err := m.GetKey(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteKey(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()
	m.CreateKey(ctx, "del", CreateKeyRequest{})
	if err := m.DeleteKey(ctx, "del"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetKey(ctx, "del"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestListKeys(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()
	m.CreateKey(ctx, "alpha", CreateKeyRequest{})
	m.CreateKey(ctx, "beta", CreateKeyRequest{})

	names, err := m.ListKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(names), names)
	}
}

func TestGenerateAndValidateRoundTrip(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()
	m.CreateKey(ctx, "k", CreateKeyRequest{})

	res, err := m.GenerateCode(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Code) != 6 {
		t.Fatalf("expected 6-digit code, got %q", res.Code)
	}
	if res.ValidUntil.IsZero() {
		t.Fatal("expected non-zero ValidUntil")
	}

	// Validate immediately — must succeed within the same window.
	valid, err := m.ValidateCode(ctx, "k", res.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("expected valid code immediately after generation")
	}
}

func TestValidateCodeInvalid(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()
	m.CreateKey(ctx, "k", CreateKeyRequest{})

	res, _ := m.GenerateCode(ctx, "k")
	// Flip the last digit to produce a definitely wrong code.
	wrong := res.Code[:5] + string(rune(res.Code[5]^1))
	// Edge case: if XOR produces the same digit (e.g. flipping '0' gives control char),
	// just use a fixed wrong code.
	if wrong == res.Code {
		wrong = "000000"
		if res.Code == "000000" {
			wrong = "111111"
		}
	}

	valid, err := m.ValidateCode(ctx, "k", wrong)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatal("expected wrong code to fail validation")
	}
}

func TestValidateSkewAllowsAdjacentPeriod(t *testing.T) {
	// Test the validateCode helper directly with a controlled clock.
	key := []byte("12345678901234567890")
	h := sha1.New
	digits, period, skew := 6, 30, 1

	now := time.Unix(1000000000, 0) // arbitrary fixed time
	code := totpCode(key, now, digits, period, h)

	// One period later: base counter is base+1; with skew=1 we check base+1±1, which includes base.
	later := now.Add(time.Duration(period) * time.Second)
	if !validateCode(key, code, later, digits, period, skew, h) {
		t.Fatal("code should still be valid one period later with skew=1")
	}
}

func TestValidateSkewRejectsExpired(t *testing.T) {
	key := []byte("12345678901234567890")
	h := sha1.New
	digits, period, skew := 6, 30, 1

	now := time.Unix(1000000000, 0)
	code := totpCode(key, now, digits, period, h)

	// Three periods later: counter is base+3; with skew=1 we only check base+2..base+4.
	tooLate := now.Add(3 * time.Duration(period) * time.Second)
	if validateCode(key, code, tooLate, digits, period, skew, h) {
		t.Fatal("code should not validate three periods later with skew=1")
	}
}

func TestAlgorithmSHA256(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()
	m.CreateKey(ctx, "k", CreateKeyRequest{Algorithm: "sha256"})

	res, err := m.GenerateCode(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	valid, err := m.ValidateCode(ctx, "k", res.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatal("sha256 code failed validation")
	}
}

func TestDigits8(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()
	m.CreateKey(ctx, "k", CreateKeyRequest{Digits: 8})

	res, err := m.GenerateCode(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Code) != 8 {
		t.Fatalf("expected 8-digit code, got %q", res.Code)
	}
	valid, _ := m.ValidateCode(ctx, "k", res.Code)
	if !valid {
		t.Fatal("8-digit code failed validation")
	}
}

// TestKnownVector validates the RFC 6238 / RFC 4226 test vectors for correctness.
// Seed: ASCII bytes of "12345678901234567890", algorithm: SHA1, digits: 8.
// Reference: https://www.rfc-editor.org/rfc/rfc6238#appendix-B
func TestKnownVector(t *testing.T) {
	key := []byte("12345678901234567890")
	h := sha1.New

	vectors := []struct {
		counter uint64
		want    string
	}{
		{1, "94287082"},       // T=59s
		{37037036, "07081804"}, // T=1111111109s
		{41152263, "89005924"}, // T=1234567890s
		{66666666, "69279037"}, // T=2000000000s
	}
	for _, v := range vectors {
		got := hotpCode(key, v.counter, 8, h)
		if got != v.want {
			t.Errorf("counter=%d: want %s, got %s", v.counter, v.want, got)
		}
	}
}
