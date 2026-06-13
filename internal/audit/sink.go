package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Sink receives audit entries for forwarding to an external system.
// Send must not block longer than a reasonable timeout — the caller
// does not wait for delivery. Close releases any held resources.
type Sink interface {
	Send(e Entry) error
	Close() error
}

// SinkConfig describes a registered audit sink persisted in the barrier.
type SinkConfig struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"` // "webhook" | "syslog"
	Options map[string]string `json:"options"`
}

// Dispatcher fans audit entries to the hash-chain Logger and zero or more
// additional Sinks. Sink failures are non-fatal: they are counted but never
// block the request path.
type Dispatcher struct {
	logger Loggable

	mu     sync.RWMutex
	sinks  map[string]Sink
	errors map[string]int // error counts per sink name
}

// NewDispatcher creates a Dispatcher wrapping l.
// l may be *Logger, *RotatingFileLogger, or any Loggable.
func NewDispatcher(l Loggable) *Dispatcher {
	return &Dispatcher{
		logger: l,
		sinks:  make(map[string]Sink),
		errors: make(map[string]int),
	}
}

// Log writes e to the hash-chain Logger and all registered Sinks.
func (d *Dispatcher) Log(e Entry) {
	d.logger.Log(e)

	d.mu.RLock()
	defer d.mu.RUnlock()
	for name, sink := range d.sinks {
		if err := sink.Send(e); err != nil {
			d.errors[name]++
		}
	}
}

// Register adds or replaces a named Sink. The old Sink (if any) is closed.
func (d *Dispatcher) Register(name string, sink Sink) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if old, ok := d.sinks[name]; ok {
		_ = old.Close()
	}
	d.sinks[name] = sink
	d.errors[name] = 0
}

// Deregister removes a named Sink and closes it.
func (d *Dispatcher) Deregister(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	sink, ok := d.sinks[name]
	if !ok {
		return errors.New("sink not found: " + name)
	}
	err := sink.Close()
	delete(d.sinks, name)
	delete(d.errors, name)
	return err
}

// List returns names and error counts of all registered sinks.
func (d *Dispatcher) List() map[string]int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[string]int, len(d.errors))
	for k, v := range d.errors {
		out[k] = v
	}
	return out
}

// Close closes all sinks.
func (d *Dispatcher) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, s := range d.sinks {
		_ = s.Close()
	}
	d.sinks = make(map[string]Sink)
}

// ─── Webhook sink ───────────────────────────────────────────────────────────

// WebhookSink POSTs each audit entry as JSON to a URL.
type WebhookSink struct {
	URL     string
	Timeout time.Duration
	client  *http.Client
}

// NewWebhookSink creates a WebhookSink that POSTs to url.
// timeout 0 defaults to 5 s.
func NewWebhookSink(url string, timeout time.Duration) *WebhookSink {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookSink{
		URL:     url,
		Timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

func (w *WebhookSink) Send(e Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (w *WebhookSink) Close() error { return nil }

// ─── File sink ───────────────────────────────────────────────────────────────

// FileSink writes audit entries to a rotating local file.
type FileSink struct {
	rfl *RotatingFileLogger
}

// NewFileSink opens path for append and wraps it in a rotating audit sink.
// maxSizeMB ≤ 0 defaults to 100 MiB; maxBackups ≤ 0 defaults to 7.
func NewFileSink(path string, maxSizeMB int64, maxBackups int) (*FileSink, error) {
	var maxBytes int64
	if maxSizeMB > 0 {
		maxBytes = maxSizeMB << 20
	}
	rfl, err := NewRotatingFileLogger(path, maxBytes, maxBackups)
	if err != nil {
		return nil, err
	}
	return &FileSink{rfl: rfl}, nil
}

func (f *FileSink) Send(e Entry) error {
	f.rfl.Log(e)
	return nil
}

func (f *FileSink) Close() error { return f.rfl.Close() }
