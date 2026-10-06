package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultMaxSize    = 100 << 20 // 100 MiB
	DefaultMaxBackups = 7
)

// RotatingFileLogger wraps Logger with size-based rotation.
// When the active file exceeds MaxSize the file is renamed to
// <base>.<timestamp>.log and a fresh Logger is started.
// At most MaxBackups old files are kept; older ones are deleted.
type RotatingFileLogger struct {
	mu         sync.Mutex
	path       string
	maxSize    int64
	maxBackups int
	logger     *Logger
	size       int64
	total      atomic.Int64 // bytes written across all rotated files
	lastHash   string       // hash/seq of the last record found on disk at open
	lastSeq    uint64
}

// NewRotatingFileLogger creates a RotatingFileLogger.
// maxSize 0 → DefaultMaxSize; maxBackups 0 → DefaultMaxBackups.
func NewRotatingFileLogger(path string, maxSize int64, maxBackups int) (*RotatingFileLogger, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	if maxBackups <= 0 {
		maxBackups = DefaultMaxBackups
	}

	f, sz, err := openAppend(path)
	if err != nil {
		return nil, err
	}

	rl := &RotatingFileLogger{
		path:       path,
		maxSize:    maxSize,
		maxBackups: maxBackups,
		logger:     NewLogger(f),
		size:       sz,
	}
	rl.lastHash, rl.lastSeq = readTail(path)
	return rl, nil
}

// LastHash returns the hash and sequence number of the last record that was
// already in the file when it was opened ("" , 0 for a fresh file). The
// Dispatcher uses it to continue the chain across restarts.
func (rl *RotatingFileLogger) LastHash() (string, uint64) { return rl.lastHash, rl.lastSeq }

// Path returns the path of the active log file.
func (rl *RotatingFileLogger) Path() string { return rl.path }

// BytesWritten returns the bytes written since open (across rotations).
func (rl *RotatingFileLogger) BytesWritten() int64 { return rl.total.Load() }

// Log writes one entry, rotating the file if it has reached MaxSize.
func (rl *RotatingFileLogger) Log(e Entry) { _, _ = rl.Write(e) }

// Write is Log that reports the bytes written and any write error.
func (rl *RotatingFileLogger) Write(e Entry) (int, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.size >= rl.maxSize {
		_ = rl.rotateLocked()
	}
	n, err := rl.logger.Write(e)
	rl.size += int64(n)
	rl.total.Add(int64(n))
	return n, err
}

// rotateLocked must be called with rl.mu held.
func (rl *RotatingFileLogger) rotateLocked() error {
	// Close old writer if possible.
	if c, ok := rl.logger.w.(interface{ Close() error }); ok {
		_ = c.Close()
	}

	ts := time.Now().UTC().Format("20060102T150405")
	dst := strings.TrimSuffix(rl.path, filepath.Ext(rl.path)) + "." + ts + ".log"
	_ = os.Rename(rl.path, dst)

	f, sz, err := openAppend(rl.path)
	if err != nil {
		return err
	}
	rl.logger = NewLogger(f) // new hash chain in new file
	rl.size = sz

	go rl.purgeOldFiles()
	return nil
}

func (rl *RotatingFileLogger) purgeOldFiles() {
	base := strings.TrimSuffix(rl.path, filepath.Ext(rl.path))
	glob := base + ".????????T??????.log"
	matches, err := filepath.Glob(glob)
	if err != nil || len(matches) <= rl.maxBackups {
		return
	}
	sort.Strings(matches)
	for _, old := range matches[:len(matches)-rl.maxBackups] {
		_ = os.Remove(old)
	}
}

// Close flushes and closes the active log file.
func (rl *RotatingFileLogger) Close() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if c, ok := rl.logger.w.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}

func openAppend(path string) (*os.File, int64, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 — operator-configured audit log path
	if err != nil {
		return nil, 0, fmt.Errorf("audit: open %q: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// readTail returns the hash and seq of the last complete JSON record of the
// file at path, or ("", 0) when there is none / it cannot be parsed.
func readTail(path string) (string, uint64) {
	f, err := os.Open(path) // #nosec G304 — operator-configured audit log path
	if err != nil {
		return "", 0
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return "", 0
	}
	const window = 64 << 10
	off := info.Size() - window
	if off < 0 {
		off = 0
	}
	buf := make([]byte, info.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return "", 0
	}
	// Only complete lines count; a trailing partial line is ignored.
	if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
		buf = buf[:i]
	} else {
		return "", 0
	}
	if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
		buf = buf[i+1:]
	} else if off > 0 {
		return "", 0 // the only line in the window is cut at its start
	}
	var e Entry
	if json.Unmarshal(buf, &e) != nil || len(e.Hash) != 64 {
		return "", 0
	}
	return e.Hash, e.Seq
}
