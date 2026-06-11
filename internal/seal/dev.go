package seal

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
)

const rootKeySize = 32

// Dev is an auto-unseal Seal that stores the root key in plaintext on local
// disk. It exists so you can run `tuck` with zero ceremony during development.
//
// WARNING: insecure by design. Anyone who can read the key file can decrypt
// every secret. Never use the dev seal in production — that is what the KMS and
// Shamir seals are for.
type Dev struct {
	path string
}

// NewDev returns a dev seal that keeps its root key at path.
func NewDev(path string) *Dev {
	return &Dev{path: path}
}

func (d *Dev) Type() string { return "dev" }

// Init generates a fresh root key, writes it to disk, and returns it wrapped
// in an InitResult. Shares is always nil for the dev seal.
func (d *Dev) Init() (*InitResult, error) {
	if _, err := os.Stat(d.path); err == nil {
		return nil, fmt.Errorf("dev seal: key file %q already exists", d.path)
	}
	key := make([]byte, rootKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(d.path, key, 0600); err != nil {
		return nil, fmt.Errorf("dev seal: write key: %w", err)
	}
	return &InitResult{RootKey: key}, nil
}

// Unseal reads the root key from disk and returns it. For the dev seal this
// always succeeds as long as the key file exists and has the right size.
func (d *Dev) Unseal() ([]byte, error) {
	key, err := os.ReadFile(d.path)
	if err != nil {
		return nil, fmt.Errorf("dev seal: read key: %w", err)
	}
	if len(key) != rootKeySize {
		return nil, errors.New("dev seal: key file has wrong size")
	}
	return key, nil
}
