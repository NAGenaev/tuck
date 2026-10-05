package tlsutil

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"sync"
	"time"
)

// Reloader serves a certificate loaded from disk and swaps it when the files
// change (cert-manager renewals, Kubernetes Secret rotation, manual replace).
// A failed reload keeps the previous certificate.
type Reloader struct {
	certFile, keyFile string

	mu   sync.RWMutex
	cert *tls.Certificate
	sig  string
}

// NewReloader loads the initial certificate; it fails if the pair is invalid.
func NewReloader(certFile, keyFile string) (*Reloader, error) {
	r := &Reloader{certFile: certFile, keyFile: keyFile}
	if _, err := r.reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// GetCertificate implements tls.Config.GetCertificate.
func (r *Reloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cert, nil
}

func (r *Reloader) signature() (string, error) {
	var sig string
	for _, p := range []string{r.certFile, r.keyFile} {
		fi, err := os.Stat(p) // follows symlinks, so ..data swaps in k8s mounts are seen
		if err != nil {
			return "", err
		}
		sig += fmt.Sprintf("%d:%d;", fi.ModTime().UnixNano(), fi.Size())
	}
	return sig, nil
}

func (r *Reloader) reload() (bool, error) {
	sig, err := r.signature()
	if err != nil {
		return false, err
	}
	r.mu.RLock()
	same := sig == r.sig
	r.mu.RUnlock()
	if same {
		return false, nil
	}
	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return false, err
	}
	r.mu.Lock()
	r.cert, r.sig = &cert, sig
	r.mu.Unlock()
	return true, nil
}

// Watch polls the files every interval until ctx is done, logging through
// logf when a new certificate is loaded or a reload fails.
func (r *Reloader) Watch(ctx context.Context, interval time.Duration, logf func(format string, args ...any)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			switch changed, err := r.reload(); {
			case err != nil:
				logf("tuck: TLS certificate reload failed, keeping the previous one: %v", err)
			case changed:
				logf("tuck: TLS certificate reloaded (cert=%s)", r.certFile)
			}
		}
	}
}
