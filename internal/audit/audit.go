// Package audit provides a tamper-evident append-only audit log.
// Each entry is a JSON line containing a hash-chain link so that any
// deletion or modification of historical records is detectable.
//
// Values are NEVER logged — only paths, methods, status codes, and
// token fingerprints (SHA-256 of the token ID, truncated to 12 hex chars).
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// Entry is one audit record.
type Entry struct {
	Time        string `json:"time"`
	RequestID   string `json:"request_id,omitempty"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Status      int    `json:"status"`
	ClientIP    string `json:"client_ip,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"` // truncated SHA-256(tokenID)
	DurationMS  int64  `json:"duration_ms"`
	PrevHash    string `json:"prev_hash"` // hex SHA-256 of previous record
	Hash        string `json:"hash"`      // hex SHA-256 of this record (excl. hash field)
}

// Logger is a thread-safe tamper-evident audit logger.
type Logger struct {
	mu       sync.Mutex
	w        io.Writer
	prevHash string // hex SHA-256 of last written entry; "0"*64 for first
}

// NewLogger creates a Logger writing to w.
func NewLogger(w io.Writer) *Logger {
	return &Logger{
		w:        w,
		prevHash: fmt.Sprintf("%064x", 0),
	}
}

// NewFileLogger opens path for append-only writing and returns a Logger.
// The file is created if it doesn't exist.
func NewFileLogger(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 — operator-configured audit log path
	if err != nil {
		return nil, fmt.Errorf("audit: open %q: %w", path, err)
	}
	return NewLogger(f), nil
}

// Nop returns a Logger that discards all entries.
func Nop() *Logger { return NewLogger(io.Discard) }

// Log records one audit entry. It blocks until the entry is written.
// Errors are intentionally swallowed to keep audit from blocking the request
// path — in production, use file-system monitoring to detect write failures.
func (l *Logger) Log(e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e.PrevHash = l.prevHash

	// Compute hash over the entry WITHOUT the Hash field.
	e.Hash = ""
	plain, err := json.Marshal(e)
	if err != nil {
		return
	}
	h := sha256.Sum256(plain)
	e.Hash = hex.EncodeToString(h[:])
	l.prevHash = e.Hash

	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	line = append(line, '\n')
	_, _ = l.w.Write(line)
}

// Fingerprint returns a short (12-char) hex token fingerprint safe to log.
func Fingerprint(tokenID string) string {
	if tokenID == "" {
		return ""
	}
	h := sha256.Sum256([]byte(tokenID))
	return hex.EncodeToString(h[:6]) // 6 bytes = 12 hex chars
}

// ClientIP extracts the remote IP from a request, respecting X-Forwarded-For.
func ClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if host, _, err := net.SplitHostPort(fwd); err == nil {
			return host
		}
		return fwd
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// Loggable is implemented by Logger and Dispatcher.
type Loggable interface {
	Log(Entry)
}

// Middleware wraps handler h, writing one audit entry per request.
// requestIDKey is the context key used to propagate a request-id.
func Middleware(l Loggable, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &recorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rw, r)

		tok := r.Header.Get("X-Tuck-Token")
		l.Log(Entry{
			Time:        start.UTC().Format(time.RFC3339Nano),
			RequestID:   r.Header.Get("X-Request-Id"),
			Method:      r.Method,
			Path:        r.URL.Path,
			Status:      rw.status,
			ClientIP:    ClientIP(r),
			Fingerprint: Fingerprint(tok),
			DurationMS:  time.Since(start).Milliseconds(),
		})
	})
}

// recorder captures the HTTP status code written by a handler.
type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
