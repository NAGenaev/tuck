package kvsecret

import "testing"

// FuzzUnmarshal tests that Unmarshal never panics on arbitrary bytes,
// including the legacy raw-bytes fallback path and the JSON envelope path.
func FuzzUnmarshal(f *testing.F) {
	// Valid JSON envelope.
	f.Add([]byte(`{"value":"aGVsbG8=","created_at":"2024-01-01T00:00:00Z"}`))
	// Legacy raw bytes.
	f.Add([]byte("raw secret value"))
	// Empty.
	f.Add([]byte{})
	// Null JSON.
	f.Add([]byte("null"))
	// Truncated JSON.
	f.Add([]byte(`{"value":`))
	// Binary garbage.
	f.Add([]byte{0x00, 0xff, 0xfe, 0xab, 0xcd})

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Unmarshal panicked on input %x: %v", data, r)
			}
		}()
		entry := Unmarshal(data)
		if entry == nil {
			t.Error("Unmarshal returned nil — must always return a non-nil Entry")
		}
	})
}
