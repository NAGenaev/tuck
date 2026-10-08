package tlsutil

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

// LoadCAPool reads a PEM CA bundle for clients that verify the Tuck server
// certificate. The system roots are not included: a pinned bundle is the point.
func LoadCAPool(caFile string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(caFile) // #nosec G304 — operator-configured CA file path
	if err != nil {
		return nil, fmt.Errorf("read CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("no valid certificates found in CA file")
	}
	return pool, nil
}
