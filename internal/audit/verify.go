package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// VerifyResult is the outcome of a hash-chain check.
type VerifyResult struct {
	OK bool `json:"ok"`
	// Checked is the number of records examined (up to and including the
	// first broken one).
	Checked int `json:"checked"`
	// FirstBroken is the 0-based index of the first record that fails, -1 when
	// the chain is intact.
	FirstBroken int    `json:"first_broken"`
	BrokenSeq   uint64 `json:"broken_seq,omitempty"`
	Reason      string `json:"reason,omitempty"`
	// Legacy counts records written before sequence numbers existed; their
	// hashes are still checked, but a deleted legacy record cannot be told
	// apart by sequence number (the prev_hash link still catches it).
	Legacy int `json:"legacy,omitempty"`
	// FromGenesis is true when the first checked record links to the genesis
	// hash, i.e. the checked range starts at the very beginning of a chain.
	FromGenesis bool   `json:"from_genesis"`
	HeadHash    string `json:"head_hash,omitempty"`
}

// VerifyEntries checks a consecutive range of records: every record must
// hash to its own Hash, link to its predecessor via PrevHash and, where
// sequence numbers exist, follow it without gaps. The first record of the range
// is anchored to anchor when anchor is non-empty, and is otherwise trusted as
// the start of the range (its own hash is still verified).
func VerifyEntries(entries []Entry, anchor string) VerifyResult {
	res := VerifyResult{OK: true, FirstBroken: -1}
	var prev *Entry
	for i := range entries {
		e := entries[i]
		res.Checked++
		if e.Seq == 0 {
			res.Legacy++
		}
		fail := func(reason string) VerifyResult {
			res.OK, res.FirstBroken, res.BrokenSeq, res.Reason = false, i, e.Seq, reason
			return res
		}
		if got := ComputeHash(e); got == "" || got != e.Hash {
			return fail("record hash does not match its content")
		}
		switch {
		case prev != nil:
			if e.PrevHash != prev.Hash {
				return fail("prev_hash does not match the previous record")
			}
			if prev.Seq != 0 && e.Seq != 0 && e.Seq != prev.Seq+1 {
				return fail(fmt.Sprintf("sequence gap: %d follows %d", e.Seq, prev.Seq))
			}
		case anchor != "":
			if e.PrevHash != anchor {
				return fail("first record does not link to the expected anchor")
			}
		}
		if i == 0 {
			res.FromGenesis = e.PrevHash == GenesisHash
		}
		prev = &entries[i]
		res.HeadHash = e.Hash
	}
	return res
}

// maxVerifyLine bounds one JSON line when verifying a file.
const maxVerifyLine = 1 << 20

// VerifyFile checks the hash chain of the audit log file at path. A line that
// is not valid JSON counts as a broken record. The caller is responsible for
// path being trusted: use Dispatcher.VerifyFileSink for client-driven calls.
func VerifyFile(path string) (VerifyResult, error) {
	f, err := os.Open(path) // #nosec G304 — path comes from operator config, never from a request
	if err != nil {
		return VerifyResult{}, fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = f.Close() }()

	res := VerifyResult{OK: true, FirstBroken: -1}
	br := bufio.NewReaderSize(f, 64<<10)

	var prev *Entry
	idx := 0
	for {
		line, rerr := br.ReadBytes('\n')
		if rerr != nil {
			// A final line without its newline is a write in progress (the
			// logger is appending concurrently): it is not part of the log yet.
			if rerr == io.EOF {
				break
			}
			res.OK, res.FirstBroken, res.Reason = false, idx, "unreadable record: "+rerr.Error()
			res.Checked++
			return res, nil
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 {
			continue
		}
		if len(line) > maxVerifyLine {
			res.Checked++
			res.OK, res.FirstBroken, res.Reason = false, idx, "record is too large"
			return res, nil
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			res.Checked++
			res.OK, res.FirstBroken, res.Reason = false, idx, "line is not a valid audit record"
			return res, nil
		}
		// Verify one record at a time so memory stays flat on large files.
		var window []Entry
		if prev != nil {
			window = []Entry{*prev, e}
		} else {
			window = []Entry{e}
		}
		r := VerifyEntries(window, "")
		if !r.OK {
			res.Checked++
			res.OK, res.FirstBroken, res.BrokenSeq, res.Reason = false, idx, e.Seq, r.Reason
			return res, nil
		}
		if idx == 0 {
			res.FromGenesis = e.PrevHash == GenesisHash
		}
		if e.Seq == 0 {
			res.Legacy++
		}
		res.Checked++
		res.HeadHash = e.Hash
		cp := e
		prev = &cp
		idx++
	}
	return res, nil
}
