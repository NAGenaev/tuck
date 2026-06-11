// Package seal provides the unseal strategies that protect Tuck's root key.
//
// The root key is what unlocks the barrier. A Seal decides where that key comes
// from: held on disk (dev), split via Shamir, or wrapped by a cloud KMS
// (auto-unseal). Removing the manual-unseal ceremony is Tuck's headline
// difference from Vault — so production seals are KMS-first by design.
// Milestone 0 ships only the dev seal.
package seal

// Seal produces the root key used to unseal the barrier.
type Seal interface {
	// Init generates and persists a fresh root key, returning it. Called once,
	// when a backend is first initialized.
	Init() ([]byte, error)
	// Unseal returns the root key, e.g. by reading it from disk or a KMS.
	Unseal() ([]byte, error)
	// Type names the seal strategy, for logging.
	Type() string
}
