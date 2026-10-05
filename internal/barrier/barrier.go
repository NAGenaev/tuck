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
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
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

	mu        sync.RWMutex
	keys      map[uint32][]byte // barrier keys (DEKs) by version; nil when sealed
	active    uint32            // version used for new writes
	rootKey   []byte            // root key, held while unsealed so the keyring can be re-wrapped
	suite     Suite             // cipher suite protecting this store
	bindAAD   bool              // authenticate the storage key as AEAD associated data
	versioned bool              // entries carry a 4-byte DEK version prefix
	pathKey   []byte            // root secret for path encryption (nil = keys stored in the clear)
	paths     *pathCodec        // non-nil when storage keys are encrypted
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
	return b.initialize(ctx, rootKey, suiteName, metaVersionPathEncryption)
}

// initialize writes a new store at the given metadata version (older
// versions exist so tests can build stores in earlier formats).
func (b *Barrier) initialize(ctx context.Context, rootKey []byte, suiteName string, metaVersion int) error {
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
	var pathKey []byte
	if metaVersion >= metaVersionPathEncryption {
		pathKey = make([]byte, keySize)
		if _, err := io.ReadFull(rand.Reader, pathKey); err != nil {
			return fmt.Errorf("generate path key: %w", err)
		}
	}
	kr, err := marshalKeyring(1, map[uint32][]byte{1: barrierKey}, pathKey)
	if err != nil {
		return err
	}
	wrapped, err := suite.seal(rootKey, kr, []byte(keyringPath))
	if err != nil {
		return fmt.Errorf("wrap barrier key: %w", err)
	}
	meta, err := json.Marshal(storeMeta{Version: metaVersion, Cipher: suite.Name})
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
	if b.keys != nil {
		return nil // already unsealed
	}
	e, err := b.backend.Get(ctx, keyringPath)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotInitialized
	}
	suite, flags, err := b.loadMeta(ctx)
	if err != nil {
		return err
	}
	var aad []byte
	if flags.bindAAD {
		aad = []byte(keyringPath)
	}
	plain, err := suite.open(rootKey, e.Value, aad)
	if err != nil {
		return ErrUnsealFailed
	}
	keys, active, pathKey := map[uint32][]byte{0: plain}, uint32(0), []byte(nil)
	if flags.versioned {
		if keys, active, pathKey, err = parseKeyring(plain); err != nil {
			return ErrUnsealFailed
		}
	}
	var paths *pathCodec
	if flags.encPaths {
		if len(pathKey) != keySize {
			return ErrUnsealFailed
		}
		if paths, err = newPathCodec(suite, pathKey); err != nil {
			return err
		}
	}
	b.suite, b.bindAAD, b.versioned = suite, flags.bindAAD, flags.versioned
	b.pathKey, b.paths = pathKey, paths
	b.keys, b.active = keys, active
	b.rootKey = append([]byte(nil), rootKey...)
	return nil
}

// Seal drops the barrier key from memory. Reads and writes then fail until the
// next Unseal.
func (b *Barrier) Seal() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, k := range b.keys {
		zero(k)
	}
	zero(b.rootKey)
	zero(b.pathKey)
	if b.paths != nil {
		b.paths.zero()
	}
	b.keys, b.rootKey, b.pathKey, b.paths = nil, nil, nil, nil
}

// IsSealed reports whether the barrier is currently sealed.
func (b *Barrier) IsSealed() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.keys == nil
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
	if b.keys == nil {
		return nil, ErrSealed
	}
	pk := b.phys(key)
	e, err := b.backend.Get(ctx, pk)
	if err != nil || e == nil {
		return nil, err
	}
	plain, err := b.openEntry(pk, e.Value)
	if err != nil {
		return nil, fmt.Errorf("decrypt %q: %w", key, err)
	}
	return &physical.Entry{Key: key, Value: plain}, nil
}

// Put encrypts entry.Value with the barrier key and stores it.
func (b *Barrier) Put(ctx context.Context, entry *physical.Entry) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.keys == nil {
		return ErrSealed
	}
	pk := b.phys(entry.Key)
	ct, err := b.sealEntry(pk, entry.Value)
	if err != nil {
		return err
	}
	return b.backend.Put(ctx, &physical.Entry{Key: pk, Value: ct})
}

// Delete removes key from the backend.
func (b *Barrier) Delete(ctx context.Context, key string) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.keys == nil {
		return ErrSealed
	}
	return b.backend.Delete(ctx, b.phys(key))
}

// Rekey wraps the current barrier key (DEK) with newRootKey and persists the
// updated keyring. Use this for root-key rotation: the in-memory DEK is
// unchanged, so no data re-encryption is required.
func (b *Barrier) Rekey(ctx context.Context, newRootKey []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.keys == nil {
		return ErrSealed
	}
	if err := b.persistKeyring(ctx, newRootKey); err != nil {
		return err
	}
	zero(b.rootKey)
	b.rootKey = append([]byte(nil), newRootKey...)
	return nil
}

// List returns keys with the given prefix from the backend.
// Requires the barrier to be unsealed.
func (b *Barrier) List(ctx context.Context, prefix string) ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.keys == nil {
		return nil, ErrSealed
	}
	return b.listKeys(ctx, prefix)
}

// --- store metadata and crypto helpers ---

type storeMeta struct {
	Version int    `json:"version"`
	Cipher  string `json:"cipher"`
}

// loadMeta reads the store metadata. A store without a metadata record
// predates cipher suites: it is AES-256-GCM with no associated data.
type storeFlags struct{ bindAAD, versioned, encPaths bool }

func (b *Barrier) loadMeta(ctx context.Context) (Suite, storeFlags, error) {
	e, err := b.backend.Get(ctx, metaPath)
	if err != nil {
		return Suite{}, storeFlags{}, err
	}
	if e == nil {
		s, err := LookupSuite(SuiteAES256GCM)
		return s, storeFlags{}, err
	}
	var m storeMeta
	if err := json.Unmarshal(e.Value, &m); err != nil {
		return Suite{}, storeFlags{}, fmt.Errorf("parse barrier metadata: %w", err)
	}
	s, err := LookupSuite(m.Cipher)
	if err != nil {
		return Suite{}, storeFlags{}, err
	}
	return s, storeFlags{
		bindAAD:   m.Version >= metaVersionAADBinding,
		versioned: m.Version >= metaVersionVersionedKeys,
		encPaths:  m.Version >= metaVersionPathEncryption,
	}, nil
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

// --- versioned barrier keys ---

type keyringDoc struct {
	Active  uint32            `json:"active"`
	Keys    map[string][]byte `json:"keys"`
	PathKey []byte            `json:"path_key,omitempty"`
}

func marshalKeyring(active uint32, keys map[uint32][]byte, pathKey []byte) ([]byte, error) {
	doc := keyringDoc{Active: active, PathKey: pathKey, Keys: make(map[string][]byte, len(keys))}
	for v, k := range keys {
		doc.Keys[strconv.FormatUint(uint64(v), 10)] = k
	}
	return json.Marshal(doc)
}

func parseKeyring(raw []byte) (map[uint32][]byte, uint32, []byte, error) {
	var doc keyringDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, 0, nil, err
	}
	keys := make(map[uint32][]byte, len(doc.Keys))
	for s, k := range doc.Keys {
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil || len(k) != keySize {
			return nil, 0, nil, errors.New("malformed keyring")
		}
		keys[uint32(v)] = k
	}
	if _, ok := keys[doc.Active]; !ok {
		return nil, 0, nil, errors.New("keyring has no active key")
	}
	return keys, doc.Active, doc.PathKey, nil
}

// persistKeyring wraps the current keyring with rootKey and stores it.
// Caller holds b.mu for writing.
func (b *Barrier) persistKeyring(ctx context.Context, rootKey []byte) error {
	var plain []byte
	if b.versioned {
		var err error
		if plain, err = marshalKeyring(b.active, b.keys, b.pathKey); err != nil {
			return err
		}
		defer zero(plain)
	} else {
		plain = b.keys[b.active]
	}
	wrapped, err := b.suite.seal(rootKey, plain, b.aadFor(keyringPath))
	if err != nil {
		return fmt.Errorf("wrap keyring: %w", err)
	}
	return b.backend.Put(ctx, &physical.Entry{Key: keyringPath, Value: wrapped})
}

// sealEntry encrypts a value with the active barrier key. Caller holds b.mu.
func (b *Barrier) sealEntry(key string, value []byte) ([]byte, error) {
	ct, err := b.suite.seal(b.keys[b.active], value, b.aadFor(key))
	if err != nil || !b.versioned {
		return ct, err
	}
	out := make([]byte, 4+len(ct))
	binary.BigEndian.PutUint32(out, b.active)
	copy(out[4:], ct)
	return out, nil
}

func (b *Barrier) entryVersion(data []byte) (uint32, []byte, error) {
	if !b.versioned {
		return b.active, data, nil
	}
	if len(data) < 4 {
		return 0, nil, errors.New("ciphertext too short")
	}
	return binary.BigEndian.Uint32(data), data[4:], nil
}

// openEntry decrypts a stored value with whichever barrier key sealed it.
// Caller holds b.mu.
func (b *Barrier) openEntry(key string, data []byte) ([]byte, error) {
	ver, body, err := b.entryVersion(data)
	if err != nil {
		return nil, err
	}
	k, ok := b.keys[ver]
	if !ok {
		return nil, fmt.Errorf("unknown barrier key version %d", ver)
	}
	return b.suite.open(k, body, b.aadFor(key))
}

// KeyVersion reports the barrier key version used for new writes (0 for
// stores that predate key versioning).
func (b *Barrier) KeyVersion() uint32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.active
}

// ErrNotVersioned is returned by RotateDataKey on a store created before
// barrier keys were versioned.
var ErrNotVersioned = errors.New("barrier store predates key versioning; data key rotation unsupported")

// RotateDataKey generates a new barrier key (DEK) and makes it the active
// one: all subsequent writes use it, existing entries stay readable under
// their old key. When reencrypt is true every existing entry is then
// rewritten under the new key and the old keys are discarded. Reencryption
// blocks all barrier reads and writes for its duration. It returns the new
// key version and the number of entries rewritten.
func (b *Barrier) RotateDataKey(ctx context.Context, reencrypt bool) (uint32, int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.keys == nil {
		return 0, 0, ErrSealed
	}
	if !b.versioned {
		return 0, 0, ErrNotVersioned
	}
	nk := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, nk); err != nil {
		return 0, 0, fmt.Errorf("generate barrier key: %w", err)
	}
	newVer := b.active + 1
	b.keys[newVer] = nk
	prev := b.active
	b.active = newVer
	if err := b.persistKeyring(ctx, b.rootKey); err != nil {
		delete(b.keys, newVer)
		b.active = prev
		zero(nk)
		return 0, 0, err
	}
	if !reencrypt {
		return newVer, 0, nil
	}
	n, err := b.reencryptLocked(ctx)
	if err != nil {
		return newVer, n, err
	}
	for v, k := range b.keys {
		if v != b.active {
			zero(k)
			delete(b.keys, v)
		}
	}
	return newVer, n, b.persistKeyring(ctx, b.rootKey)
}

// reencryptLocked rewrites every entry not sealed under the active key.
func (b *Barrier) reencryptLocked(ctx context.Context) (int, error) {
	keys, err := b.backend.List(ctx, "")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, k := range keys {
		if strings.HasPrefix(k, "barrier/") {
			continue
		}
		e, err := b.backend.Get(ctx, k)
		if err != nil {
			return n, err
		}
		if e == nil {
			continue
		}
		ver, _, err := b.entryVersion(e.Value)
		if err != nil {
			return n, fmt.Errorf("reencrypt %q: %w", k, err)
		}
		if ver == b.active {
			continue
		}
		plain, err := b.openEntry(k, e.Value)
		if err != nil {
			return n, fmt.Errorf("reencrypt %q: %w", k, err)
		}
		ct, err := b.sealEntry(k, plain)
		if err != nil {
			return n, err
		}
		if err := b.backend.Put(ctx, &physical.Entry{Key: k, Value: ct}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
