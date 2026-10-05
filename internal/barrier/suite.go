package barrier

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"sort"
	"sync"
)

// Cipher suite names stored in the barrier metadata.
const (
	SuiteAES256GCM        = "aes-256-gcm"
	SuiteKuznyechikMGM    = "gost-kuznyechik-mgm"
	defaultSuiteName      = SuiteAES256GCM
	suiteKeySize          = 32
	metaPath              = "barrier/meta"
	metaVersionAADBinding = 1
)

// Suite describes an AEAD used to protect barrier data. Every suite must use
// 256-bit keys. A build that links a certified crypto provider (for example a
// FSB-certified CSP behind PKCS#11) registers its own Suite with RegisterSuite
// and selects it by name when the store is initialised.
type Suite struct {
	Name string
	// New returns an AEAD keyed with a 32-byte key.
	New func(key []byte) (cipher.AEAD, error)
	// ClearNonceTopBit zeroes the most significant bit of every random nonce
	// (required by MGM, whose nonce is 127 bits wide).
	ClearNonceTopBit bool
}

var (
	suitesMu sync.RWMutex
	suites   = map[string]Suite{
		SuiteAES256GCM: {
			Name: SuiteAES256GCM,
			New: func(key []byte) (cipher.AEAD, error) {
				block, err := aes.NewCipher(key)
				if err != nil {
					return nil, err
				}
				return cipher.NewGCM(block)
			},
		},
	}
)

// RegisterSuite makes a cipher suite available by name. It panics on a
// duplicate name so a misconfigured build fails at start-up.
func RegisterSuite(s Suite) {
	suitesMu.Lock()
	defer suitesMu.Unlock()
	if s.Name == "" || s.New == nil {
		panic("barrier: invalid cipher suite")
	}
	if _, dup := suites[s.Name]; dup {
		panic("barrier: duplicate cipher suite " + s.Name)
	}
	suites[s.Name] = s
}

// LookupSuite returns the suite registered under name ("" = default AES-256-GCM).
func LookupSuite(name string) (Suite, error) {
	if name == "" {
		name = defaultSuiteName
	}
	suitesMu.RLock()
	defer suitesMu.RUnlock()
	s, ok := suites[name]
	if !ok {
		return Suite{}, fmt.Errorf("barrier: unknown cipher suite %q (available: %v)", name, suiteNamesLocked())
	}
	return s, nil
}

// SuiteNames lists the registered cipher suites.
func SuiteNames() []string {
	suitesMu.RLock()
	defer suitesMu.RUnlock()
	return suiteNamesLocked()
}

func suiteNamesLocked() []string {
	out := make([]string, 0, len(suites))
	for n := range suites {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
