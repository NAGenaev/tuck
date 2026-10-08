package audit

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// sinkQueueSize bounds the per-sink delivery queue. When a sink cannot keep
	// up, new entries for it are dropped (and counted) instead of blocking the
	// request path.
	sinkQueueSize = 2048
	// failFast is the number of consecutive failed deliveries after which a
	// sink stops retrying each entry (one attempt only) until it succeeds again.
	failFast = 3
	// drainTimeout bounds how long Close waits for queued entries.
	drainTimeout = 2 * time.Second
	// seedWindow is how much of the log file tail is read to repopulate the
	// in-memory window and the counters after a restart.
	seedWindow = 8 << 20
)

// Dispatcher is the single authority of the audit hash chain. Every entry is
// sealed (sequence number, prev_hash, hash) exactly once, then fanned out to
// the primary log, the in-memory window (readable log, live tail) and every
// registered Sink. Sink delivery is asynchronous: a slow or failing sink never
// blocks the request path.
type Dispatcher struct {
	logger Loggable
	ring   *Ring
	stats  *Stats

	chainMu  sync.Mutex // serialises sealing + fan-out so all outputs share one order
	prevHash string
	seq      uint64

	mu    sync.RWMutex
	sinks map[string]*sinkRunner
}

// NewDispatcher creates a Dispatcher wrapping l with the default window size.
// l may be *Logger, *RotatingFileLogger, or any Loggable.
func NewDispatcher(l Loggable) *Dispatcher { return NewDispatcherSize(l, DefaultBufferSize) }

// NewDispatcherSize is NewDispatcher with an explicit in-memory window size.
// When l is a file logger that already holds records, the chain continues from
// the last record on disk, and the window and counters are repopulated from
// the tail of that file.
func NewDispatcherSize(l Loggable, bufSize int) *Dispatcher {
	if bufSize <= 0 {
		bufSize = DefaultBufferSize
	}
	d := &Dispatcher{
		logger:   l,
		ring:     NewRing(bufSize),
		stats:    NewStats(nil),
		prevHash: GenesisHash,
		sinks:    make(map[string]*sinkRunner),
	}
	if lh, ok := l.(interface{ LastHash() (string, uint64) }); ok {
		if h, s := lh.LastHash(); h != "" {
			d.prevHash, d.seq = h, s
		}
	}
	if p := d.PrimaryPath(); p != "" {
		d.seed(p)
	}
	return d
}

// seed loads the tail of the primary log into the window and the counters.
func (d *Dispatcher) seed(path string) {
	entries := readTailEntries(path, seedWindow, d.ring.Cap())
	for _, e := range entries {
		d.ring.Append(e)
		if t, err := time.Parse(time.RFC3339Nano, e.Time); err == nil {
			d.stats.recordAt(t, e)
		}
	}
}

// Log seals e into the chain and delivers it everywhere.
func (d *Dispatcher) Log(e Entry) {
	if e.Operation == "" {
		e.Operation = Operation(e.Method, e.Path)
	}
	d.chainMu.Lock()
	d.seq++
	e.Seq = d.seq
	e.PrevHash = d.prevHash
	e.Hash = ComputeHash(e)
	d.prevHash = e.Hash
	d.logger.Log(e)
	d.ring.Append(e)
	d.mu.RLock()
	for _, r := range d.sinks {
		r.enqueue(e)
	}
	d.mu.RUnlock()
	d.chainMu.Unlock()
	d.stats.Record(e)
}

// Ring exposes the in-memory window of recent entries.
func (d *Dispatcher) Ring() *Ring { return d.ring }

// Stats exposes the aggregate counters.
func (d *Dispatcher) Stats() *Stats { return d.stats }

// Head returns the hash and sequence number of the newest sealed entry.
func (d *Dispatcher) Head() (string, uint64) {
	d.chainMu.Lock()
	defer d.chainMu.Unlock()
	return d.prevHash, d.seq
}

// PrimaryPath is the path of the primary audit log file ("" when the primary
// log is not a file).
func (d *Dispatcher) PrimaryPath() string {
	if p, ok := d.logger.(interface{ Path() string }); ok {
		return p.Path()
	}
	return ""
}

// VerifyMemory checks the hash chain of the in-memory window.
func (d *Dispatcher) VerifyMemory() VerifyResult {
	return VerifyEntries(d.ring.Snapshot(), "")
}

// VerifyFileSink verifies the hash chain on disk. name "" (or "primary")
// selects the primary log; otherwise it must be the name of a registered file
// sink. Client-supplied paths are never accepted.
func (d *Dispatcher) VerifyFileSink(name string) (VerifyResult, error) {
	path := ""
	if name == "" || name == "primary" {
		path = d.PrimaryPath()
		if path == "" {
			return VerifyResult{}, errors.New("no primary audit log file configured")
		}
	} else {
		d.mu.RLock()
		r, ok := d.sinks[name]
		d.mu.RUnlock()
		if !ok {
			return VerifyResult{}, ErrSinkNotFound
		}
		fs, ok := r.sink.(*FileSink)
		if !ok {
			return VerifyResult{}, fmt.Errorf("sink %q is not a file sink", name)
		}
		path = fs.Path()
	}
	return VerifyFile(path)
}

// Register adds or replaces a named Sink (enabled). The old Sink, if any, is
// drained and closed.
func (d *Dispatcher) Register(name string, sink Sink) {
	typ := ""
	switch sink.(type) {
	case *WebhookSink:
		typ = "webhook"
	case *FileSink:
		typ = "file"
	}
	d.RegisterWith(name, typ, sink, false)
}

// RegisterWith is Register with an explicit sink type and initial paused state.
func (d *Dispatcher) RegisterWith(name, typ string, sink Sink, paused bool) {
	r := newSinkRunner(name, typ, sink)
	r.paused.Store(paused)
	d.mu.Lock()
	old := d.sinks[name]
	d.sinks[name] = r
	d.mu.Unlock()
	if old != nil {
		_ = old.close(drainTimeout)
	}
}

// Deregister removes a named Sink, draining its queue and closing it.
func (d *Dispatcher) Deregister(name string) error {
	d.mu.Lock()
	r, ok := d.sinks[name]
	delete(d.sinks, name)
	d.mu.Unlock()
	if !ok {
		return errors.New("sink not found: " + name)
	}
	return r.close(drainTimeout)
}

// SetPaused pauses or resumes delivery to a sink. Entries produced while a
// sink is paused are not replayed on resume.
func (d *Dispatcher) SetPaused(name string, paused bool) error {
	d.mu.RLock()
	r, ok := d.sinks[name]
	d.mu.RUnlock()
	if !ok {
		return ErrSinkNotFound
	}
	r.paused.Store(paused)
	return nil
}

// List returns names and error counts of all registered sinks.
func (d *Dispatcher) List() map[string]int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[string]int, len(d.sinks))
	for k, r := range d.sinks {
		out[k] = int(r.snapshot().Errors)
	}
	return out
}

// SinkStats returns runtime statistics for every registered sink.
func (d *Dispatcher) SinkStats() map[string]SinkStats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[string]SinkStats, len(d.sinks))
	for k, r := range d.sinks {
		out[k] = r.snapshot()
	}
	return out
}

// Close drains and closes all sinks.
func (d *Dispatcher) Close() {
	d.mu.Lock()
	rs := d.sinks
	d.sinks = make(map[string]*sinkRunner)
	d.mu.Unlock()
	for _, r := range rs {
		_ = r.close(drainTimeout)
	}
}

// ─── per-sink worker ────────────────────────────────────────────────────────

// SinkStats is the runtime state of one sink. Counters start at zero when the
// sink is (re-)registered or the process starts; they are not persisted.
type SinkStats struct {
	Sent          int64  `json:"sent"`    // records delivered
	Bytes         int64  `json:"bytes"`   // bytes delivered (file/webhook; 0 when the sink cannot tell)
	Errors        int64  `json:"errors"`  // failed deliveries (after retries)
	Retries       int64  `json:"retries"` // extra attempts made after a transient failure
	Dropped       int64  `json:"dropped"` // entries lost because the queue was full
	Skipped       int64  `json:"skipped"` // entries not delivered because the sink was paused
	Queue         int    `json:"queue"`   // entries waiting for delivery
	Failing       int    `json:"consecutive_failures"`
	Paused        bool   `json:"paused"`
	LastError     string `json:"last_error,omitempty"`
	LastErrorAt   string `json:"last_error_at,omitempty"`
	LastSuccessAt string `json:"last_success_at,omitempty"`
	// NextRetryAt is set while the worker waits out a retry backoff.
	NextRetryAt string `json:"next_retry_at,omitempty"`
}

type sinkRunner struct {
	name string
	typ  string
	sink Sink

	paused atomic.Bool
	q      chan Entry
	done   chan struct{}
	stop   chan struct{}

	closeMu sync.Mutex
	closed  bool

	mu        sync.Mutex
	st        SinkStats
	nextRetry time.Time
}

func newSinkRunner(name, typ string, sink Sink) *sinkRunner {
	r := &sinkRunner{
		name: name,
		typ:  typ,
		sink: sink,
		q:    make(chan Entry, sinkQueueSize),
		done: make(chan struct{}),
		stop: make(chan struct{}),
	}
	go r.run()
	return r
}

func (r *sinkRunner) enqueue(e Entry) {
	if r.paused.Load() {
		r.mu.Lock()
		r.st.Skipped++
		r.mu.Unlock()
		return
	}
	r.closeMu.Lock()
	defer r.closeMu.Unlock()
	if r.closed {
		return
	}
	select {
	case r.q <- e:
	default:
		r.mu.Lock()
		r.st.Dropped++
		r.mu.Unlock()
	}
}

func (r *sinkRunner) run() {
	defer close(r.done)
	for e := range r.q {
		r.deliver(e)
	}
}

func (r *sinkRunner) deliver(e Entry) {
	attempts, delay := 1, time.Duration(0)
	if rp, ok := r.sink.(retrier); ok {
		attempts, delay = rp.RetryPolicy()
		if attempts < 1 {
			attempts = 1
		}
	}
	r.mu.Lock()
	if r.st.Failing >= failFast {
		attempts = 1 // the sink has been down for a while: do not stall the queue
	}
	r.mu.Unlock()

	var (
		n   int
		err error
	)
	for a := 1; a <= attempts; a++ {
		n, err = r.send(e)
		if err == nil || isPermanent(err) || a == attempts {
			break
		}
		r.mu.Lock()
		r.st.Retries++
		r.nextRetry = time.Now().Add(delay)
		r.mu.Unlock()
		if !r.sleep(delay) {
			break
		}
		delay *= 4
	}

	now := time.Now().UTC().Format(time.RFC3339)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextRetry = time.Time{}
	if err != nil {
		r.st.Errors++
		r.st.Failing++
		r.st.LastError = truncate(err.Error(), 300)
		r.st.LastErrorAt = now
		return
	}
	r.st.Sent++
	r.st.Bytes += int64(n)
	r.st.Failing = 0
	r.st.LastSuccessAt = now
}

func (r *sinkRunner) send(e Entry) (n int, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = permanentError{fmt.Errorf("sink panic: %v", p)}
		}
	}()
	if bs, ok := r.sink.(byteSender); ok {
		return bs.SendBytes(e)
	}
	return 0, r.sink.Send(e)
}

// sleep waits d or until the runner is told to stop; it reports whether the
// full delay elapsed.
func (r *sinkRunner) sleep(d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-r.stop:
		return false
	}
}

func (r *sinkRunner) snapshot() SinkStats {
	r.mu.Lock()
	st := r.st
	next := r.nextRetry
	r.mu.Unlock()
	st.Queue = len(r.q)
	st.Paused = r.paused.Load()
	if !next.IsZero() && time.Now().Before(next) {
		st.NextRetryAt = next.UTC().Format(time.RFC3339)
	}
	return st
}

// close stops accepting entries, gives the worker up to timeout to drain the
// queue, then closes the sink.
func (r *sinkRunner) close(timeout time.Duration) error {
	r.closeMu.Lock()
	if r.closed {
		r.closeMu.Unlock()
		return nil
	}
	r.closed = true
	close(r.q)
	r.closeMu.Unlock()

	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-r.done:
	case <-t.C:
		close(r.stop) // abort pending backoff sleeps
		<-r.done
	}
	return r.sink.Close()
}

func isPermanent(err error) bool {
	var p permanent
	return errors.As(err, &p) && p.Permanent()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// PathInUse reports whether path is already written by the primary audit log or
// by a file sink other than except. Two writers on one file would interleave
// and duplicate records.
func (d *Dispatcher) PathInUse(path, except string) bool {
	norm := func(p string) string {
		if a, err := filepath.Abs(p); err == nil {
			p = a
		}
		return filepath.Clean(p)
	}
	want := norm(path)
	if p := d.PrimaryPath(); p != "" && norm(p) == want {
		return true
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	for name, r := range d.sinks {
		if name == except {
			continue
		}
		if fs, ok := r.sink.(*FileSink); ok && norm(fs.Path()) == want {
			return true
		}
	}
	return false
}
