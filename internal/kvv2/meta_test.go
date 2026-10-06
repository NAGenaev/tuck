package kvv2

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

func ptr[T any](v T) *T { return &v }

func TestAuthorRecorded(t *testing.T) {
	s := New(newMem())
	ctx := WithAuthor(context.Background(), Author{DisplayName: "alice", Accessor: "acc123", Fingerprint: "ab12cd34ef56"})
	if _, err := s.Write(ctx, "k", []byte("v"), nil); err != nil {
		t.Fatal(err)
	}
	// A write without an author in ctx is allowed and leaves it empty.
	if _, err := s.Write(context.Background(), "k", []byte("v2"), nil); err != nil {
		t.Fatal(err)
	}
	m, _ := s.GetMeta(ctx, "k")
	if got := m.Versions[1].CreatedBy.Label(); got != "alice" {
		t.Fatalf("v1 author = %q", got)
	}
	if m.Versions[1].CreatedBy.Accessor != "acc123" || m.Versions[1].CreatedBy.Fingerprint != "ab12cd34ef56" {
		t.Fatalf("author = %+v", m.Versions[1].CreatedBy)
	}
	if m.Versions[2].CreatedBy != nil || m.Versions[2].CreatedBy.Label() != "" {
		t.Fatalf("v2 should have no author: %+v", m.Versions[2].CreatedBy)
	}
	if m.CreatedAt.IsZero() || m.UpdatedAt.Before(m.CreatedAt) {
		t.Fatalf("timestamps: created=%v updated=%v", m.CreatedAt, m.UpdatedAt)
	}
	// Label fallbacks.
	if (&Author{Accessor: "a", Fingerprint: "f"}).Label() != "a" || (&Author{Fingerprint: "f"}).Label() != "f" {
		t.Fatal("label fallback broken")
	}
}

// Entries stored before authorship / oldest_version / timestamps / cas existed
// must still load and keep working.
func TestLegacyMetadataLoads(t *testing.T) {
	mem := newMem()
	old := `{"current_version":3,"max_versions":10,"versions":{` +
		`"1":{"version":1,"created_at":"2024-01-02T03:04:05Z","destroyed":true},` +
		`"2":{"version":2,"created_at":"2024-02-02T03:04:05Z","destroyed":false},` +
		`"3":{"version":3,"created_at":"2024-03-02T03:04:05Z","destroyed":false}}}`
	mem.data[metaKey("legacy")] = []byte(old)
	mem.data[dataKey("legacy", 3)] = []byte("val3")

	s := New(mem)
	ctx := context.Background()
	m, err := s.GetMeta(ctx, "legacy")
	if err != nil || m == nil {
		t.Fatalf("GetMeta: %v %v", m, err)
	}
	if m.OldestVersion != 1 {
		t.Errorf("OldestVersion = %d, want 1", m.OldestVersion)
	}
	if m.CASRequired || len(m.CustomMetadata) != 0 {
		t.Errorf("unexpected defaults: %+v", m)
	}
	if m.CreatedAt.Format(time.RFC3339) != "2024-01-02T03:04:05Z" || m.UpdatedAt.Format(time.RFC3339) != "2024-03-02T03:04:05Z" {
		t.Errorf("derived timestamps: %v %v", m.CreatedAt, m.UpdatedAt)
	}
	if m.Versions[3].CreatedBy != nil || m.Versions[3].CreatedBy.Label() != "" {
		t.Errorf("legacy version must have no author")
	}
	val, _, err := s.Read(ctx, "legacy", 0)
	if err != nil || string(val) != "val3" {
		t.Fatalf("read legacy: %q %v", val, err)
	}
	if v, err := s.Write(ctx, "legacy", []byte("val4"), nil); err != nil || v != 4 {
		t.Fatalf("write on legacy: %d %v", v, err)
	}
	// And the rewritten entry round-trips.
	var raw KVMeta
	if err := json.Unmarshal(mem.data[metaKey("legacy")], &raw); err != nil || raw.CurrentVersion != 4 {
		t.Fatalf("stored: %+v %v", raw, err)
	}
}

func TestLegacyNullVersionEntry(t *testing.T) {
	mem := newMem()
	mem.data[metaKey("k")] = []byte(`{"current_version":2,"versions":{"1":null,"2":{"version":2}}}`)
	s := New(mem)
	m, err := s.GetMeta(context.Background(), "k")
	if err != nil || m.Versions[1] != nil && len(m.Versions) != 1 {
		t.Fatalf("null version entry not dropped: %+v %v", m, err)
	}
}

func TestCASRequiredPerSecret(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()
	if _, err := s.UpdateMetadata(ctx, "k", MetaUpdate{CASRequired: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write(ctx, "k", []byte("v"), nil); !errors.Is(err, ErrCASRequired) {
		t.Fatalf("write without cas: %v", err)
	}
	zero := 0
	if v, err := s.Write(ctx, "k", []byte("v"), &zero); err != nil || v != 1 {
		t.Fatalf("cas=0 on new secret: %d %v", v, err)
	}
	bad := 5
	if _, err := s.Write(ctx, "k", []byte("x"), &bad); !errors.Is(err, ErrCASMismatch) {
		t.Fatalf("bad cas: %v", err)
	}
	if !strings.Contains(err2s(s.Write(ctx, "k", []byte("x"), &bad)), "expected version 5, current is 1") {
		t.Fatal("mismatch message changed")
	}
	one := 1
	if v, err := s.Write(ctx, "k", []byte("v2"), &one); err != nil || v != 2 {
		t.Fatalf("good cas: %d %v", v, err)
	}
	// Turning the flag off re-allows plain writes.
	_, _ = s.UpdateMetadata(ctx, "k", MetaUpdate{CASRequired: ptr(false)})
	if _, err := s.Write(ctx, "k", []byte("v3"), nil); err != nil {
		t.Fatalf("plain write after disabling: %v", err)
	}
}

func err2s(_ int, err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestCASRequiredByMount(t *testing.T) {
	def := Defaults{CASRequired: true, MaxVersions: 3}
	s := New(newMem()).WithDefaults(func(context.Context) Defaults { return def })
	ctx := context.Background()
	if _, err := s.Write(ctx, "a", []byte("v"), nil); !errors.Is(err, ErrCASRequired) {
		t.Fatalf("mount cas_required not enforced: %v", err)
	}
	zero := 0
	if _, err := s.Write(ctx, "a", []byte("v"), &zero); err != nil {
		t.Fatal(err)
	}
	m, _ := s.GetMeta(ctx, "a")
	if m.MaxVersions != 3 {
		t.Fatalf("mount max_versions should seed new secrets, got %d", m.MaxVersions)
	}
	// Lifting the mount option takes effect immediately.
	def = Defaults{}
	if _, err := s.Write(ctx, "a", []byte("v"), nil); err != nil {
		t.Fatalf("after lifting mount option: %v", err)
	}
}

func TestEvictionReportsOldestVersion(t *testing.T) {
	mem := newMem()
	s := New(mem)
	ctx := context.Background()
	if _, err := s.UpdateMetadata(ctx, "k", MetaUpdate{MaxVersions: ptr(3)}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		_, _ = s.Write(ctx, "k", []byte("v"), nil)
	}
	m, _ := s.GetMeta(ctx, "k")
	if m.OldestVersion != 1 || len(m.Versions) != 3 {
		t.Fatalf("no eviction yet: oldest=%d n=%d", m.OldestVersion, len(m.Versions))
	}
	for i := 4; i <= 6; i++ {
		_, _ = s.Write(ctx, "k", []byte("v"), nil)
	}
	m, _ = s.GetMeta(ctx, "k")
	if m.CurrentVersion != 6 || m.OldestVersion != 4 || len(m.Versions) != 3 {
		t.Fatalf("after eviction: cur=%d oldest=%d n=%d", m.CurrentVersion, m.OldestVersion, len(m.Versions))
	}
	for v := 1; v <= 3; v++ {
		if _, ok := mem.data[dataKey("k", v)]; ok {
			t.Errorf("data of evicted version %d still stored", v)
		}
		if _, _, err := s.Read(ctx, "k", v); err == nil {
			t.Errorf("version %d should be gone", v)
		}
	}
	// A lowered cap evicts all excess versions on the next write, not just one.
	_, _ = s.UpdateMetadata(ctx, "k", MetaUpdate{MaxVersions: ptr(1)})
	_, _ = s.Write(ctx, "k", []byte("v"), nil)
	m, _ = s.GetMeta(ctx, "k")
	if m.OldestVersion != 7 || len(m.Versions) != 1 {
		t.Fatalf("after lowering cap: oldest=%d n=%d", m.OldestVersion, len(m.Versions))
	}
	// 0 = unlimited.
	_, _ = s.UpdateMetadata(ctx, "k", MetaUpdate{MaxVersions: ptr(0)})
	for i := 0; i < 15; i++ {
		_, _ = s.Write(ctx, "k", []byte("v"), nil)
	}
	m, _ = s.GetMeta(ctx, "k")
	if m.OldestVersion != 7 || len(m.Versions) != 16 {
		t.Fatalf("unlimited: oldest=%d n=%d", m.OldestVersion, len(m.Versions))
	}
}

func TestCustomMetadata(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()
	str := func(v string) *string { return &v }

	// Metadata can exist before the first write.
	m, err := s.UpdateMetadata(ctx, "k", MetaUpdate{CustomMetadata: map[string]*string{"owner": str("team-a"), "ticket": str("SEC-1")}, Replace: true})
	if err != nil || m.CurrentVersion != 0 || m.CustomMetadata["owner"] != "team-a" {
		t.Fatalf("pre-write metadata: %+v %v", m, err)
	}
	if v, _, err := s.Read(ctx, "k", 0); v != nil || err != nil {
		t.Fatalf("path without versions must read as absent: %v %v", v, err)
	}
	if v, err := s.Write(ctx, "k", []byte("v"), nil); err != nil || v != 1 {
		t.Fatalf("write: %d %v", v, err)
	}
	// Merge: set one, delete one.
	m, err = s.UpdateMetadata(ctx, "k", MetaUpdate{CustomMetadata: map[string]*string{"owner": str("team-b"), "ticket": nil}})
	if err != nil || len(m.CustomMetadata) != 1 || m.CustomMetadata["owner"] != "team-b" {
		t.Fatalf("merge: %+v %v", m.CustomMetadata, err)
	}
	// Writing a secret keeps custom metadata.
	_, _ = s.Write(ctx, "k", []byte("v2"), nil)
	m, _ = s.GetMeta(ctx, "k")
	if m.CustomMetadata["owner"] != "team-b" {
		t.Fatal("custom metadata lost on write")
	}
	// Untouched when the map is nil; replaced (cleared) when empty.
	_, _ = s.UpdateMetadata(ctx, "k", MetaUpdate{MaxVersions: ptr(4)})
	m, _ = s.GetMeta(ctx, "k")
	if m.CustomMetadata["owner"] != "team-b" || m.MaxVersions != 4 {
		t.Fatalf("partial update clobbered: %+v", m)
	}
	_, _ = s.UpdateMetadata(ctx, "k", MetaUpdate{CustomMetadata: map[string]*string{}, Replace: true})
	m, _ = s.GetMeta(ctx, "k")
	if len(m.CustomMetadata) != 0 {
		t.Fatalf("replace with {} should clear: %+v", m.CustomMetadata)
	}
}

func TestCustomMetadataLimits(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()
	mk := func(n int) map[string]*string {
		m := map[string]*string{}
		for i := 0; i < n; i++ {
			v := "x"
			m[strings.Repeat("k", 1)+string(rune('a'+i%26))+strings.Repeat("0", i/26)] = &v
		}
		return m
	}
	if _, err := s.UpdateMetadata(ctx, "k", MetaUpdate{CustomMetadata: mk(MaxCustomMetadataPairs), Replace: true}); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	if _, err := s.UpdateMetadata(ctx, "k", MetaUpdate{CustomMetadata: mk(MaxCustomMetadataPairs + 1), Replace: true}); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("too many pairs: %v", err)
	}
	bad := map[string]map[string]*string{
		"empty key":    {"": ptr("v")},
		"long key":     {strings.Repeat("k", MaxCustomMetadataKey+1): ptr("v")},
		"long value":   {"k": ptr(strings.Repeat("v", MaxCustomMetadataValue+1))},
		"control char": {"k": ptr("a\x00b")},
		"bad utf8":     {"k": ptr("\xff\xfe")},
	}
	for name, cm := range bad {
		if _, err := s.UpdateMetadata(ctx, "k2", MetaUpdate{CustomMetadata: cm, Replace: true}); !errors.Is(err, ErrInvalidMetadata) {
			t.Errorf("%s: expected ErrInvalidMetadata, got %v", name, err)
		}
	}
	if m, _ := s.GetMeta(ctx, "k2"); m != nil {
		t.Error("a rejected update must not create the metadata record")
	}
	for _, n := range []int{-1, MaxVersionsLimit + 1} {
		if _, err := s.UpdateMetadata(ctx, "k3", MetaUpdate{MaxVersions: ptr(n)}); !errors.Is(err, ErrInvalidMetadata) {
			t.Errorf("max_versions=%d: expected ErrInvalidMetadata, got %v", n, err)
		}
	}
}

func TestListDetailed(t *testing.T) {
	s := New(newMem())
	ctx := WithAuthor(context.Background(), Author{DisplayName: "bob"})
	_, _ = s.Write(ctx, "app/db", []byte("SECRET-DB"), nil)
	_, _ = s.Write(ctx, "app/db", []byte("SECRET-DB-2"), nil)
	_, _ = s.Write(ctx, "app/cache", []byte("c"), nil)
	_, _ = s.Write(ctx, "app/team/token", []byte("t"), nil)
	_, _ = s.Write(ctx, "app/team/deep/x", []byte("x"), nil)
	_, _ = s.Write(ctx, "top", []byte("t"), nil)
	_ = s.SoftDelete(ctx, "app/cache", []int{1})
	_ = s.SoftDelete(ctx, "app/db", []int{1})
	_ = s.Destroy(ctx, "top", []int{1})
	_, _ = s.UpdateMetadata(ctx, "app/db", MetaUpdate{CASRequired: ptr(true)})

	rows, tot, err := s.ListDetailed(ctx, "app/", ListOptions{})
	if err != nil || tot != nil {
		t.Fatalf("err=%v totals=%v", err, tot)
	}
	byName := map[string]ListEntry{}
	var names []string
	for _, r := range rows {
		byName[r.Name] = r
		names = append(names, r.Name)
	}
	if strings.Join(names, ",") != "cache,db,team/" {
		t.Fatalf("names: %v", names)
	}
	if db := byName["db"]; db.CurrentVersion != 2 || db.Versions != 2 || db.UpdatedBy != "bob" || db.UpdatedAt.IsZero() || db.Deleted || !db.CASRequired {
		t.Fatalf("db: %+v", db)
	}
	if c := byName["cache"]; c.CurrentVersion != 1 || !c.Deleted {
		t.Fatalf("cache: %+v", c)
	}
	if f := byName["team/"]; !f.Folder || f.Count != 2 {
		t.Fatalf("folder: %+v", f)
	}

	// Values never leak: the listing has no field that could carry them.
	raw, _ := json.Marshal(rows)
	if strings.Contains(string(raw), "SECRET-DB") {
		t.Fatal("secret value in listing")
	}

	_, tot, err = s.ListDetailed(ctx, "", ListOptions{Totals: true})
	if err != nil || tot == nil {
		t.Fatal(err)
	}
	want := Totals{Secrets: 5, Folders: 3, Versions: 6, Deleted: 2, Destroyed: 1}
	if *tot != want {
		t.Fatalf("totals = %+v, want %+v", *tot, want)
	}
}

func TestListDetailedAllowFilter(t *testing.T) {
	s := New(newMem())
	ctx := context.Background()
	_, _ = s.Write(ctx, "a/open", []byte("1"), nil)
	_, _ = s.Write(ctx, "a/closed", []byte("1"), nil)
	_, _ = s.Write(ctx, "a/sub/closed2", []byte("1"), nil)
	allow := func(p string) bool { return p == "a/open" }
	rows, tot, err := s.ListDetailed(ctx, "a/", ListOptions{Totals: true, Allow: allow})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		switch r.Name {
		case "open":
			if r.Restricted || r.CurrentVersion != 1 {
				t.Errorf("open: %+v", r)
			}
		case "closed":
			if !r.Restricted || r.CurrentVersion != 0 || !r.UpdatedAt.IsZero() {
				t.Errorf("closed must carry no metadata: %+v", r)
			}
		}
	}
	if tot.Secrets != 1 || tot.Versions != 1 || tot.Hidden != 2 {
		t.Fatalf("totals = %+v", *tot)
	}
}

func TestConcurrentWritesAreSerialised(t *testing.T) {
	s := New(&lockedMem{memBarrier: newMem()})
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.Write(ctx, "k", []byte("v"), nil)
		}()
	}
	wg.Wait()
	m, _ := s.GetMeta(ctx, "k")
	if m.CurrentVersion != 20 {
		t.Fatalf("lost writes: current_version = %d, want 20", m.CurrentVersion)
	}
}

// Only one of several writers carrying the same cas may win.
func TestConcurrentCASSingleWinner(t *testing.T) {
	s := New(&lockedMem{memBarrier: newMem()})
	ctx := context.Background()
	_, _ = s.Write(ctx, "k", []byte("v"), nil)
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cas := 1
			if _, err := s.Write(ctx, "k", []byte("v"), &cas); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("%d writers won the same CAS", wins)
	}
}

// lockedMem makes the test barrier safe for concurrent use (the real barrier is).
type lockedMem struct {
	*memBarrier
	mu sync.Mutex
}

func (l *lockedMem) Get(c context.Context, k string) (*physical.Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.memBarrier.Get(c, k)
}
func (l *lockedMem) Put(c context.Context, e *physical.Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.memBarrier.Put(c, e)
}
func (l *lockedMem) Delete(c context.Context, k string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.memBarrier.Delete(c, k)
}
func (l *lockedMem) List(c context.Context, p string) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.memBarrier.List(c, p)
}
