package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func mkEntry(i int, status int) Entry {
	return Entry{
		Time:        time.Now().UTC().Format(time.RFC3339Nano),
		Method:      "GET",
		Path:        fmt.Sprintf("/v1/secret/app/%d", i),
		Status:      status,
		Fingerprint: "abcdef012345",
		DurationMS:  1,
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// ── chain ────────────────────────────────────────────────────────────────────

func TestDispatcherSealsOneChain(t *testing.T) {
	var buf bytes.Buffer
	d := NewDispatcher(NewLogger(&buf))
	for i := 0; i < 5; i++ {
		d.Log(mkEntry(i, 200))
	}
	win := d.Ring().Snapshot()
	if len(win) != 5 {
		t.Fatalf("ring len = %d", len(win))
	}
	for i, e := range win {
		if e.Seq != uint64(i+1) {
			t.Fatalf("entry %d seq = %d", i, e.Seq)
		}
		if e.Operation != "read" {
			t.Fatalf("operation = %q", e.Operation)
		}
	}
	if win[0].PrevHash != GenesisHash {
		t.Fatal("first record must link to genesis")
	}
	if r := d.VerifyMemory(); !r.OK || r.Checked != 5 || !r.FromGenesis {
		t.Fatalf("memory verify = %+v", r)
	}

	// The file lines must be byte-identical to the window: ONE chain.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("file lines = %d", len(lines))
	}
	for i, ln := range lines {
		var e Entry
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Fatal(err)
		}
		if e != win[i] {
			t.Fatalf("file line %d differs from window:\n%+v\n%+v", i, e, win[i])
		}
	}
}

func TestDispatcherConcurrentChainStaysValid(t *testing.T) {
	d := NewDispatcher(Nop())
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				d.Log(mkEntry(i, 200))
			}
		}()
	}
	wg.Wait()
	if r := d.VerifyMemory(); !r.OK || r.Checked != 800 {
		t.Fatalf("verify = %+v", r)
	}
}

func TestDispatcherResumesChainFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	rl, err := NewRotatingFileLogger(path, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	d1 := NewDispatcher(rl)
	for i := 0; i < 3; i++ {
		d1.Log(mkEntry(i, 200))
	}
	_ = rl.Close()

	rl2, err := NewRotatingFileLogger(path, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rl2.Close() }()
	d2 := NewDispatcher(rl2)
	if got := d2.Ring().Len(); got != 3 {
		t.Fatalf("window not repopulated from disk: %d", got)
	}
	if snap := d2.Stats().Snapshot(5); snap.Today != 3 {
		t.Fatalf("today counter not repopulated: %+v", snap)
	}
	d2.Log(mkEntry(3, 200))

	res, err := VerifyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Checked != 4 || !res.FromGenesis {
		t.Fatalf("chain broken across restart: %+v", res)
	}
	if r := d2.VerifyMemory(); !r.OK || r.Checked != 4 {
		t.Fatalf("memory verify = %+v", r)
	}
	if _, seq := d2.Head(); seq != 4 {
		t.Fatalf("seq = %d", seq)
	}
}

func TestDispatcherChainSurvivesRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	rl, err := NewRotatingFileLogger(path, 1, 3) // 1 byte → rotate on every write
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rl.Close() }()
	d := NewDispatcher(rl)
	for i := 0; i < 4; i++ {
		d.Log(mkEntry(i, 200))
	}
	if r := d.VerifyMemory(); !r.OK || r.Checked != 4 {
		t.Fatalf("verify = %+v", r)
	}
	time.Sleep(150 * time.Millisecond) // let the async purge of old files finish before TempDir cleanup
}

// ── verify ───────────────────────────────────────────────────────────────────

func sealedChain(n int) []Entry {
	d := NewDispatcher(Nop())
	for i := 0; i < n; i++ {
		d.Log(mkEntry(i, 200))
	}
	return d.Ring().Snapshot()
}

func TestVerifyEntriesDetectsTampering(t *testing.T) {
	t.Run("modified record", func(t *testing.T) {
		c := sealedChain(6)
		c[3].Status = 500
		r := VerifyEntries(c, "")
		if r.OK || r.FirstBroken != 3 {
			t.Fatalf("got %+v", r)
		}
	})
	t.Run("deleted record", func(t *testing.T) {
		c := sealedChain(6)
		c = append(c[:2], c[3:]...)
		r := VerifyEntries(c, "")
		if r.OK || r.FirstBroken != 2 {
			t.Fatalf("got %+v", r)
		}
	})
	t.Run("rehashed after edit still breaks the link", func(t *testing.T) {
		c := sealedChain(6)
		c[2].Path = "/v1/x"
		c[2].Hash = ComputeHash(c[2])
		r := VerifyEntries(c, "")
		if r.OK || r.FirstBroken != 3 {
			t.Fatalf("got %+v", r)
		}
	})
	t.Run("anchor mismatch", func(t *testing.T) {
		c := sealedChain(3)
		if r := VerifyEntries(c, strings.Repeat("a", 64)); r.OK || r.FirstBroken != 0 {
			t.Fatalf("got %+v", r)
		}
		if r := VerifyEntries(c, GenesisHash); !r.OK {
			t.Fatalf("got %+v", r)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if r := VerifyEntries(nil, ""); !r.OK || r.Checked != 0 || r.FirstBroken != -1 {
			t.Fatalf("got %+v", r)
		}
	})
}

func TestVerifyFileTamperAndLegacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	// Legacy lines: written by the pre-Seq format (no seq/operation/namespace).
	type legacy struct {
		Time        string `json:"time"`
		RequestID   string `json:"request_id,omitempty"`
		Method      string `json:"method"`
		Path        string `json:"path"`
		Status      int    `json:"status"`
		ClientIP    string `json:"client_ip,omitempty"`
		Fingerprint string `json:"fingerprint,omitempty"`
		DurationMS  int64  `json:"duration_ms"`
		PrevHash    string `json:"prev_hash"`
		Hash        string `json:"hash"`
	}
	var sb strings.Builder
	prev := GenesisHash
	for i := 0; i < 3; i++ {
		l := legacy{Time: "2026-01-01T00:00:0" + fmt.Sprint(i) + "Z", Method: "PUT", Path: "/v1/secret/a", Status: 204, PrevHash: prev}
		plain, _ := json.Marshal(l)
		l.Hash = hashHex(plain)
		prev = l.Hash
		line, _ := json.Marshal(l)
		sb.Write(line)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := VerifyFile(path)
	if err != nil || !res.OK || res.Checked != 3 || res.Legacy != 3 {
		t.Fatalf("legacy chain: %+v, %v", res, err)
	}

	// A new dispatcher continues the legacy chain (seq 0 → 1).
	rl, err := NewRotatingFileLogger(path, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	d := NewDispatcher(rl)
	d.Log(mkEntry(0, 200))
	_ = rl.Close()
	res, err = VerifyFile(path)
	if err != nil || !res.OK || res.Checked != 4 {
		t.Fatalf("legacy+new chain: %+v, %v", res, err)
	}

	// Tamper with the second line.
	b, _ := os.ReadFile(path)
	b = bytes.Replace(b, []byte(`"status":204`), []byte(`"status":200`), 1)
	_ = os.WriteFile(path, b, 0o600)
	res, _ = VerifyFile(path)
	if res.OK || res.FirstBroken != 0 {
		t.Fatalf("tampered file: %+v", res)
	}

	// Garbage line.
	_ = os.WriteFile(path, []byte("not json\n"), 0o600)
	res, _ = VerifyFile(path)
	if res.OK || res.FirstBroken != 0 {
		t.Fatalf("garbage file: %+v", res)
	}
	if _, err := VerifyFile(filepath.Join(dir, "missing.log")); err == nil {
		t.Fatal("expected error for a missing file")
	}
}

func TestVerifyFileSinkOnlyKnownPaths(t *testing.T) {
	dir := t.TempDir()
	d := NewDispatcher(Nop())
	if _, err := d.VerifyFileSink(""); err == nil {
		t.Fatal("no primary file: expected error")
	}
	fs, err := NewFileSink(filepath.Join(dir, "sink.log"), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	d.Register("f", fs)
	wh := NewWebhookSink("http://127.0.0.1:1", time.Second)
	wh.Attempts = 1
	d.Register("w", wh)
	defer d.Close()
	d.Log(mkEntry(0, 200))
	waitFor(t, "file sink write", func() bool { return d.SinkStats()["f"].Sent == 1 })

	res, err := d.VerifyFileSink("f")
	if err != nil || !res.OK || res.Checked != 1 {
		t.Fatalf("file sink verify: %+v, %v", res, err)
	}
	if _, err := d.VerifyFileSink("w"); err == nil {
		t.Fatal("webhook sink must not be verifiable as a file")
	}
	if _, err := d.VerifyFileSink("nope"); !errors.Is(err, ErrSinkNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestReadTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	if h, s := readTail(path); h != "" || s != 0 {
		t.Fatal("missing file")
	}
	rl, _ := NewRotatingFileLogger(path, 0, 0)
	d := NewDispatcher(rl)
	d.Log(mkEntry(0, 200))
	d.Log(mkEntry(1, 200))
	_ = rl.Close()
	wantH, wantS := d.Head()
	if h, s := readTail(path); h != wantH || s != wantS {
		t.Fatalf("readTail = %s/%d want %s/%d", h, s, wantH, wantS)
	}
	// A torn final line is ignored.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"time":"x","seq":9`)
	_ = f.Close()
	if h, s := readTail(path); h != wantH || s != wantS {
		t.Fatalf("torn tail: %s/%d", h, s)
	}
}

// ── ring & filter ────────────────────────────────────────────────────────────

func TestRingOverwriteAfterAndGap(t *testing.T) {
	d := NewDispatcherSize(Nop(), 4)
	for i := 0; i < 10; i++ {
		d.Log(mkEntry(i, 200))
	}
	r := d.Ring()
	if r.Len() != 4 || r.Cap() != 4 {
		t.Fatalf("len/cap = %d/%d", r.Len(), r.Cap())
	}
	w := r.Window()
	if w.OldestSeq != 7 || w.NewestSeq != 10 {
		t.Fatalf("window = %+v", w)
	}
	got, gap := r.After(8, 0)
	if len(got) != 2 || got[0].Seq != 9 || gap {
		t.Fatalf("After(8) = %v gap=%v", got, gap)
	}
	got, gap = r.After(2, 0)
	if len(got) != 4 || !gap {
		t.Fatalf("After(2) len=%d gap=%v", len(got), gap)
	}
	got, _ = r.After(0, 2)
	if len(got) != 2 || got[0].Seq != 7 {
		t.Fatalf("After(0,2) = %v", got)
	}
	// The window of an overwritten ring is still a valid chain.
	if v := d.VerifyMemory(); !v.OK || v.FromGenesis {
		t.Fatalf("verify = %+v", v)
	}
}

func TestRingWaitWakesOnAppend(t *testing.T) {
	r := NewRing(3)
	ch := r.Wait()
	select {
	case <-ch:
		t.Fatal("woke before append")
	default:
	}
	r.Append(Entry{Seq: 1})
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("not woken")
	}
}

func TestFilter(t *testing.T) {
	now := time.Now().UTC()
	ents := []Entry{
		{Seq: 1, Time: now.Add(-2 * time.Hour).Format(time.RFC3339Nano), Method: "GET", Path: "/v1/secret/db", Status: 200, Fingerprint: "aaaaaaaaaaaa"},
		{Seq: 2, Time: now.Add(-time.Minute).Format(time.RFC3339Nano), Method: "PUT", Path: "/v1/secret/db", Status: 204, Namespace: "team-a"},
		{Seq: 3, Time: now.Format(time.RFC3339Nano), Method: "POST", Path: "/v1/auth/jwt/login", Status: 403, ClientIP: "10.0.0.7"},
		{Seq: 4, Time: now.Format(time.RFC3339Nano), Method: "DELETE", Path: "/v1/secret/x", Status: 500},
	}
	count := func(f Filter) int {
		n := 0
		for _, e := range ents {
			if f.Match(e) {
				n++
			}
		}
		return n
	}
	cases := []struct {
		name string
		f    Filter
		want int
	}{
		{"empty", Filter{}, 4},
		{"path", Filter{Path: "secret/db"}, 2},
		{"op write", Filter{Operation: "write"}, 1}, // PUT only: POST .../login is classified "login"
		{"op login", Filter{Operation: "login"}, 1},
		{"op delete", Filter{Operation: "delete"}, 1},
		{"status exact", Filter{Status: "403"}, 1},
		{"status class", Filter{Status: "2xx"}, 2},
		{"status error", Filter{Status: "error"}, 2},
		{"since", Filter{Since: now.Add(-10 * time.Minute)}, 3},
		{"until", Filter{Until: now.Add(-time.Hour)}, 1},
		{"after seq", Filter{AfterSeq: 2}, 2},
		{"namespace", Filter{Namespace: "team-a"}, 1},
		{"root namespace", Filter{Namespace: "root"}, 3},
		{"q fingerprint", Filter{Query: "AAAA"}, 1},
		{"q ip", Filter{Query: "10.0.0.7"}, 1},
		{"combined", Filter{Path: "secret", Status: "2xx", Operation: "read"}, 1},
	}
	for _, c := range cases {
		if got := count(c.f); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
	for _, ok := range []string{"", "all", "error", "404", "4xx", "5XX"} {
		if !ValidStatusSpec(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"abc", "9xx", "99", "1000", "4x"} {
		if ValidStatusSpec(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestRingQueryNewestFirstWithLimit(t *testing.T) {
	d := NewDispatcher(Nop())
	for i := 0; i < 10; i++ {
		st := 200
		if i%2 == 1 {
			st = 404
		}
		d.Log(mkEntry(i, st))
	}
	got, matched := d.Ring().Query(Filter{Status: "404"}, 3)
	if matched != 5 || len(got) != 3 || got[0].Seq != 10 || got[2].Seq != 6 {
		t.Fatalf("matched=%d got=%v", matched, got)
	}
}

// ── stats ────────────────────────────────────────────────────────────────────

func TestStatsBucketsAndRate(t *testing.T) {
	clock := time.Date(2026, 3, 1, 12, 0, 30, 0, time.UTC)
	s := NewStats(func() time.Time { return clock })

	s.Record(Entry{Method: "GET", Path: "/v1/a", Status: 200})
	s.Record(Entry{Method: "PUT", Path: "/v1/a", Status: 500})
	clock = clock.Add(2 * time.Minute)
	s.Record(Entry{Method: "GET", Path: "/v1/a", Status: 200, Operation: "read"})

	snap := s.Snapshot(5)
	if snap.Today != 3 || snap.TodayErrors != 1 || snap.Total != 3 {
		t.Fatalf("snap = %+v", snap)
	}
	if snap.ByOperation["read"] != 2 || snap.ByOperation["write"] != 1 {
		t.Fatalf("by op = %v", snap.ByOperation)
	}
	if len(snap.PerMinute) != 5 {
		t.Fatalf("bars = %d", len(snap.PerMinute))
	}
	last := snap.PerMinute[4]
	if last.Count != 1 || snap.PerMinute[2].Count != 2 || snap.PerMinute[2].Errors != 1 {
		t.Fatalf("bars = %+v", snap.PerMinute)
	}
	if snap.LastMinute != 1 {
		t.Fatalf("last minute = %d", snap.LastMinute)
	}

	// 40 s later the first record of the third minute has aged out of the rate.
	clock = clock.Add(40 * time.Second)
	if got := s.Snapshot(1).LastMinute; got != 1 {
		t.Fatalf("rate = %d", got)
	}
	clock = clock.Add(30 * time.Second)
	if got := s.Snapshot(1).LastMinute; got != 0 {
		t.Fatalf("rate after window = %d", got)
	}

	// New UTC day resets "today" but keeps the histogram.
	clock = time.Date(2026, 3, 2, 0, 5, 0, 0, time.UTC)
	snap = s.Snapshot(30)
	if snap.Today != 0 || len(snap.ByOperation) != 0 {
		t.Fatalf("next day = %+v", snap)
	}
	if snap.Total != 3 {
		t.Fatalf("total = %d", snap.Total)
	}
}

func TestStatsIgnoresOldAndClampsWindow(t *testing.T) {
	now := time.Now()
	s := NewStats(func() time.Time { return now })
	s.recordAt(now.Add(-25*time.Hour), Entry{Method: "GET", Status: 200})
	if s.Snapshot(10).Total != 0 {
		t.Fatal("old record counted")
	}
	if n := len(s.Snapshot(1_000_000).PerMinute); n != statsMinutes {
		t.Fatalf("clamp = %d", n)
	}
	if n := len(s.Snapshot(-3).PerMinute); n != 1 {
		t.Fatalf("clamp min = %d", n)
	}
}

// ── sinks ────────────────────────────────────────────────────────────────────

type fakeSink struct {
	mu       sync.Mutex
	got      []Entry
	failNext atomic.Int32 // number of upcoming Send calls to fail
	perm     bool
	calls    atomic.Int32
	closed   atomic.Bool
	block    chan struct{}
}

func (f *fakeSink) Send(e Entry) error {
	_, err := f.SendBytes(e)
	return err
}

func (f *fakeSink) SendBytes(e Entry) (int, error) {
	f.calls.Add(1)
	if f.block != nil {
		<-f.block
	}
	if f.failNext.Add(-1) >= 0 {
		err := errors.New("boom")
		if f.perm {
			return 0, permanentError{err}
		}
		return 0, err
	}
	f.mu.Lock()
	f.got = append(f.got, e)
	f.mu.Unlock()
	return 10, nil
}

func (f *fakeSink) Close() error { f.closed.Store(true); return nil }

func (f *fakeSink) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.got)
}

// retrySink makes fakeSink retryable with a tiny backoff.
type retrySink struct{ *fakeSink }

func (retrySink) RetryPolicy() (int, time.Duration) { return 3, time.Millisecond }

func TestSinkDeliveryStatsAndOrder(t *testing.T) {
	d := NewDispatcher(Nop())
	fs := &fakeSink{}
	d.RegisterWith("s", "file", fs, false)
	defer d.Close()
	for i := 0; i < 20; i++ {
		d.Log(mkEntry(i, 200))
	}
	waitFor(t, "delivery", func() bool { return fs.count() == 20 })
	for i, e := range fs.got {
		if e.Seq != uint64(i+1) {
			t.Fatalf("out of order at %d: seq %d", i, e.Seq)
		}
	}
	st := d.SinkStats()["s"]
	if st.Sent != 20 || st.Bytes != 200 || st.Errors != 0 || st.LastSuccessAt == "" || st.LastError != "" {
		t.Fatalf("stats = %+v", st)
	}
	if d.List()["s"] != 0 {
		t.Fatalf("List = %v", d.List())
	}
}

func TestSinkRetriesTransientFailure(t *testing.T) {
	d := NewDispatcher(Nop())
	fs := &fakeSink{}
	fs.failNext.Store(2)
	d.RegisterWith("r", "webhook", retrySink{fs}, false)
	defer d.Close()
	d.Log(mkEntry(0, 200))
	waitFor(t, "retried delivery", func() bool { return d.SinkStats()["r"].Sent == 1 })
	st := d.SinkStats()["r"]
	if st.Retries != 2 || st.Errors != 0 || fs.calls.Load() != 3 {
		t.Fatalf("stats = %+v calls=%d", st, fs.calls.Load())
	}
}

func TestSinkPermanentErrorNotRetriedAndRecorded(t *testing.T) {
	d := NewDispatcher(Nop())
	fs := &fakeSink{perm: true}
	fs.failNext.Store(1)
	d.RegisterWith("p", "webhook", retrySink{fs}, false)
	defer d.Close()
	d.Log(mkEntry(0, 200))
	waitFor(t, "failure", func() bool { return d.SinkStats()["p"].Errors == 1 })
	st := d.SinkStats()["p"]
	if fs.calls.Load() != 1 || st.Retries != 0 || st.LastError != "boom" || st.LastErrorAt == "" || st.Failing != 1 {
		t.Fatalf("stats = %+v calls=%d", st, fs.calls.Load())
	}
	// Recovery clears the failing streak but keeps the last error for the UI.
	d.Log(mkEntry(1, 200))
	waitFor(t, "recovery", func() bool { return d.SinkStats()["p"].Sent == 1 })
	if st := d.SinkStats()["p"]; st.Failing != 0 || st.LastError == "" {
		t.Fatalf("stats = %+v", st)
	}
}

func TestSinkPauseResume(t *testing.T) {
	d := NewDispatcher(Nop())
	fs := &fakeSink{}
	d.RegisterWith("s", "file", fs, true) // registered paused
	defer d.Close()
	d.Log(mkEntry(0, 200))
	d.Log(mkEntry(1, 200))
	st := d.SinkStats()["s"]
	if !st.Paused || st.Skipped != 2 || st.Sent != 0 {
		t.Fatalf("paused stats = %+v", st)
	}
	if err := d.SetPaused("s", false); err != nil {
		t.Fatal(err)
	}
	d.Log(mkEntry(2, 200))
	waitFor(t, "resumed delivery", func() bool { return fs.count() == 1 })
	if fs.got[0].Seq != 3 {
		t.Fatalf("seq = %d", fs.got[0].Seq)
	}
	if err := d.SetPaused("missing", true); !errors.Is(err, ErrSinkNotFound) {
		t.Fatalf("got %v", err)
	}
	// The primary window is unaffected by pausing a sink.
	if d.Ring().Len() != 3 {
		t.Fatalf("ring len = %d", d.Ring().Len())
	}
}

func TestSinkQueueOverflowDropsInsteadOfBlocking(t *testing.T) {
	d := NewDispatcher(Nop())
	fs := &fakeSink{block: make(chan struct{})}
	d.RegisterWith("slow", "webhook", fs, false)

	done := make(chan struct{})
	go func() {
		for i := 0; i < sinkQueueSize+50; i++ {
			d.Log(mkEntry(i, 200))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Log blocked on a stuck sink")
	}
	st := d.SinkStats()["slow"]
	if st.Dropped < 40 || st.Queue == 0 {
		t.Fatalf("stats = %+v", st)
	}
	close(fs.block)
	d.Close()
	if !fs.closed.Load() {
		t.Fatal("sink not closed")
	}
}

func TestDeregisterDrainsAndCloses(t *testing.T) {
	d := NewDispatcher(Nop())
	fs := &fakeSink{}
	d.Register("x", fs)
	for i := 0; i < 10; i++ {
		d.Log(mkEntry(i, 200))
	}
	if err := d.Deregister("x"); err != nil {
		t.Fatal(err)
	}
	if fs.count() != 10 || !fs.closed.Load() {
		t.Fatalf("drained %d closed=%v", fs.count(), fs.closed.Load())
	}
	if err := d.Deregister("x"); err == nil {
		t.Fatal("second deregister must fail")
	}
	// Logging after removal must not panic.
	d.Log(mkEntry(0, 200))
}

func TestReRegisterReplacesAndResetsStats(t *testing.T) {
	d := NewDispatcher(Nop())
	a, b := &fakeSink{}, &fakeSink{}
	d.Register("x", a)
	d.Log(mkEntry(0, 200))
	waitFor(t, "a", func() bool { return a.count() == 1 })
	d.Register("x", b)
	if !a.closed.Load() {
		t.Fatal("old sink not closed")
	}
	if st := d.SinkStats()["x"]; st.Sent != 0 {
		t.Fatalf("stats not reset: %+v", st)
	}
}

func TestWebhookErrorsDoNotLeakURL(t *testing.T) {
	w := NewWebhookSink("http://user:s3cr3t@127.0.0.1:1/hook?token=abc", 200*time.Millisecond)
	w.Attempts = 1
	_, err := w.SendBytes(mkEntry(0, 200))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, leak := range []string{"s3cr3t", "token=abc", "127.0.0.1:1/hook"} {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("error leaks %q: %v", leak, err)
		}
	}
}

func TestSinkConfigBackwardCompat(t *testing.T) {
	var c SinkConfig
	if err := json.Unmarshal([]byte(`{"name":"a","type":"webhook","options":{"url":"http://x"}}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Disabled {
		t.Fatal("old configs must load as enabled")
	}
	out, _ := json.Marshal(SinkConfig{Name: "a", Type: "file"})
	if strings.Contains(string(out), "disabled") {
		t.Fatalf("zero value must not be serialised: %s", out)
	}
}

func hashHex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestVerifyFileIgnoresWriteInProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.log")
	rl, _ := NewRotatingFileLogger(path, 0, 0)
	d := NewDispatcher(rl)
	d.Log(mkEntry(0, 200))
	d.Log(mkEntry(1, 200))
	_ = rl.Close()
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"time":"x","seq":3,"meth`)
	_ = f.Close()
	res, err := VerifyFile(path)
	if err != nil || !res.OK || res.Checked != 2 {
		t.Fatalf("got %+v, %v", res, err)
	}
}
