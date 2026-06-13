package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

	return &RotatingFileLogger{
		path:       path,
		maxSize:    maxSize,
		maxBackups: maxBackups,
		logger:     NewLogger(f),
		size:       sz,
	}, nil
}

// Log writes one entry, rotating the file if it has reached MaxSize.
func (rl *RotatingFileLogger) Log(e Entry) {
	rl.mu.Lock()
	if rl.size >= rl.maxSize {
		_ = rl.rotateLocked()
	}
	l := rl.logger
	rl.size += 512 // conservative estimate per JSON line
	rl.mu.Unlock()

	l.Log(e)
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
