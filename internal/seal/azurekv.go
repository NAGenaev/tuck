package seal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
)

// AzureKVCrypter is the encrypt/decrypt interface used by AzureKVSeal.
// The real implementation wraps *azkeys.Client; implement it to inject stubs
// in tests without real Azure credentials.
type AzureKVCrypter interface {
	Encrypt(ctx context.Context, keyName string, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, keyName string, ciphertext []byte) ([]byte, error)
}

// azureKVRealClient wraps *azkeys.Client behind AzureKVCrypter.
type azureKVRealClient struct {
	inner     *azkeys.Client
	algorithm azkeys.EncryptionAlgorithm
}

func (r *azureKVRealClient) Encrypt(ctx context.Context, keyName string, plaintext []byte) ([]byte, error) {
	resp, err := r.inner.Encrypt(ctx, keyName, "", azkeys.KeyOperationParameters{
		Algorithm: &r.algorithm,
		Value:     plaintext,
	}, nil)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (r *azureKVRealClient) Decrypt(ctx context.Context, keyName string, ciphertext []byte) ([]byte, error) {
	resp, err := r.inner.Decrypt(ctx, keyName, "", azkeys.KeyOperationParameters{
		Algorithm: &r.algorithm,
		Value:     ciphertext,
	}, nil)
	if err != nil {
		return nil, err
	}
	return resp.Result, nil
}

// AzureKVSeal implements auto-unseal via Azure Key Vault.
//
// The root key is generated locally, encrypted with the specified Key Vault
// key, and stored as base64 in a local file. On restart, the ciphertext is
// read and decrypted by Key Vault — the plaintext never touches disk.
//
// The key must be an RSA key (2048, 3072, or 4096 bit). The default
// algorithm is RSA-OAEP-256.
//
// Credentials are resolved through DefaultAzureCredential:
// AZURE_CLIENT_ID / AZURE_CLIENT_SECRET / AZURE_TENANT_ID env vars →
// Managed Identity (AKS Workload Identity / VM MSI) →
// Azure CLI credentials (local development).
type AzureKVSeal struct {
	vaultURL    string               // e.g. "https://my-vault.vault.azure.net"
	keyName     string               // key name inside the vault
	algorithm   azkeys.EncryptionAlgorithm
	wrappedPath string              // local file storing the base64 ciphertext
	crypter     AzureKVCrypter      // nil until first use; replaced in tests
}

// NewAzureKV creates an AzureKVSeal.
//
//   - vaultURL: Azure Key Vault URL (e.g. "https://my-vault.vault.azure.net")
//   - keyName: name of the RSA key inside the vault
//   - algorithm: encryption algorithm; "" defaults to RSA-OAEP-256
//   - wrappedPath: local file to store the encrypted root key ciphertext
//   - opts: optional functional overrides (primarily for tests)
func NewAzureKV(vaultURL, keyName, algorithm, wrappedPath string, opts ...func(*AzureKVSeal)) *AzureKVSeal {
	algo := azkeys.EncryptionAlgorithmRSAOAEP256
	if algorithm != "" {
		algo = azkeys.EncryptionAlgorithm(algorithm)
	}
	a := &AzureKVSeal{
		vaultURL:    vaultURL,
		keyName:     keyName,
		algorithm:   algo,
		wrappedPath: wrappedPath,
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

// WithAzureKVCrypter overrides the Key Vault crypter. Use in tests to avoid real Azure calls.
func WithAzureKVCrypter(c AzureKVCrypter) func(*AzureKVSeal) {
	return func(a *AzureKVSeal) { a.crypter = c }
}

func (a *AzureKVSeal) Type() string { return "azurekv" }

// Init generates a 32-byte root key, encrypts it via Azure Key Vault, stores
// the ciphertext locally, and returns the plaintext in InitResult. Shares is nil.
func (a *AzureKVSeal) Init() (*InitResult, error) {
	ctx := context.Background()
	c, err := a.kvCrypter()
	if err != nil {
		return nil, err
	}

	key := make([]byte, rootKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("azurekv seal: generate root key: %w", err)
	}

	ciphertext, err := c.Encrypt(ctx, a.keyName, key)
	if err != nil {
		return nil, fmt.Errorf("azurekv seal: Key Vault Encrypt: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	if err := os.WriteFile(a.wrappedPath, []byte(encoded), 0600); err != nil {
		return nil, fmt.Errorf("azurekv seal: write wrapped key: %w", err)
	}

	return &InitResult{RootKey: key}, nil
}

// Unseal reads the stored ciphertext, decrypts it via Azure Key Vault, and
// returns the root key.
func (a *AzureKVSeal) Unseal() ([]byte, error) {
	ctx := context.Background()
	c, err := a.kvCrypter()
	if err != nil {
		return nil, err
	}

	encoded, err := os.ReadFile(a.wrappedPath)
	if err != nil {
		return nil, fmt.Errorf("azurekv seal: read wrapped key: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("azurekv seal: decode ciphertext: %w", err)
	}

	key, err := c.Decrypt(ctx, a.keyName, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("azurekv seal: Key Vault Decrypt: %w", err)
	}

	if len(key) != rootKeySize {
		return nil, fmt.Errorf("azurekv seal: decrypted key has unexpected size %d (want %d)", len(key), rootKeySize)
	}

	return key, nil
}

// kvCrypter returns the injected crypter or lazily creates a real Azure KV client.
func (a *AzureKVSeal) kvCrypter() (AzureKVCrypter, error) {
	if a.crypter != nil {
		return a.crypter, nil
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("azurekv seal: build Azure credential: %w", err)
	}
	client, err := azkeys.NewClient(a.vaultURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("azurekv seal: create Key Vault client: %w", err)
	}
	a.crypter = &azureKVRealClient{inner: client, algorithm: a.algorithm}
	return a.crypter, nil
}
