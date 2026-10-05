// Package barrier is Tuck's cryptographic core. It sits between the logical
// layer and the physical storage backend: everything written through the
// barrier is encrypted at rest, and nothing can be read until the barrier is
// unsealed with the root key.
//
// Key hierarchy (envelope encryption):
//
//	root key  --(AEAD)-->  barrier key (DEK)  --(AEAD)-->  secret data
//
// The AEAD is a pluggable cipher suite chosen when the store is initialised
// (see Suite): AES-256-GCM by default, or GOST R 34.12-2015 "Kuznyechik" in
// MGM mode. The root key lives only in memory and is supplied by a Seal at
// unseal time. The barrier key is generated once at Initialize, wrapped with
// the root key, and stored in the backend as the "keyring". Secret values are
// encrypted with the barrier key. To rotate the root key later you only
// re-wrap the keyring — you never have to re-encrypt the data.
//
// Stores created by this version bind every ciphertext to its storage key as
// AEAD associated data, so a ciphertext cannot be moved to another key.
// Stores created before cipher suites existed have no metadata record and are
// read as AES-256-GCM without associated data.
package barrier

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/NAGenaev/tuck/internal/physical"
)

// keyringPath is where the wrapped barrier key lives in the backend. The
// "barrier/" prefix is reserved for internal use and must not hold secret data.
const keyringPath = "barrier/keyring"

const keySize = suiteKeySize

// Errors returned by the barrier.
var (
	ErrSealed         = errors.New("barrier is sealed")
	ErrAlreadyInit    = errors.New("barrier is already initialized")
	ErrNotInitialized = errors.New("barrier is not initialized")
	ErrUnsealFailed   = errors.New("unseal failed: wrong root key or corrupt keyring")
)

// Barrier is the encryption layer over a physical.Backend.
type Barrier struct {
	backend physical.Backend

	mu      sync.RWMutex
	key     []byte // barrier key (DEK); nil when sealed
	suite   Suite  // cipher suite protecting this store
	bindAAD bool   // authenticate the storage key as AEAD associated data
}

// New creates a Barrier over the given backend. It starts sealed.
func New(backend physical.Backend) *Barrier {
	return &Barrier{backend: backend}
}

// Initialized reports whether a keyring already exists in the backend.
func (b *Barrier) Initialized(ctx context.Context) (bool, error) {
	e, err := b.backend.Get(ctx, keyringPath)
	if err != nil {
		return false, err
	}
	return e != nil, nil
}

// Initialize generates a fresh barrier key, wraps it with rootKey, and
// persists it using the default cipher suite. It must be called exactly once
// for the life of a backend.
func (b *Barrier) Initialize(ctx context.Context, rootKey []byte) error {
	return b.InitializeWithSuite(ctx, rootKey, "")
}

// InitializeWithSuite is Initialize with an explicit cipher suite name
// ("" = aes-256-gcm). The choice is recorded in the store and cannot change
// afterwards.
func (b *Barrier) InitializeWithSuite(ctx context.Context, rootKey []byte, suiteName string) error {
	suite, err := LookupSuite(suiteName)
	if err != nil {
		return err
	}
	if len(rootKey) != keySize {
		return fmt.Errorf("root key must be %d bytes, got %d", keySize, len(rootKey))
	}
	inited, err := b.Initialized(ctx)
	if err != nil {
		return err
	}
	if inited {
		return ErrAlreadyInit
	}

	barrierKey := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, barrierKey); err != nil {
		return fmt.Errorf("generate barrier key: %w", err)
	}
	wrapped, err := suite.seal(rootKey, barrierKey, []byte(keyringPath))
	if err != nil {
		return fmt.Errorf("wrap barrier key: %w", err)
	}
	meta, err := json.Marshal(storeMeta{Version: metaVersionAADBinding, Cipher: suite.Name})
	if err != nil {
		return err
	}
	// Metadata first: the keyring's existence is what marks the store initialised.
	if err := b.backend.Put(ctx, &physical.Entry{Key: metaPath, Value: meta}); err != nil {
		return fmt.Errorf("write barrier metadata: %w", err)
	}
	return b.backend.Put(ctx, &physical.Entry{Key: keyringPath, Value: wrapped})
}

// Unseal loads the keyring, unwraps the barrier key with rootKey, and holds it
// in memory so reads and writes can proceed.
func (b *Barrier) Unseal(ctx context.Context, rootKey []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.key != nil {
		return nil // already unsealed
	}
	e, err := b.backend.Get(ctx, keyringPath)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotInitialized
	}
	suite, bindAAD, err := b.loadMeta(ctx)
	if err != nil {
		return err
	}
	var aad []byte
	if bindAAD {
		aad = []byte(keyringPath)
	}
	barrierKey, err := suite.open(rootKey, e.Value, aad)
	if err != nil {
		return ErrUnsealFailed
	}
	b.suite, b.bindAAD = suite, bindAAD
	b.key = barrierKey
	return nil
}

// Seal drops the barrier key from memory. Reads and writes then fail until the
// next Unseal.
func (b *Barrier) Seal() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.key != nil {
		zero(b.key)
		b.key = nil
	}
}

// IsSealed reports whether the barrier is currently sealed.
func (b *Barrier) IsSealed() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.key == nil
}

// CipherSuite reports the cipher suite protecting this store ("" until the
// barrier has been unsealed once).
func (b *Barrier) CipherSuite() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.suite.Name
}

// Get decrypts and returns the entry at key, or (nil, nil) if absent.
func (b *Barrier) Get(ctx context.Context, key string) (*physical.Entry, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.key == nil {
		return nil, ErrSealed
	}
	e, err := b.backend.Get(ctx, key)
	if err != nil || e == nil {
		return nil, err
	}
	plain, err := b.suite.open(b.key, e.Value, b.aadFor(key))
	if err != nil {
		return nil, fmt.Errorf("decrypt %q: %w", key, err)
	}
	return &physical.Entry{Key: key, Value: plain}, nil
}

// Put encrypts entry.Value with the barrier key and stores it.
func (b *Barrier) Put(ctx context.Context, entry *physical.Entry) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.key == nil {
		return ErrSealed
	}
	ct, err := b.suite.seal(b.key, entry.Value, b.aadFor(entry.Key))
	if err != nil {
		return err
	}
	return b.backend.Put(ctx, &physical.Entry{Key: entry.Key, Value: ct})
}

// Delete removes key from the backend.
func (b *Barrier) Delete(ctx context.Context, key string) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.key == nil {
		return ErrSealed
	}
	return b.backend.Delete(ctx, key)
}

// Rekey wraps the current barrier key (DEK) with newRootKey and persists the
// updated keyring. Use this for root-key rotation: the in-memory DEK is
// unchanged, so no data re-encryption is required.
func (b *Barrier) Rekey(ctx context.Context, newRootKey []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.key == nil {
		return ErrSealed
	}
	wrapped, err := b.suite.seal(newRootKey, b.key, b.aadFor(keyringPath))
	if err != nil {
		return fmt.Errorf("re-wrap barrier key: %w", err)
	}
	return b.backend.Put(ctx, &physical.Entry{Key: keyringPath, Value: wrapped})
}

// List returns keys with the given prefix from the backend.
// Requires the barrier to be unsealed.
func (b *Barrier) List(ctx context.Context, prefix string) ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.key == nil {
		return nil, ErrSealed
	}
	return b.backend.List(ctx, prefix)
}

// --- store metadata and crypto helpers ---

type storeMeta struct {
	Version int    `json:"version"`
	Cipher  string `json:"cipher"`
}

// loadMeta reads the store metadata. A store without a metadata record
// predates cipher suites: it is AES-256-GCM with no associated data.
func (b *Barrier) loadMeta(ctx context.Context) (Suite, bool, error) {
	e, err := b.backend.Get(ctx, metaPath)
	if err != nil {
		return Suite{}, false, err
	}
	if e == nil {
		s, err := LookupSuite(SuiteAES256GCM)
		return s, false, err
	}
	var m storeMeta
	if err := json.Unmarshal(e.Value, &m); err != nil {
		return Suite{}, false, fmt.Errorf("parse barrier metadata: %w", err)
	}
	s, err := LookupSuite(m.Cipher)
	if err != nil {
		return Suite{}, false, err
	}
	return s, m.Version >= metaVersionAADBinding, nil
}

func (b *Barrier) aadFor(key string) []byte {
	if b.bindAAD {
		return []byte(key)
	}
	return nil
}

// seal returns nonce||ciphertext||tag.
func (s Suite) seal(key, plaintext, aad []byte) ([]byte, error) {
	aead, err := s.New(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	if s.ClearNonceTopBit {
		nonce[0] &= 0x7f
	}
	return aead.Seal(nonce, nonce, plaintext, aad), nil
}

func (s Suite) open(key, data, aad []byte) ([]byte, error) {
	aead, err := s.New(key)
	if err != nil {
		return nil, err
	}
	ns := aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}
	return aead.Open(nil, data[:ns], data[ns:], aad)
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
