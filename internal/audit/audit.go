// Package audit provides a tamper-evident append-only audit log.
// Each entry is a JSON line containing a hash-chain link so that any
// deletion or modification of historical records is detectable.
//
// Values are NEVER logged — only paths, methods, status codes, and
// token fingerprints (SHA-256 of the token ID, truncated to 12 hex chars).
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/NAGenaev/tuck/internal/clientip"
)

// GenesisHash is the prev_hash of the very first record of a chain.
var GenesisHash = strings.Repeat("0", 64)

// Entry is one audit record.
//
// The field order below is part of the hash contract: Hash is the SHA-256 of
// the JSON encoding of the entry (this struct order, Hash set to ""). New
// optional fields are `omitempty`, so records written by older versions keep
// re-encoding to exactly the bytes that were hashed and stay verifiable.
type Entry struct {
	Time        string `json:"time"`
	Seq         uint64 `json:"seq,omitempty"` // monotonically increasing per chain
	RequestID   string `json:"request_id,omitempty"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Operation   string `json:"operation,omitempty"` // read|write|delete|list|login|other
	Namespace   string `json:"namespace,omitempty"` // validated namespace of the request; "" = root
	Status      int    `json:"status"`
	ClientIP    string `json:"client_ip,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"` // truncated SHA-256(tokenID)
	// Identity is who the request is attributed to: the token's display name
	// (e.g. "ldap:alice") or, for a login attempt, the claimed login. It is
	// written at request time, so it stays readable after the token is revoked.
	Identity string `json:"identity,omitempty"`
	EntityID string `json:"entity_id,omitempty"` // stable identity of the person/workload across logins
	DurationMS  int64  `json:"duration_ms"`
	PrevHash    string `json:"prev_hash"` // hex SHA-256 of previous record
	Hash        string `json:"hash"`      // hex SHA-256 of this record (excl. hash field)
}

// ComputeHash returns the hex SHA-256 of e encoded with its Hash field empty.
func ComputeHash(e Entry) string {
	e.Hash = ""
	plain, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(plain)
	return hex.EncodeToString(h[:])
}

// Operation classifies an HTTP request as read, write, delete, list, login or
// other. It is derived from the method and the (query-less) path only.
func Operation(method, path string) string {
	p := strings.TrimPrefix(path, "/v1/")
	if strings.HasPrefix(p, "auth/") && strings.Contains(p, "/login") {
		return "login"
	}
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead:
		return "read"
	case "LIST":
		return "list"
	case http.MethodPut, http.MethodPost, http.MethodPatch:
		return "write"
	case http.MethodDelete:
		return "delete"
	}
	return "other"
}

// Logger is a thread-safe tamper-evident audit logger.
type Logger struct {
	mu       sync.Mutex
	w        io.Writer
	prevHash string // hex SHA-256 of last written entry; "0"*64 for first
	written  atomic.Int64
}

// NewLogger creates a Logger writing to w.
func NewLogger(w io.Writer) *Logger {
	return &Logger{
		w:        w,
		prevHash: GenesisHash,
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
func (l *Logger) Log(e Entry) { _, _ = l.Write(e) }

// Write is Log that reports the number of bytes written and the write error.
//
// An entry that arrives already sealed (Hash != "", i.e. chained by the
// Dispatcher) is written verbatim so the file, the in-memory window and every
// sink share ONE chain. Otherwise the Logger chains it on its own.
func (l *Logger) Write(e Entry) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if e.Hash == "" {
		e.PrevHash = l.prevHash
		e.Hash = ComputeHash(e)
		if e.Hash == "" {
			return 0, fmt.Errorf("audit: cannot encode entry")
		}
	}
	l.prevHash = e.Hash

	line, err := json.Marshal(e)
	if err != nil {
		return 0, err
	}
	line = append(line, '\n')
	n, err := l.w.Write(line)
	l.written.Add(int64(n))
	return n, err
}

// BytesWritten is the number of bytes this Logger has written so far.
func (l *Logger) BytesWritten() int64 { return l.written.Load() }

// Fingerprint returns a short (12-char) hex token fingerprint safe to log.
func Fingerprint(tokenID string) string {
	if tokenID == "" {
		return ""
	}
	h := sha256.Sum256([]byte(tokenID))
	return hex.EncodeToString(h[:6]) // 6 bytes = 12 hex chars
}

// ClientIP extracts the client address from a request. X-Forwarded-For is
// honoured only when the direct peer is a configured trusted proxy
// (see package clientip); otherwise a client could write any address into
// the audit log.
func ClientIP(r *http.Request) string { return clientip.FromRequest(r) }

// Loggable is implemented by Logger and Dispatcher.
type Loggable interface {
	Log(Entry)
}

type annotationKey struct{}

// annotation lets inner handlers hand request facts that only they can
// validate (the effective namespace) back to the audit middleware.
type annotation struct {
	mu        sync.Mutex
	namespace string
	identity  string
	entityID  string
}

// SetIdentity records who the request is attributed to for its audit entry
// (token display name, entity id). It is a no-op outside of Middleware.
func SetIdentity(ctx context.Context, identity, entityID string) {
	if a, ok := ctx.Value(annotationKey{}).(*annotation); ok {
		a.mu.Lock()
		a.identity, a.entityID = sanitizeLabel(identity), sanitizeLabel(entityID)
		a.mu.Unlock()
	}
}

// SetNamespace records the validated namespace of the request for its audit
// entry. It is a no-op outside of Middleware.
func SetNamespace(ctx context.Context, ns string) {
	if a, ok := ctx.Value(annotationKey{}).(*annotation); ok {
		a.mu.Lock()
		a.namespace = ns
		a.mu.Unlock()
	}
}

// maxLabelLen bounds identity and entity labels in audit records.
const maxLabelLen = 128

// sanitizeLabel makes a label that may come from an unauthenticated request
// (the login name of a failed attempt) safe for the audit trail: bounded, and
// without control characters that could forge lines in line-based sinks.
func sanitizeLabel(s string) string {
	if len(s) > maxLabelLen {
		s = s[:maxLabelLen]
		for len(s) > 0 && !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029 {
			return '?'
		}
		return r
	}, s)
}

// Middleware wraps handler h, writing one audit entry per request.
func Middleware(l Loggable, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &recorder{ResponseWriter: w, status: http.StatusOK}
		ann := &annotation{}
		r = r.WithContext(context.WithValue(r.Context(), annotationKey{}, ann))
		h.ServeHTTP(rw, r)

		ann.mu.Lock()
		ns, identity, entityID := ann.namespace, ann.identity, ann.entityID
		ann.mu.Unlock()

		tok := r.Header.Get("X-Tuck-Token")
		l.Log(Entry{
			Time:        start.UTC().Format(time.RFC3339Nano),
			RequestID:   r.Header.Get("X-Request-Id"),
			Method:      r.Method,
			Path:        r.URL.Path,
			Operation:   Operation(r.Method, r.URL.Path),
			Namespace:   ns,
			Status:      rw.status,
			ClientIP:    ClientIP(r),
			Fingerprint: Fingerprint(tok),
			Identity:    identity,
			EntityID:    entityID,
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

// Flush lets streaming handlers (SSE) push data through the recorder.
func (r *recorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying writer to http.ResponseController
// (deadlines, flushing).
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
