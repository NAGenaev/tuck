package seal

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/NAGenaev/tuck/internal/shamir"
)

// ErrNeedsShards is returned by ShamirSeal.Unseal() to signal that the server
// started successfully but is waiting for operators to supply shards via
// POST /v1/sys/unseal before the barrier can be opened.
var ErrNeedsShards = errors.New("shamir seal: barrier needs shards to unseal")

// ErrAlreadyUnsealed is returned by AcceptShard once the threshold has been
// reached and the shard buffer has been cleared.
var ErrAlreadyUnsealed = errors.New("shamir seal: already collected enough shards")

// shamirConfig is stored on disk so the server knows N and K on restart.
// The actual shares are NEVER written to disk — operators keep them.
type shamirConfig struct {
	N int `json:"n"` // total shares
	K int `json:"k"` // threshold
}

// ShamirSeal implements a k-of-n Shamir's Secret Sharing unseal strategy.
//
// On Init the root key is split into N shares which are returned via
// InitResult.Shares. The server starts sealed on every boot and operators must
// call AcceptShard K times before the barrier opens.
//
// The config file (configPath) stores only N and K — no key material ever
// touches disk.
type ShamirSeal struct {
	configPath string
	n, k       int

	mu       sync.Mutex
	received [][]byte // accumulated raw shares; cleared after threshold reached
	done     bool     // true once threshold reached and shard buffer zeroed
}

// NewShamir creates a ShamirSeal. configPath is where N/K are persisted between
// restarts. n is the total number of shares; k is the reconstruction threshold.
func NewShamir(configPath string, n, k int) (*ShamirSeal, error) {
	if k < 2 || k > n || n > 255 {
		return nil, fmt.Errorf("shamir seal: invalid n=%d k=%d (need 2 <= k <= n <= 255)", n, k)
	}
	return &ShamirSeal{
		configPath: configPath,
		n:          n,
		k:          k,
	}, nil
}

// NewShamirFromConfig loads N and K from a previously written config file.
// Use this on restarts when N and K are not available from flags.
func NewShamirFromConfig(configPath string) (*ShamirSeal, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("shamir seal: read config: %w", err)
	}
	var cfg shamirConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("shamir seal: parse config: %w", err)
	}
	return NewShamir(configPath, cfg.N, cfg.K)
}

func (s *ShamirSeal) Type() string { return "shamir" }

// N returns the total number of shares this seal was configured with.
func (s *ShamirSeal) N() int { return s.n }

// K returns the minimum number of shares required to unseal.
func (s *ShamirSeal) K() int { return s.k }

// Init generates a fresh root key, splits it into s.n shares, writes only the
// config (N/K) to disk, and returns all shares as base64url strings in
// InitResult.Shares.
//
// Each share encodes as base64url(raw_share_bytes) where raw_share_bytes is
// [x_coord, y_0, y_1, ..., y_{len-1}] as returned by shamir.Split.
func (s *ShamirSeal) Init() (*InitResult, error) {
	key := make([]byte, rootKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("shamir seal: generate root key: %w", err)
	}

	shares, err := shamir.Split(key, s.n, s.k)
	if err != nil {
		return nil, fmt.Errorf("shamir seal: split: %w", err)
	}

	// Encode each raw share (already [x, y_0..y_{n-1}]) as base64url.
	encoded := make([]string, len(shares))
	for i, sh := range shares {
		encoded[i] = base64.RawURLEncoding.EncodeToString(sh)
	}

	// Persist only N/K — never the key or shares.
	cfg := shamirConfig{N: s.n, K: s.k}
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("shamir seal: marshal config: %w", err)
	}
	if err := os.WriteFile(s.configPath, cfgBytes, 0600); err != nil {
		return nil, fmt.Errorf("shamir seal: write config: %w", err)
	}

	return &InitResult{RootKey: key, Shares: encoded}, nil
}

// Unseal always returns ErrNeedsShards — operators must supply shards via
// AcceptShard. Core.Start detects this sentinel and enters the "waiting for
// shards" state.
func (s *ShamirSeal) Unseal() ([]byte, error) {
	return nil, ErrNeedsShards
}

// AcceptShard decodes a base64url-encoded share string, accumulates it, and
// reconstructs the root key once the threshold K is reached. After
// reconstruction the shard buffer is zeroed to minimise key-material exposure.
//
// Returns (false, nil, nil) while still collecting shards.
// Returns (true, rootKey, nil) when the threshold is met.
// Returns an error on bad encoding, duplicate x-coordinate, or if already done.
func (s *ShamirSeal) AcceptShard(share string) (bool, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done {
		return false, nil, ErrAlreadyUnsealed
	}

	// Decode the base64url share: [x_coord, y_0, ..., y_{n-1}]
	raw, err := base64.RawURLEncoding.DecodeString(share)
	if err != nil {
		return false, nil, fmt.Errorf("shamir seal: decode share: %w", err)
	}
	if len(raw) < 2 {
		return false, nil, fmt.Errorf("shamir seal: share payload too short (%d bytes)", len(raw))
	}

	x := raw[0]
	// Reject duplicate x-coordinates early for a cleaner error message.
	for _, existing := range s.received {
		if existing[0] == x {
			return false, nil, fmt.Errorf("shamir seal: duplicate shard (x=%d already received)", x)
		}
	}

	// Copy the share so the caller's buffer can be freed.
	sh := make([]byte, len(raw))
	copy(sh, raw)
	s.received = append(s.received, sh)

	if len(s.received) < s.k {
		return false, nil, nil // still waiting
	}

	// Threshold reached — reconstruct the root key.
	key, err := shamir.Combine(s.received[:s.k])
	if err != nil {
		return false, nil, fmt.Errorf("shamir seal: combine: %w", err)
	}

	// Zero and discard the buffered shards.
	for i := range s.received {
		for j := range s.received[i] {
			s.received[i][j] = 0
		}
	}
	s.received = nil
	s.done = true

	return true, key, nil
}

// ShardsProgress returns (K, received) — the threshold and how many shards
// have been accepted so far in this unseal attempt.
func (s *ShamirSeal) ShardsProgress() (required, received int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.k, len(s.received)
}
