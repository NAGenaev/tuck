// Package barrier is Tuck's cryptographic core. It sits between the logical
// layer and the physical storage backend: everything written through the
// barrier is encrypted at rest, and nothing can be read until the barrier is
// unsealed with the root key.
//
// Key hierarchy (envelope encryption):
//
//	root key  --(AES-GCM)-->  barrier key (DEK)  --(AES-GCM)-->  secret data
//
// The root key lives only in memory and is supplied by a Seal at unseal time.
// The barrier key is generated once at Initialize, wrapped with the root key,
// and stored in the backend as the "keyring". Secret values are encrypted with
// the barrier key. To rotate the root key later you only re-wrap the keyring —
// you never have to re-encrypt the data.
package barrier

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/NAGenaev/tuck/internal/physical"
)

// keyringPath is where the wrapped barrier key lives in the backend. The
// "barrier/" prefix is reserved for internal use and must not hold secret data.
const keyringPath = "barrier/keyring"

const keySize = 32 // AES-256

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

	mu  sync.RWMutex
	key []byte // barrier key (DEK); nil when sealed
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
// persists it. It must be called exactly once for the life of a backend.
func (b *Barrier) Initialize(ctx context.Context, rootKey []byte) error {
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
	wrapped, err := encrypt(rootKey, barrierKey)
	if err != nil {
		return fmt.Errorf("wrap barrier key: %w", err)
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
	barrierKey, err := decrypt(rootKey, e.Value)
	if err != nil {
		return ErrUnsealFailed
	}
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
	plain, err := decrypt(b.key, e.Value)
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
	ct, err := encrypt(b.key, entry.Value)
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

// --- crypto helpers: AES-256-GCM, nonce prefixed to the ciphertext ---

func encrypt(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// Seal appends the ciphertext to nonce, yielding nonce||ciphertext.
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decrypt(key, data []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := data[:ns], data[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
