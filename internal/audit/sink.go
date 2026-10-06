package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Sink receives audit entries for forwarding to an external system.
// Send must not block longer than a reasonable timeout — the Dispatcher
// delivers from a per-sink worker, never from the request path.
// Close releases any held resources.
type Sink interface {
	Send(e Entry) error
	Close() error
}

// byteSender is implemented by sinks that can report how many bytes a
// successful delivery put on the wire / on disk.
type byteSender interface {
	SendBytes(e Entry) (int, error)
}

// retrier is implemented by sinks whose failures are worth retrying with
// exponential backoff (network sinks).
type retrier interface {
	RetryPolicy() (attempts int, base time.Duration)
}

// permanent marks an error that retrying cannot fix (e.g. HTTP 4xx).
type permanent interface{ Permanent() bool }

type permanentError struct{ error }

func (permanentError) Permanent() bool { return true }
func (p permanentError) Unwrap() error { return p.error }

// SinkConfig describes a registered audit sink persisted in the barrier.
type SinkConfig struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"` // "webhook" | "file" | "syslog"
	Options map[string]string `json:"options"`
	// Disabled pauses delivery without deleting the sink. The zero value
	// (absent in older stored configs) means enabled.
	Disabled bool `json:"disabled,omitempty"`
}

// ─── Webhook sink ───────────────────────────────────────────────────────────

// WebhookSink POSTs each audit entry as JSON to a URL. Transient failures
// (network errors, 5xx, 408, 429) are retried with exponential backoff by the
// Dispatcher; 4xx answers are not.
type WebhookSink struct {
	URL     string
	Timeout time.Duration
	// Attempts is the total number of delivery attempts per entry (>= 1).
	Attempts int
	// Backoff is the base delay between attempts; it quadruples each retry.
	Backoff time.Duration
	client  *http.Client
}

// NewWebhookSink creates a WebhookSink that POSTs to url.
// timeout 0 defaults to 5 s.
func NewWebhookSink(url string, timeout time.Duration) *WebhookSink {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &WebhookSink{
		URL:      url,
		Timeout:  timeout,
		Attempts: 3,
		Backoff:  500 * time.Millisecond,
		client:   &http.Client{Timeout: timeout},
	}
}

// RetryPolicy implements retrier.
func (w *WebhookSink) RetryPolicy() (int, time.Duration) { return w.Attempts, w.Backoff }

func (w *WebhookSink) Send(e Entry) error {
	_, err := w.SendBytes(e)
	return err
}

// SendBytes implements byteSender.
func (w *WebhookSink) SendBytes(e Entry) (int, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return 0, permanentError{err}
	}
	ctx, cancel := context.WithTimeout(context.Background(), w.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(data))
	if err != nil {
		return 0, permanentError{redactURLError(err)}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return 0, redactURLError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		herr := fmt.Errorf("webhook: HTTP %d", resp.StatusCode)
		if resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
			return 0, permanentError{herr}
		}
		return 0, herr
	}
	return len(data), nil
}

func (w *WebhookSink) Close() error { return nil }

// redactURLError drops the request URL from *url.Error: webhook URLs often
// carry credentials or tokens that must not surface in stats or the UI.
func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("webhook %s: %w", ue.Op, ue.Err)
	}
	return err
}

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
	_, err := f.rfl.Write(e)
	return err
}

// SendBytes implements byteSender.
func (f *FileSink) SendBytes(e Entry) (int, error) { return f.rfl.Write(e) }

// Path returns the path of the active file.
func (f *FileSink) Path() string { return f.rfl.Path() }

func (f *FileSink) Close() error { return f.rfl.Close() }
