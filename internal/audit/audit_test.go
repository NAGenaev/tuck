package audit_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/NAGenaev/tuck/internal/audit"
)

// TestHashChain verifies that:
//  1. The first entry's PrevHash is the genesis hash (64 zeros).
//  2. The second entry's PrevHash equals the first entry's Hash.
//  3. Each entry's Hash is the SHA-256 of its own JSON with the Hash field empty.
func TestHashChain(t *testing.T) {
	var buf bytes.Buffer
	l := audit.NewLogger(&buf)

	l.Log(audit.Entry{
		Time:   "2026-01-01T00:00:00Z",
		Method: "GET",
		Path:   "/v1/secret/foo",
		Status: 200,
	})
	l.Log(audit.Entry{
		Time:   "2026-01-01T00:00:01Z",
		Method: "PUT",
		Path:   "/v1/secret/bar",
		Status: 204,
	})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}

	var e1, e2 audit.Entry
	if err := json.Unmarshal([]byte(lines[0]), &e1); err != nil {
		t.Fatalf("unmarshal entry 1: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &e2); err != nil {
		t.Fatalf("unmarshal entry 2: %v", err)
	}

	// First entry's PrevHash must be 64 zeros (genesis).
	genesis := strings.Repeat("0", 64)
	if e1.PrevHash != genesis {
		t.Errorf("entry1 PrevHash = %q, want %q", e1.PrevHash, genesis)
	}

	// Verify entry1's Hash is correct.
	if err := verifyHash(e1); err != nil {
		t.Errorf("entry1 hash mismatch: %v", err)
	}

	// Second entry's PrevHash must equal first entry's Hash.
	if e2.PrevHash != e1.Hash {
		t.Errorf("entry2 PrevHash = %q, want entry1 Hash %q", e2.PrevHash, e1.Hash)
	}

	// Verify entry2's Hash is correct.
	if err := verifyHash(e2); err != nil {
		t.Errorf("entry2 hash mismatch: %v", err)
	}
}

// verifyHash recomputes an entry's hash and compares it to the stored value.
func verifyHash(e audit.Entry) error {
	stored := e.Hash
	e.Hash = ""
	plain, err := json.Marshal(e)
	if err != nil {
		return err
	}
	h := sha256.Sum256(plain)
	computed := hex.EncodeToString(h[:])
	if computed != stored {
		return &hashMismatch{computed: computed, stored: stored}
	}
	return nil
}

type hashMismatch struct{ computed, stored string }

func (e *hashMismatch) Error() string {
	return "computed " + e.computed + " but stored " + e.stored
}

// TestFingerprint checks that the fingerprint function returns 12 hex chars
// and that an empty token ID returns an empty string.
func TestFingerprint(t *testing.T) {
	fp := audit.Fingerprint("some-token-id")
	if len(fp) != 12 {
		t.Errorf("Fingerprint length = %d, want 12", len(fp))
	}
	if audit.Fingerprint("") != "" {
		t.Error("Fingerprint(\"\") should return \"\"")
	}
}

func TestRotatingFileLogger(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/audit.log"

	// Set maxSize to 1 byte so every write triggers rotation.
	rl, err := audit.NewRotatingFileLogger(path, 1, 3)
	if err != nil {
		t.Fatalf("NewRotatingFileLogger: %v", err)
	}

	t.Cleanup(func() { _ = rl.Close() })

	for i := 0; i < 5; i++ {
		rl.Log(audit.Entry{
			Method: "GET",
			Path:   "/v1/health",
			Status: 200,
		})
	}

	// Active file must exist.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("active log file missing: %v", err)
	}
}
