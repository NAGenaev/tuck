package barrier

import (
	"crypto/cipher"

	"github.com/NAGenaev/tuck/internal/gost/kuznyechik"
	"github.com/NAGenaev/tuck/internal/gost/mgm"
)

func init() {
	RegisterSuite(Suite{
		Name: SuiteKuznyechikMGM,
		New: func(key []byte) (cipher.AEAD, error) {
			block, err := kuznyechik.NewCipher(key)
			if err != nil {
				return nil, err
			}
			return mgm.New(block)
		},
		ClearNonceTopBit: true,
	})
}
