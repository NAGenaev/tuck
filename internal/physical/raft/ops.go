package raft

import (
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	hraft "github.com/hashicorp/raft"
)

var (
	// ErrInvalidTransferTarget is returned when the requested new leader is
	// unknown, not a voter, or is this node.
	ErrInvalidTransferTarget = errors.New("raft: invalid leadership transfer target")
	// ErrNoOtherVoter is returned when this node is the only voter, so there is
	// nobody to hand leadership to.
	ErrNoOtherVoter = errors.New("raft: no other voter to transfer leadership to")
	// ErrNoNewData is returned by CreateSnapshot when nothing changed since the
	// latest stored snapshot.
	ErrNoNewData = hraft.ErrNothingNewToSnapshot
	// ErrSnapshotNotFound is returned for an unknown snapshot ID.
	ErrSnapshotNotFound = errors.New("raft: snapshot not found")
)

// StepDown hands leadership to another voter and returns once the transfer has
// finished (or failed). With an empty targetID the library picks the most
// up-to-date follower. Must be called on the leader.
func (b *Backend) StepDown(targetID string) error {
	if !b.IsLeader() {
		return ErrNotLeader
	}
	if targetID == b.nodeID {
		return fmt.Errorf("%w: the target is the current leader", ErrInvalidTransferTarget)
	}
	f := b.raft.GetConfiguration()
	if err := f.Error(); err != nil {
		return err
	}
	var target *hraft.Server
	others := 0
	for _, srv := range f.Configuration().Servers {
		srv := srv
		if string(srv.ID) != b.nodeID && srv.Suffrage == hraft.Voter {
			others++
		}
		if targetID != "" && string(srv.ID) == targetID {
			target = &srv
		}
	}
	if others == 0 {
		return ErrNoOtherVoter
	}
	if targetID == "" {
		return b.raft.LeadershipTransfer().Error()
	}
	if target == nil {
		return fmt.Errorf("%w: %q is not in the cluster configuration", ErrInvalidTransferTarget, targetID)
	}
	if target.Suffrage != hraft.Voter {
		return fmt.Errorf("%w: %q is not a voter", ErrInvalidTransferTarget, targetID)
	}
	return b.raft.LeadershipTransferToServer(target.ID, target.Address).Error()
}

// SnapshotInfo describes a snapshot stored in this node's snapshot directory.
type SnapshotInfo struct {
	ID    string `json:"id"`
	Index uint64 `json:"index"`
	Term  uint64 `json:"term"`
	Size  int64  `json:"size"`
	// CreatedAt is the creation time encoded by the snapshot store in the ID
	// (RFC 3339, UTC); empty when the ID does not carry one.
	CreatedAt string `json:"created_at,omitempty"`
	// Voters is the number of voters in the configuration at that index.
	Voters int `json:"voters"`
}

func snapshotInfo(m *hraft.SnapshotMeta) SnapshotInfo {
	info := SnapshotInfo{ID: m.ID, Index: m.Index, Term: m.Term, Size: m.Size}
	if i := strings.LastIndexByte(m.ID, '-'); i >= 0 {
		if ms, err := strconv.ParseInt(m.ID[i+1:], 10, 64); err == nil && ms > 0 {
			info.CreatedAt = time.UnixMilli(ms).UTC().Format(time.RFC3339)
		}
	}
	for _, s := range m.Configuration.Servers {
		if s.Suffrage == hraft.Voter {
			info.Voters++
		}
	}
	return info
}

// ListSnapshots returns the snapshots kept in this node's snapshot directory,
// newest first. The store retains the three most recent.
func (b *Backend) ListSnapshots() ([]SnapshotInfo, error) {
	metas, err := b.snaps.List()
	if err != nil {
		return nil, fmt.Errorf("raft: list snapshots: %w", err)
	}
	out := make([]SnapshotInfo, 0, len(metas))
	for _, m := range metas {
		out = append(out, snapshotInfo(m))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Index != out[j].Index {
			return out[i].Index > out[j].Index
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

// CreateSnapshot takes a snapshot of the FSM now and stores it in this node's
// snapshot directory. It works on any node. When nothing changed since the
// last snapshot it returns ErrNoNewData.
func (b *Backend) CreateSnapshot() (SnapshotInfo, error) {
	// The library snapshots even when the newest stored snapshot is already at
	// the applied index; that only burns one of the retained slots.
	if existing, err := b.ListSnapshots(); err == nil && len(existing) > 0 && existing[0].Index >= b.raft.AppliedIndex() {
		return SnapshotInfo{}, ErrNoNewData
	}
	f := b.raft.Snapshot()
	if err := f.Error(); err != nil {
		return SnapshotInfo{}, err
	}
	meta, rc, err := f.Open()
	if err != nil {
		return SnapshotInfo{}, fmt.Errorf("raft: open new snapshot: %w", err)
	}
	_ = rc.Close()
	return snapshotInfo(meta), nil
}

// OpenSnapshot opens a stored snapshot for download. The stream has the same
// format as Backend.Snapshot. The ID must name a snapshot returned by
// ListSnapshots.
func (b *Backend) OpenSnapshot(id string) (SnapshotInfo, io.ReadCloser, error) {
	metas, err := b.snaps.List()
	if err != nil {
		return SnapshotInfo{}, nil, fmt.Errorf("raft: list snapshots: %w", err)
	}
	for _, m := range metas {
		if m.ID != id {
			continue
		}
		meta, rc, err := b.snaps.Open(m.ID)
		if err != nil {
			return SnapshotInfo{}, nil, fmt.Errorf("raft: open snapshot: %w", err)
		}
		return snapshotInfo(meta), rc, nil
	}
	return SnapshotInfo{}, nil, ErrSnapshotNotFound
}

// TLSInfo describes the certificate this node presents on the Raft transport.
type TLSInfo struct {
	Subject     string   `json:"subject,omitempty"`
	Issuer      string   `json:"issuer,omitempty"`
	Serial      string   `json:"serial,omitempty"`
	NotBefore   string   `json:"not_before,omitempty"`
	NotAfter    string   `json:"not_after,omitempty"`
	DaysLeft    *int     `json:"days_left,omitempty"`
	DNSNames    []string `json:"dns_names,omitempty"`
	IPAddresses []string `json:"ip_addresses,omitempty"`
	ServerName  string   `json:"server_name,omitempty"`
	// CACerts is the number of certificates in the trusted CA bundle.
	CACerts int `json:"ca_certs"`
	// Error is set when the certificate file could not be read or parsed.
	Error string `json:"error,omitempty"`
}

type tlsCacheEntry struct {
	sig  string
	info TLSInfo
}

// tlsInfo reports the node certificate of the Raft transport, or nil when the
// transport is plain TCP. Parsed results are cached until the files change
// (the transport itself reloads rotated certificates the same way).
func (b *Backend) tlsInfo() *TLSInfo {
	c := b.tlsCfg
	if c == nil {
		return nil
	}
	sig := ""
	for _, p := range []string{c.CertFile, c.CAFile} {
		if fi, err := os.Stat(p); err == nil {
			sig += fmt.Sprintf("%d:%d;", fi.ModTime().UnixNano(), fi.Size())
		}
	}
	b.tlsMu.Lock()
	defer b.tlsMu.Unlock()
	if b.tlsCache == nil || b.tlsCache.sig != sig {
		b.tlsCache = &tlsCacheEntry{sig: sig, info: readTLSInfo(c)}
	}
	info := b.tlsCache.info
	if info.NotAfter != "" {
		if na, err := time.Parse(time.RFC3339, info.NotAfter); err == nil {
			d := int(time.Until(na).Hours() / 24)
			info.DaysLeft = &d
		}
	}
	return &info
}

func readTLSInfo(c *TLSConfig) TLSInfo {
	info := TLSInfo{ServerName: c.ServerName}
	if pemData, err := os.ReadFile(c.CAFile); err == nil {
		for rest := pemData; ; {
			var blk *pem.Block
			blk, rest = pem.Decode(rest)
			if blk == nil {
				break
			}
			if blk.Type == "CERTIFICATE" {
				info.CACerts++
			}
		}
	}
	data, err := os.ReadFile(c.CertFile)
	if err != nil {
		info.Error = "certificate file is not readable"
		return info
	}
	var leaf *x509.Certificate
	for rest := data; leaf == nil; {
		var blk *pem.Block
		blk, rest = pem.Decode(rest)
		if blk == nil {
			break
		}
		if blk.Type == "CERTIFICATE" {
			leaf, err = x509.ParseCertificate(blk.Bytes)
			if err != nil {
				info.Error = "certificate could not be parsed"
				return info
			}
		}
	}
	if leaf == nil {
		info.Error = "no certificate found in file"
		return info
	}
	info.Subject = leaf.Subject.String()
	info.Issuer = leaf.Issuer.String()
	info.Serial = hex.EncodeToString(leaf.SerialNumber.Bytes())
	info.NotBefore = leaf.NotBefore.UTC().Format(time.RFC3339)
	info.NotAfter = leaf.NotAfter.UTC().Format(time.RFC3339)
	info.DNSNames = leaf.DNSNames
	for _, ip := range leaf.IPAddresses {
		info.IPAddresses = append(info.IPAddresses, ip.String())
	}
	return info
}
