package audit

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBufferSize is the number of recent audit records kept in memory for
// the readable log, the live tail and the in-memory chain verification.
const DefaultBufferSize = 10000

// Ring is a bounded, concurrency-safe buffer of the most recent audit entries
// (oldest overwritten first). It holds only what Entry holds: no secret values.
type Ring struct {
	mu     sync.RWMutex
	buf    []Entry
	next   int // index the next entry is written to
	n      int // number of valid entries
	last   uint64
	notify chan struct{} // closed (and replaced) on every Append
}

// NewRing returns a Ring holding at most capacity entries (min 1).
func NewRing(capacity int) *Ring {
	if capacity < 1 {
		capacity = 1
	}
	return &Ring{buf: make([]Entry, capacity), notify: make(chan struct{})}
}

// Cap is the maximum number of buffered entries.
func (r *Ring) Cap() int { return len(r.buf) }

// Len is the current number of buffered entries.
func (r *Ring) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.n
}

// Head is the sequence number of the newest appended entry (0 = none yet).
func (r *Ring) Head() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.last
}

// Append stores e and wakes every goroutine blocked on Wait.
func (r *Ring) Append(e Entry) {
	r.mu.Lock()
	r.buf[r.next] = e
	r.next = (r.next + 1) % len(r.buf)
	if r.n < len(r.buf) {
		r.n++
	}
	if e.Seq > r.last {
		r.last = e.Seq
	}
	old := r.notify
	r.notify = make(chan struct{})
	r.mu.Unlock()
	close(old)
}

// Wait returns a channel that is closed by the next Append.
func (r *Ring) Wait() <-chan struct{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.notify
}

// snapshotLocked returns entries oldest → newest. Caller holds r.mu.
func (r *Ring) snapshotLocked() []Entry {
	out := make([]Entry, 0, r.n)
	start := (r.next - r.n + len(r.buf)) % len(r.buf)
	for i := 0; i < r.n; i++ {
		out = append(out, r.buf[(start+i)%len(r.buf)])
	}
	return out
}

// Snapshot returns a copy of every buffered entry, oldest → newest.
func (r *Ring) Snapshot() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshotLocked()
}

// After returns the entries with Seq > seq, oldest → newest, at most max
// (0 = unlimited). gap reports that entries between seq and the oldest
// returned entry were already overwritten.
func (r *Ring) After(seq uint64, max int) (out []Entry, gap bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	start := (r.next - r.n + len(r.buf)) % len(r.buf)
	for i := 0; i < r.n; i++ {
		e := r.buf[(start+i)%len(r.buf)]
		if e.Seq <= seq {
			continue
		}
		if len(out) == 0 && seq != 0 && e.Seq > seq+1 {
			gap = true
		}
		out = append(out, e)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out, gap
}

// Window describes the buffered range.
type Window struct {
	Size      int    `json:"size"`
	Capacity  int    `json:"capacity"`
	OldestSeq uint64 `json:"oldest_seq"`
	NewestSeq uint64 `json:"newest_seq"`
}

// Window returns the current buffered range.
func (r *Ring) Window() Window {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w := Window{Size: r.n, Capacity: len(r.buf), NewestSeq: r.last}
	if r.n > 0 {
		w.OldestSeq = r.buf[(r.next-r.n+len(r.buf))%len(r.buf)].Seq
	}
	return w
}

// Query returns up to limit entries matching f, newest first, plus the total
// number of buffered entries that matched.
func (r *Ring) Query(f Filter, limit int) (out []Entry, matched int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := 0; i < r.n; i++ {
		e := r.buf[(r.next-1-i+2*len(r.buf))%len(r.buf)]
		if !f.Match(e) {
			continue
		}
		matched++
		if limit <= 0 || len(out) < limit {
			out = append(out, e)
		}
	}
	return out, matched
}

// Filter selects audit entries. Zero fields match everything.
type Filter struct {
	Since     time.Time // entry time >= Since
	Until     time.Time // entry time <= Until
	AfterSeq  uint64    // entry seq > AfterSeq
	Path      string    // substring of the request path
	Operation string    // read|write|delete|list|login|other
	Status    string    // "403" (exact), "4xx" (class) or "error" (>= 400)
	Namespace string    // exact; "root" matches the root namespace
	Query     string    // case-insensitive substring over path, request id, fingerprint, ip, namespace
}

// Match reports whether e satisfies every set criterion.
func (f Filter) Match(e Entry) bool {
	if f.AfterSeq != 0 && e.Seq <= f.AfterSeq {
		return false
	}
	if !f.Since.IsZero() || !f.Until.IsZero() {
		t, err := time.Parse(time.RFC3339Nano, e.Time)
		if err != nil {
			return false
		}
		if !f.Since.IsZero() && t.Before(f.Since) {
			return false
		}
		if !f.Until.IsZero() && t.After(f.Until) {
			return false
		}
	}
	if f.Path != "" && !strings.Contains(e.Path, f.Path) {
		return false
	}
	if f.Operation != "" && f.Operation != "all" {
		op := e.Operation
		if op == "" {
			op = Operation(e.Method, e.Path)
		}
		if op != f.Operation {
			return false
		}
	}
	if !statusMatches(f.Status, e.Status) {
		return false
	}
	if f.Namespace != "" && f.Namespace != "all" {
		ns := e.Namespace
		if ns == "" {
			ns = "root"
		}
		if ns != f.Namespace {
			return false
		}
	}
	if f.Query != "" {
		q := strings.ToLower(f.Query)
		hay := strings.ToLower(e.Path + " " + e.RequestID + " " + e.Fingerprint + " " + e.ClientIP + " " + e.Namespace + " " + e.Identity)
		if !strings.Contains(hay, q) {
			return false
		}
	}
	return true
}

func statusMatches(spec string, status int) bool {
	switch spec {
	case "", "all":
		return true
	case "error":
		return status >= 400
	}
	if len(spec) == 3 && (spec[1:] == "xx" || spec[1:] == "XX") {
		return status/100 == int(spec[0]-'0')
	}
	if n, err := strconv.Atoi(spec); err == nil {
		return status == n
	}
	return false
}

// ValidStatusSpec reports whether spec is a status filter statusMatches understands.
func ValidStatusSpec(spec string) bool {
	switch spec {
	case "", "all", "error":
		return true
	}
	if len(spec) == 3 && (spec[1:] == "xx" || spec[1:] == "XX") {
		return spec[0] >= '1' && spec[0] <= '5'
	}
	n, err := strconv.Atoi(spec)
	return err == nil && n >= 100 && n <= 599
}
