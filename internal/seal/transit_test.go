package seal_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/NAGenaev/tuck/internal/seal"
)

// fakeTransit is a minimal in-memory implementation of the Vault Transit
// encrypt/decrypt API, used for testing TransitSeal without a real Vault.
type fakeTransit struct {
	// map from ciphertext -> plaintext (base64-encoded)
	store map[string]string
}

func newFakeTransit() *fakeTransit {
	return &fakeTransit{store: make(map[string]string)}
}

func (f *fakeTransit) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/transit/encrypt/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Plaintext string `json:"plaintext"`
		}
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		// Use a simple reversible "encryption": prefix with "ct:"
		ct := "vault:v1:" + base64.RawURLEncoding.EncodeToString([]byte("ct:"+req.Plaintext))
		f.store[ct] = req.Plaintext
		resp := map[string]any{
			"data": map[string]string{"ciphertext": ct},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
	mux.HandleFunc("/v1/transit/decrypt/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ciphertext string `json:"ciphertext"`
		}
		json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
		pt, ok := f.store[req.Ciphertext]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"errors": []string{"unknown ciphertext"},
			})
			return
		}
		resp := map[string]any{
			"data": map[string]string{"plaintext": pt},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
	return mux
}

// TestTransitSeal_InitUnsealRoundTrip verifies that TransitSeal.Init() and
// Unseal() produce the same root key via the fake Transit service.
func TestTransitSeal_InitUnsealRoundTrip(t *testing.T) {
	fake := newFakeTransit()
	ts := httptest.NewServer(fake.handler())
	defer ts.Close()

	dir := t.TempDir()
	wrappedPath := filepath.Join(dir, "wrapped.key")

	s := seal.NewTransit(ts.URL, "tuck-seal", "fake-token", wrappedPath)

	result, err := s.Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(result.RootKey) != 32 {
		t.Errorf("RootKey length = %d, want 32", len(result.RootKey))
	}
	if result.Shares != nil {
		t.Error("TransitSeal.Init() should not return Shares")
	}

	// Unseal should recover the same key.
	recovered, err := s.Unseal()
	if err != nil {
		t.Fatalf("Unseal: %v", err)
	}
	for i, b := range result.RootKey {
		if recovered[i] != b {
			t.Errorf("recovered key byte[%d] = %02x, want %02x", i, recovered[i], b)
			break
		}
	}
}

// TestTransitSeal_TypeIsTransit verifies the Type() method.
func TestTransitSeal_TypeIsTransit(t *testing.T) {
	s := seal.NewTransit("http://localhost", "key", "tok", "/tmp/w")
	if s.Type() != "transit" {
		t.Errorf("Type() = %q, want \"transit\"", s.Type())
	}
}

// TestTransitSeal_UnsealMissingFile verifies that Unseal errors gracefully
// when the wrapped key file does not exist.
func TestTransitSeal_UnsealMissingFile(t *testing.T) {
	s := seal.NewTransit("http://localhost", "key", "tok", "/nonexistent/path/wrapped.key")
	if _, err := s.Unseal(); err == nil {
		t.Error("expected error for missing wrapped key file, got nil")
	}
}
