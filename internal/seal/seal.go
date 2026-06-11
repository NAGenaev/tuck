// Package seal provides the unseal strategies that protect Tuck's root key.
//
// The root key is what unlocks the barrier. A Seal decides where that key comes
// from: held on disk (dev), split via Shamir, or wrapped by a cloud KMS
// (auto-unseal). Removing the manual-unseal ceremony is Tuck's headline
// difference from Vault — so production seals are KMS-first by design.
package seal

// InitResult is the value returned by Seal.Init.
//
// For most seals only RootKey is populated. ShamirSeal additionally populates
// Shares so the operator can distribute them before the server starts accepting
// traffic.
type InitResult struct {
	// RootKey is the raw 32-byte root key that was just generated and stored
	// (or wrapped). Pass it to barrier.Initialize + barrier.Unseal on first boot.
	RootKey []byte

	// Shares is non-nil only for ShamirSeal. Each element is a base64url-encoded
	// share string that must be distributed to a different operator. The slice
	// length equals the total number of shares (N), not the threshold (K).
	Shares []string
}

// Seal produces the root key used to unseal the barrier.
type Seal interface {
	// Init generates and persists a fresh root key (or wraps it with a KMS),
	// returning an InitResult. Called exactly once, when a backend is first
	// initialized. For ShamirSeal this also splits the key and returns the
	// shares so they can be printed / distributed immediately.
	Init() (*InitResult, error)

	// Unseal returns the root key. For auto-unseal seals (Dev, Transit) this
	// returns the key directly. For interactive seals (ShamirSeal) this returns
	// ErrNeedsShards — the caller must use SharableUnseal.AcceptShard instead.
	Unseal() ([]byte, error)

	// Type names the seal strategy, used in logs and /v1/sys/seal-status.
	Type() string
}

// SharableUnseal is implemented by seals that require operators to supply key
// shards interactively before the barrier can be unsealed. The typical
// implementation is ShamirSeal.
//
// Core.Start() checks for this interface after a non-nil ErrNeedsShards from
// Unseal(). The HTTP layer then surfaces AcceptShard via POST /v1/sys/unseal.
type SharableUnseal interface {
	// AcceptShard registers one base64url-encoded shard. Returns complete=true
	// and the reconstructed root key once the threshold is reached, at which
	// point the internal shard buffer is zeroed. Subsequent calls return an
	// error.
	AcceptShard(share string) (complete bool, rootKey []byte, err error)

	// ShardsProgress reports how many shards are required (threshold K) and
	// how many have been received so far.
	ShardsProgress() (required, received int)
}
