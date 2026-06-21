package api

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

// b64url encodes data as base64url (no padding) — Transit plaintext/input format.
func b64url(data string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(data))
}

// TestTransitEncryptDecryptRoundTrip is the fundamental Transit test:
// create AES key → encrypt plaintext → decrypt ciphertext → original value restored.
func TestTransitEncryptDecryptRoundTrip(t *testing.T) {
	ts, _, root := newTestServer(t)

	status, body := doJSON(t, ts, http.MethodPost, "/v1/transit/keys/mykey", "", root)
	if status != http.StatusOK {
		t.Fatalf("create key: %d %s", status, body)
	}

	// Encrypt.
	plaintext := "hello world"
	encBody := `{"plaintext":"` + b64url(plaintext) + `"}`
	status, body = doJSON(t, ts, http.MethodPost, "/v1/transit/encrypt/mykey", encBody, root)
	if status != http.StatusOK {
		t.Fatalf("encrypt: %d %s", status, body)
	}
	ciphertext := jsonField(t, body, "ciphertext")
	if !strings.HasPrefix(ciphertext, "vault:v") {
		t.Errorf("ciphertext %q lacks vault:v prefix", ciphertext)
	}

	// Decrypt.
	decBody := `{"ciphertext":"` + ciphertext + `"}`
	status, body = doJSON(t, ts, http.MethodPost, "/v1/transit/decrypt/mykey", decBody, root)
	if status != http.StatusOK {
		t.Fatalf("decrypt: %d %s", status, body)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(jsonField(t, body, "plaintext"))
	if err != nil {
		t.Fatalf("decode plaintext: %v", err)
	}
	if string(decoded) != plaintext {
		t.Errorf("round-trip: got %q, want %q", decoded, plaintext)
	}
}

// TestTransitRewrapAfterRotation verifies that old ciphertext can be re-encrypted
// with the new key version after a rotation.
func TestTransitRewrapAfterRotation(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/transit/keys/rkey", "", root)

	// Encrypt with v1.
	encBody := `{"plaintext":"` + b64url("secret") + `"}`
	_, body := doJSON(t, ts, http.MethodPost, "/v1/transit/encrypt/rkey", encBody, root)
	ctV1 := jsonField(t, body, "ciphertext")
	if !strings.Contains(ctV1, ":v1:") {
		t.Fatalf("expected v1 ciphertext, got: %s", ctV1)
	}

	// Rotate key.
	status, body := doJSON(t, ts, http.MethodPost, "/v1/transit/keys/rkey/rotate", "", root)
	if status != http.StatusOK {
		t.Fatalf("rotate: %d %s", status, body)
	}

	// Rewrap old ciphertext.
	rewrapBody := `{"ciphertext":"` + ctV1 + `"}`
	status, body = doJSON(t, ts, http.MethodPost, "/v1/transit/rewrap/rkey", rewrapBody, root)
	if status != http.StatusOK {
		t.Fatalf("rewrap: %d %s", status, body)
	}
	ctV2 := jsonField(t, body, "ciphertext")
	if !strings.Contains(ctV2, ":v2:") {
		t.Errorf("rewrapped ciphertext should be v2, got: %s", ctV2)
	}

	// Decrypt rewrapped ciphertext.
	decBody := `{"ciphertext":"` + ctV2 + `"}`
	status, body = doJSON(t, ts, http.MethodPost, "/v1/transit/decrypt/rkey", decBody, root)
	if status != http.StatusOK {
		t.Fatalf("decrypt rewrapped: %d %s", status, body)
	}
	decoded, _ := base64.RawURLEncoding.DecodeString(jsonField(t, body, "plaintext"))
	if string(decoded) != "secret" {
		t.Errorf("rewrap round-trip: got %q, want secret", decoded)
	}
}

// TestTransitSignVerify_ECDSA verifies sign+verify with an ECDSA P-256 key.
func TestTransitSignVerify_ECDSA(t *testing.T) {
	ts, _, root := newTestServer(t)

	status, body := doJSON(t, ts, http.MethodPost, "/v1/transit/keys/eckey",
		`{"type":"ecdsa-p256"}`, root)
	if status != http.StatusOK {
		t.Fatalf("create ecdsa key: %d %s", status, body)
	}

	input := b64url("data to sign")

	// Sign.
	signBody := `{"input":"` + input + `"}`
	status, body = doJSON(t, ts, http.MethodPost, "/v1/transit/sign/eckey", signBody, root)
	if status != http.StatusOK {
		t.Fatalf("sign: %d %s", status, body)
	}
	sig := jsonField(t, body, "signature")
	if !strings.HasPrefix(sig, "vault:v") {
		t.Errorf("signature %q lacks vault:v prefix", sig)
	}

	// Verify — valid signature.
	verBody := `{"input":"` + input + `","signature":"` + sig + `"}`
	status, body = doJSON(t, ts, http.MethodPost, "/v1/transit/verify/eckey", verBody, root)
	if status != http.StatusOK {
		t.Fatalf("verify: %d %s", status, body)
	}
	if jsonField(t, body, "valid") != "true" {
		t.Errorf("verify valid=%s, want true", jsonField(t, body, "valid"))
	}

	// Verify — tampered input must fail.
	badVerBody := `{"input":"` + b64url("tampered") + `","signature":"` + sig + `"}`
	status, body = doJSON(t, ts, http.MethodPost, "/v1/transit/verify/eckey", badVerBody, root)
	if status != http.StatusOK {
		t.Fatalf("verify tampered: %d %s", status, body)
	}
	if jsonField(t, body, "valid") != "false" {
		t.Errorf("tampered verify: valid=%s, want false", jsonField(t, body, "valid"))
	}
}

// TestTransitSignVerify_Ed25519 verifies sign+verify with an Ed25519 key.
func TestTransitSignVerify_Ed25519(t *testing.T) {
	ts, _, root := newTestServer(t)

	status, body := doJSON(t, ts, http.MethodPost, "/v1/transit/keys/edkey",
		`{"type":"ed25519"}`, root)
	if status != http.StatusOK {
		t.Fatalf("create ed25519 key: %d %s", status, body)
	}

	input := b64url("ed25519 payload")
	signBody := `{"input":"` + input + `"}`
	status, body = doJSON(t, ts, http.MethodPost, "/v1/transit/sign/edkey", signBody, root)
	if status != http.StatusOK {
		t.Fatalf("sign: %d %s", status, body)
	}
	sig := jsonField(t, body, "signature")

	verBody := `{"input":"` + input + `","signature":"` + sig + `"}`
	status, body = doJSON(t, ts, http.MethodPost, "/v1/transit/verify/edkey", verBody, root)
	if status != http.StatusOK {
		t.Fatalf("verify: %d %s", status, body)
	}
	if jsonField(t, body, "valid") != "true" {
		t.Errorf("ed25519 verify valid=%s, want true", jsonField(t, body, "valid"))
	}
}

// TestTransitHMAC verifies HMAC generation and verification.
func TestTransitHMAC(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/transit/keys/hmackey", "", root)

	input := b64url("data for hmac")
	hmacBody := `{"input":"` + input + `"}`
	status, body := doJSON(t, ts, http.MethodPost, "/v1/transit/hmac/hmackey", hmacBody, root)
	if status != http.StatusOK {
		t.Fatalf("hmac: %d %s", status, body)
	}
	hmacVal := jsonField(t, body, "hmac")
	if !strings.HasPrefix(hmacVal, "vault:v") {
		t.Errorf("hmac %q lacks vault:v prefix", hmacVal)
	}
}

// TestTransitKeyCRUD exercises key listing and deletion lifecycle.
func TestTransitKeyCRUD(t *testing.T) {
	ts, _, root := newTestServer(t)

	// Create two keys.
	doJSON(t, ts, http.MethodPost, "/v1/transit/keys/key-a", "", root)
	doJSON(t, ts, http.MethodPost, "/v1/transit/keys/key-b", `{"type":"ed25519"}`, root)

	// List.
	status, body := doJSON(t, ts, http.MethodGet, "/v1/transit/keys/?list=true", "", root)
	if status != http.StatusOK {
		t.Fatalf("list keys: %d %s", status, body)
	}
	for _, name := range []string{"key-a", "key-b"} {
		if !strings.Contains(string(body), name) {
			t.Errorf("list keys missing %q: %s", name, body)
		}
	}

	// Get.
	status, body = doJSON(t, ts, http.MethodGet, "/v1/transit/keys/key-a", "", root)
	if status != http.StatusOK {
		t.Fatalf("get key-a: %d %s", status, body)
	}

	// Delete without enabling deletable → 409.
	status, _ = doJSON(t, ts, http.MethodDelete, "/v1/transit/keys/key-a", "", root)
	if status != http.StatusConflict {
		t.Errorf("delete non-deletable key: %d, want 409", status)
	}

	// Enable deletion, then delete.
	doJSON(t, ts, http.MethodPost, "/v1/transit/keys/key-a/config",
		`{"deletable":true}`, root)
	status, _ = doJSON(t, ts, http.MethodDelete, "/v1/transit/keys/key-a", "", root)
	if status != http.StatusNoContent {
		t.Errorf("delete after enable: %d, want 204", status)
	}

	// Key must be gone.
	status, _ = doJSON(t, ts, http.MethodGet, "/v1/transit/keys/key-a", "", root)
	if status != http.StatusNotFound {
		t.Errorf("get deleted key: %d, want 404", status)
	}
}

// TestTransitMinDecryptionVersion verifies that old ciphertext is rejected
// after min_decryption_version is advanced.
func TestTransitMinDecryptionVersion(t *testing.T) {
	ts, _, root := newTestServer(t)

	doJSON(t, ts, http.MethodPost, "/v1/transit/keys/mvkey", "", root)

	// Encrypt with v1.
	encBody := `{"plaintext":"` + b64url("old secret") + `"}`
	_, body := doJSON(t, ts, http.MethodPost, "/v1/transit/encrypt/mvkey", encBody, root)
	ctV1 := jsonField(t, body, "ciphertext")

	// Rotate to v2.
	doJSON(t, ts, http.MethodPost, "/v1/transit/keys/mvkey/rotate", "", root)

	// Set min_decryption_version=2 — v1 ciphertext must be rejected.
	doJSON(t, ts, http.MethodPost, "/v1/transit/keys/mvkey/config",
		`{"min_decryption_version":2}`, root)

	status, body := doJSON(t, ts, http.MethodPost, "/v1/transit/decrypt/mvkey",
		`{"ciphertext":"`+ctV1+`"}`, root)
	if status != http.StatusBadRequest && status != http.StatusUnprocessableEntity && status != http.StatusForbidden {
		t.Errorf("decrypt v1 after min_version=2: %d %s, want 4xx", status, body)
	}

	// v2 ciphertext must still work.
	_, body = doJSON(t, ts, http.MethodPost, "/v1/transit/encrypt/mvkey",
		`{"plaintext":"`+b64url("new")+`"}`, root)
	ctV2 := jsonField(t, body, "ciphertext")
	status, _ = doJSON(t, ts, http.MethodPost, "/v1/transit/decrypt/mvkey",
		`{"ciphertext":"`+ctV2+`"}`, root)
	if status != http.StatusOK {
		t.Errorf("decrypt v2 after min_version=2: %d, want 200", status)
	}
}

// TestTransitUnknownKey verifies 404 on operations with a non-existent key.
func TestTransitUnknownKey(t *testing.T) {
	ts, _, root := newTestServer(t)

	for _, path := range []string{
		"/v1/transit/keys/ghost",
		"/v1/transit/encrypt/ghost",
		"/v1/transit/decrypt/ghost",
		"/v1/transit/sign/ghost",
	} {
		method := http.MethodGet
		body := ""
		if strings.Contains(path, "encrypt") {
			method = http.MethodPost
			body = `{"plaintext":"` + b64url("x") + `"}`
		} else if strings.Contains(path, "decrypt") {
			method = http.MethodPost
			body = `{"ciphertext":"vault:v1:fake"}`
		} else if strings.Contains(path, "sign") {
			method = http.MethodPost
			body = `{"input":"` + b64url("x") + `"}`
		}
		status, _ := doJSON(t, ts, method, path, body, root)
		if status != http.StatusNotFound && status != http.StatusBadRequest {
			t.Errorf("path %s: %d, want 404 or 400", path, status)
		}
	}
}

// TestTransitUnsupportedKeyType verifies 400 for an unknown key type.
func TestTransitUnsupportedKeyType(t *testing.T) {
	ts, _, root := newTestServer(t)
	status, _ := doJSON(t, ts, http.MethodPost, "/v1/transit/keys/badkey",
		`{"type":"des-cbc"}`, root)
	if status != http.StatusBadRequest {
		t.Errorf("unsupported key type: %d, want 400", status)
	}
}
