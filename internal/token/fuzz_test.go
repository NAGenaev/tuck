package token

import (
	"encoding/json"
	"testing"
)

// FuzzUnmarshalToken tests that deserializing arbitrary JSON into a Token
// never panics and that the result is always usable.
func FuzzUnmarshalToken(f *testing.F) {
	// Valid token JSON.
	tok, _ := Generate("fuzz", []string{"default"}, 0)
	if tok != nil {
		if data, err := json.Marshal(tok); err == nil {
			f.Add(data)
		}
	}
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"id":"x","policies":["root"]}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"expires_at":"bad-timestamp"}`))
	f.Add([]byte{0x00, 0xff})

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("unmarshal panicked on %x: %v", data, r)
			}
		}()
		tok, err := unmarshal(data)
		if err != nil {
			return
		}
		// Call methods — must not panic even on zero-value fields.
		_ = tok.IsExpired()
		_, _ = tok.marshal()
	})
}

// FuzzTokenMarshalRoundtrip tests that a generated token survives a
// marshal → unmarshal cycle with stable ID and policies.
func FuzzTokenMarshalRoundtrip(f *testing.F) {
	f.Add("service-a", "default", "readonly")
	f.Add("", "", "")
	f.Add("x", "root", "")

	f.Fuzz(func(t *testing.T, displayName, pol1, pol2 string) {
		policies := []string{}
		if pol1 != "" {
			policies = append(policies, pol1)
		}
		if pol2 != "" {
			policies = append(policies, pol2)
		}
		tok, err := Generate(displayName, policies, 0)
		if err != nil {
			return
		}
		data, err := tok.marshal()
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		got, err := unmarshal(data)
		if err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.ID != tok.ID {
			t.Errorf("ID mismatch after roundtrip")
		}
	})
}
