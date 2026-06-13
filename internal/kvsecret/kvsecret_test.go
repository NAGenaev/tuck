package kvsecret

import (
	"testing"
	"time"
)

func TestNew_noTTL(t *testing.T) {
	e := New([]byte("hello"), nil, 0)
	if e.IsExpired() {
		t.Fatal("entry without TTL must not be expired")
	}
	if e.CreatedAt.IsZero() {
		t.Fatal("CreatedAt must be set")
	}
	if !e.ExpiresAt.IsZero() {
		t.Fatal("ExpiresAt must be zero when ttl=0")
	}
}

func TestNew_withTTL(t *testing.T) {
	e := New([]byte("hi"), map[string]string{"env": "prod"}, 5*time.Second)
	if e.IsExpired() {
		t.Fatal("freshly created entry must not be expired")
	}
	if e.ExpiresAt.IsZero() {
		t.Fatal("ExpiresAt must be set when ttl>0")
	}
	if e.Metadata["env"] != "prod" {
		t.Fatalf("unexpected metadata: %v", e.Metadata)
	}
}

func TestIsExpired(t *testing.T) {
	e := New([]byte("x"), nil, time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if !e.IsExpired() {
		t.Fatal("entry must be expired after TTL elapses")
	}
}

func TestMarshalUnmarshal_roundtrip(t *testing.T) {
	orig := New([]byte("secret-value"), map[string]string{"k": "v"}, time.Hour)
	data, err := orig.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got := Unmarshal(data)
	if string(got.Value) != "secret-value" {
		t.Fatalf("value mismatch: %q", got.Value)
	}
	if got.Metadata["k"] != "v" {
		t.Fatalf("metadata mismatch: %v", got.Metadata)
	}
	if got.ExpiresAt.IsZero() {
		t.Fatal("ExpiresAt must survive roundtrip")
	}
}

func TestUnmarshal_legacyBytes(t *testing.T) {
	// Raw bytes that are not a JSON Entry must be treated as the legacy value.
	raw := []byte("not json at all")
	e := Unmarshal(raw)
	if string(e.Value) != "not json at all" {
		t.Fatalf("expected raw bytes as value, got %q", e.Value)
	}
}

func TestUnmarshal_jsonWithoutCreatedAt(t *testing.T) {
	// Valid JSON but missing created_at → treated as legacy.
	raw := []byte(`{"value":"aGVsbG8="}`) // base64 "hello"
	e := Unmarshal(raw)
	// Falls back to raw bytes since CreatedAt is zero.
	if string(e.Value) != string(raw) {
		t.Fatalf("expected fallback to raw bytes, got %q", e.Value)
	}
}
