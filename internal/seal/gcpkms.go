package seal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"google.golang.org/api/option"
)

// GCPKMSCrypter is the encrypt/decrypt interface used by GCPKMSSeal.
// The real implementation wraps *kms.KeyManagementClient; implement this
// interface to inject a stub in tests without real GCP credentials.
type GCPKMSCrypter interface {
	Encrypt(ctx context.Context, keyName string, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, keyName string, ciphertext []byte) ([]byte, error)
	Close() error
}

// gcpKMSRealClient wraps the GCP Cloud KMS gRPC client behind GCPKMSCrypter.
type gcpKMSRealClient struct {
	inner *kms.KeyManagementClient
}

func (r *gcpKMSRealClient) Encrypt(ctx context.Context, keyName string, plaintext []byte) ([]byte, error) {
	resp, err := r.inner.Encrypt(ctx, &kmspb.EncryptRequest{
		Name:      keyName,
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, err
	}
	return resp.Ciphertext, nil
}

func (r *gcpKMSRealClient) Decrypt(ctx context.Context, keyName string, ciphertext []byte) ([]byte, error) {
	resp, err := r.inner.Decrypt(ctx, &kmspb.DecryptRequest{
		Name:       keyName,
		Ciphertext: ciphertext,
	})
	if err != nil {
		return nil, err
	}
	return resp.Plaintext, nil
}

func (r *gcpKMSRealClient) Close() error { return r.inner.Close() }

// GCPKMSSeal implements auto-unseal via Google Cloud KMS.
//
// The key name must be the full CryptoKey resource name:
//
//	projects/{project}/locations/{location}/keyRings/{ring}/cryptoKeys/{key}
//
// Credentials are resolved from Application Default Credentials (ADC):
// GOOGLE_APPLICATION_CREDENTIALS env var pointing to a service account JSON
// file, or the GCE/GKE metadata server (Workload Identity — recommended for
// production).
type GCPKMSSeal struct {
	keyName     string       // full CryptoKey resource name
	wrappedPath string       // local file storing the base64-encoded ciphertext
	crypter     GCPKMSCrypter // nil until first use; replaced in tests
	clientOpts  []option.ClientOption // forwarded to the real KMS client
}

// NewGCPKMS creates a GCPKMSSeal.
//
//   - keyName: full CryptoKey resource name
//     (e.g. "projects/my-project/locations/global/keyRings/tuck/cryptoKeys/seal")
//   - wrappedPath: local file path to store the encrypted root key ciphertext
//   - opts: optional functional overrides (primarily for tests)
func NewGCPKMS(keyName, wrappedPath string, opts ...func(*GCPKMSSeal)) *GCPKMSSeal {
	g := &GCPKMSSeal{keyName: keyName, wrappedPath: wrappedPath}
	for _, o := range opts {
		o(g)
	}
	return g
}

// WithGCPKMSCrypter overrides the KMS crypter. Use in tests to avoid real GCP calls.
func WithGCPKMSCrypter(c GCPKMSCrypter) func(*GCPKMSSeal) {
	return func(g *GCPKMSSeal) { g.crypter = c }
}

// WithGCPKMSClientOptions appends gRPC client options forwarded to the real
// Cloud KMS client (e.g. custom credentials, endpoint override for testing).
func WithGCPKMSClientOptions(opts ...option.ClientOption) func(*GCPKMSSeal) {
	return func(g *GCPKMSSeal) { g.clientOpts = append(g.clientOpts, opts...) }
}

func (g *GCPKMSSeal) Type() string { return "gcpkms" }

// Init generates a 32-byte root key, encrypts it via Cloud KMS, stores the
// ciphertext locally, and returns the plaintext in InitResult. Shares is nil.
func (g *GCPKMSSeal) Init() (*InitResult, error) {
	ctx := context.Background()
	c, err := g.kmsCrypter(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if g.crypter == nil {
			c.Close() //nolint:errcheck
		}
	}()

	key := make([]byte, rootKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("gcpkms seal: generate root key: %w", err)
	}

	ciphertext, err := c.Encrypt(ctx, g.keyName, key)
	if err != nil {
		return nil, fmt.Errorf("gcpkms seal: Cloud KMS Encrypt: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	if err := os.WriteFile(g.wrappedPath, []byte(encoded), 0600); err != nil {
		return nil, fmt.Errorf("gcpkms seal: write wrapped key: %w", err)
	}

	return &InitResult{RootKey: key}, nil
}

// Unseal reads the stored ciphertext, decrypts it via Cloud KMS, and returns
// the root key. Credentials are resolved from ADC.
func (g *GCPKMSSeal) Unseal() ([]byte, error) {
	ctx := context.Background()
	c, err := g.kmsCrypter(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if g.crypter == nil {
			c.Close() //nolint:errcheck
		}
	}()

	encoded, err := os.ReadFile(g.wrappedPath)
	if err != nil {
		return nil, fmt.Errorf("gcpkms seal: read wrapped key: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("gcpkms seal: decode ciphertext: %w", err)
	}

	key, err := c.Decrypt(ctx, g.keyName, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("gcpkms seal: Cloud KMS Decrypt: %w", err)
	}

	if len(key) != rootKeySize {
		return nil, fmt.Errorf("gcpkms seal: decrypted key has unexpected size %d (want %d)", len(key), rootKeySize)
	}

	return key, nil
}

// kmsCrypter returns the injected crypter or lazily creates a real GCP KMS client.
func (g *GCPKMSSeal) kmsCrypter(ctx context.Context) (GCPKMSCrypter, error) {
	if g.crypter != nil {
		return g.crypter, nil
	}
	c, err := kms.NewKeyManagementClient(ctx, g.clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("gcpkms seal: create Cloud KMS client: %w", err)
	}
	return &gcpKMSRealClient{inner: c}, nil
}
