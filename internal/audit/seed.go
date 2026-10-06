package audit

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
)

// readTailEntries returns up to maxEntries of the newest parseable records of
// the file at path (oldest → newest), reading at most the last window bytes.
func readTailEntries(path string, window int64, maxEntries int) []Entry {
	f, err := os.Open(path) // #nosec G304 — operator-configured audit log path
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return nil
	}
	off := info.Size() - window
	if off < 0 {
		off = 0
	}
	buf := make([]byte, info.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return nil
	}
	if off > 0 { // the first line is cut at its start
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		} else {
			return nil
		}
	}
	var out []Entry
	for _, line := range bytes.Split(buf, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(line, &e) != nil || e.Hash == "" {
			continue
		}
		out = append(out, e)
	}
	if maxEntries > 0 && len(out) > maxEntries {
		out = out[len(out)-maxEntries:]
	}
	return out
}
