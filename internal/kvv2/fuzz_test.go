package kvv2

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/NAGenaev/tuck/internal/physical"
)

// FuzzMetaJSON verifies that unmarshalling arbitrary JSON bytes into KVMeta and
// accessing all fields never panics. This guards the metadata parsing path that
// runs on every Read/Write operation.
func FuzzMetaJSON(f *testing.F) {
	valid, _ := json.Marshal(&KVMeta{
		CurrentVersion: 1,
		MaxVersions:    10,
		Versions:       map[int]*VerMeta{1: {Version: 1}},
	})
	f.Add(valid)
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"current_version":-1,"max_versions":0,"versions":{}}`))
	f.Add([]byte(`{"current_version":2147483647,"versions":{"999":null,"0":{"destroyed":true}}}`))
	f.Add([]byte(`{"versions":{"not-a-number":{"version":1}}}`))
	f.Add([]byte{0x00, 0xff})
	f.Add([]byte{})
	f.Add([]byte(`[]`)) // wrong JSON type

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("KVMeta unmarshal panicked on %x: %v", data, r)
			}
		}()
		var m KVMeta
		_ = json.Unmarshal(data, &m)
		// Access all fields — a nil-pointer or index-out-of-range panic here
		// would indicate a bug in the struct or downstream consumers.
		_ = m.CurrentVersion
		_ = m.MaxVersions
		for _, v := range m.Versions {
			if v != nil {
				_ = v.Version
				_ = v.Destroyed
				_ = v.CreatedAt
				_ = v.DeletedAt
			}
		}
	})
}

// FuzzWrite verifies that Store.Write never panics on arbitrary path and value inputs,
// including empty paths, binary values, very long paths, and out-of-range CAS values.
func FuzzWrite(f *testing.F) {
	f.Add("myapp/db", []byte(`{"host":"localhost"}`), 0, false)
	f.Add("", []byte{}, 0, false)
	f.Add("/leading-slash", []byte("v"), 0, false)
	f.Add("a/b/c/d/e/f/g/h", []byte{0x00, 0xff}, 0, false)
	f.Add(string(make([]byte, 512)), []byte("value"), 0, false)
	f.Add("k", []byte{}, 1, true)         // CAS=1 on empty store → mismatch
	f.Add("k", []byte("x"), 0, true)      // CAS=0 on empty store → ok (first write)
	f.Add("k", []byte("x"), -1, true)     // negative CAS

	f.Fuzz(func(t *testing.T, p string, value []byte, cas int, useCAS bool) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Write panicked on path=%q cas=%d: %v", p, cas, r)
			}
		}()
		s := New(physical.NewInMem())
		ctx := context.Background()
		var casPtr *int
		if useCAS {
			casPtr = &cas
		}
		_, _ = s.Write(ctx, p, value, casPtr)
	})
}

// FuzzReadAfterWrite verifies the invariant: data written with Write is returned
// unchanged by Read for all valid paths and payloads.
func FuzzReadAfterWrite(f *testing.F) {
	f.Add("myapp/key", []byte(`{"password":"s3cr3t"}`))
	f.Add("a", []byte{})
	f.Add("x/y/z", []byte{0x00, 0x01, 0xff})
	f.Add("key", make([]byte, 4096))

	f.Fuzz(func(t *testing.T, p string, value []byte) {
		if p == "" {
			return // empty path is rejected by the store — not the invariant we're testing
		}
		s := New(physical.NewInMem())
		ctx := context.Background()

		ver, err := s.Write(ctx, p, value, nil)
		if err != nil {
			return // skip paths rejected by the store
		}

		got, meta, err := s.Read(ctx, p, ver)
		if err != nil {
			t.Fatalf("Read after Write(%q): %v", p, err)
		}
		if meta == nil || meta.Destroyed {
			t.Errorf("Read(%q, v%d): version is nil or destroyed after Write", p, ver)
		}
		if string(got) != string(value) {
			t.Errorf("Read(%q, v%d): got %x, want %x", p, ver, got, value)
		}
	})
}
