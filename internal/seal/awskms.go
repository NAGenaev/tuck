package seal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
)

// AWSKMSCrypter is the encrypt/decrypt interface used by AWSKMSSeal.
// The real implementation wraps *kms.Client from aws-sdk-go-v2; implement
// this interface to inject a stub in tests without real AWS credentials.
type AWSKMSCrypter interface {
	Encrypt(ctx context.Context, keyID string, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, keyID string, ciphertext []byte) ([]byte, error)
}

// awsKMSRealClient wraps the AWS SDK KMS client behind AWSKMSCrypter.
type awsKMSRealClient struct {
	inner *awskms.Client
}

func (r *awsKMSRealClient) Encrypt(ctx context.Context, keyID string, plaintext []byte) ([]byte, error) {
	out, err := r.inner.Encrypt(ctx, &awskms.EncryptInput{
		KeyId:     &keyID,
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, err
	}
	return out.CiphertextBlob, nil
}

func (r *awsKMSRealClient) Decrypt(ctx context.Context, keyID string, ciphertext []byte) ([]byte, error) {
	out, err := r.inner.Decrypt(ctx, &awskms.DecryptInput{
		CiphertextBlob: ciphertext,
		KeyId:          &keyID,
	})
	if err != nil {
		return nil, err
	}
	return out.Plaintext, nil
}

// AWSKMSSeal implements auto-unseal via AWS Key Management Service (KMS).
//
// On Init a 32-byte root key is generated in memory, encrypted with the
// specified CMK, and stored as base64 in a local file. On Unseal the
// ciphertext is read from that file and decrypted by KMS — the plaintext
// never touches disk.
//
// Credentials are resolved from the standard AWS credential chain:
// IAM instance/pod role (EC2 / EKS IRSA / ECS) → environment variables
// (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY) → shared credentials file
// (~/.aws/credentials).
type AWSKMSSeal struct {
	keyID       string        // CMK ARN or alias, e.g. "alias/tuck-seal"
	region      string        // AWS region; "" = from environment / SDK defaults
	wrappedPath string        // local file storing the base64-encoded ciphertext
	crypter     AWSKMSCrypter // nil until first use; replaced in tests
}

// NewAWSKMS creates an AWSKMSSeal.
//
//   - keyID: CMK ARN or alias (e.g. "alias/tuck-seal" or
//     "arn:aws:kms:us-east-1:123456789012:key/abc-def")
//   - region: AWS region (e.g. "us-east-1"); "" = from AWS_DEFAULT_REGION or profile
//   - wrappedPath: local file path to store the encrypted root key ciphertext
//   - opts: optional functional overrides (primarily for tests)
func NewAWSKMS(keyID, region, wrappedPath string, opts ...func(*AWSKMSSeal)) *AWSKMSSeal {
	a := &AWSKMSSeal{keyID: keyID, region: region, wrappedPath: wrappedPath}
	for _, o := range opts {
		o(a)
	}
	return a
}

// WithAWSKMSCrypter overrides the KMS crypter. Use in tests to avoid real AWS calls.
func WithAWSKMSCrypter(c AWSKMSCrypter) func(*AWSKMSSeal) {
	return func(a *AWSKMSSeal) { a.crypter = c }
}

func (a *AWSKMSSeal) Type() string { return "awskms" }

// Init generates a 32-byte root key, encrypts it via the CMK, stores the
// ciphertext locally, and returns the plaintext in InitResult. Shares is nil.
func (a *AWSKMSSeal) Init() (*InitResult, error) {
	ctx := context.Background()
	c, err := a.kmsCrypter(ctx)
	if err != nil {
		return nil, err
	}

	key := make([]byte, rootKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("awskms seal: generate root key: %w", err)
	}

	ciphertext, err := c.Encrypt(ctx, a.keyID, key)
	if err != nil {
		return nil, fmt.Errorf("awskms seal: KMS Encrypt: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	if err := os.WriteFile(a.wrappedPath, []byte(encoded), 0600); err != nil {
		return nil, fmt.Errorf("awskms seal: write wrapped key: %w", err)
	}

	return &InitResult{RootKey: key}, nil
}

// Unseal reads the stored ciphertext, decrypts it via the CMK, and returns
// the root key. Credentials are resolved from the ambient AWS credential chain.
func (a *AWSKMSSeal) Unseal() ([]byte, error) {
	ctx := context.Background()
	c, err := a.kmsCrypter(ctx)
	if err != nil {
		return nil, err
	}

	encoded, err := os.ReadFile(a.wrappedPath)
	if err != nil {
		return nil, fmt.Errorf("awskms seal: read wrapped key: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("awskms seal: decode ciphertext: %w", err)
	}

	key, err := c.Decrypt(ctx, a.keyID, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("awskms seal: KMS Decrypt: %w", err)
	}

	if len(key) != rootKeySize {
		return nil, fmt.Errorf("awskms seal: decrypted key has unexpected size %d (want %d)", len(key), rootKeySize)
	}

	return key, nil
}

// kmsCrypter returns the injected crypter or lazily creates a real AWS KMS client.
func (a *AWSKMSSeal) kmsCrypter(ctx context.Context) (AWSKMSCrypter, error) {
	if a.crypter != nil {
		return a.crypter, nil
	}
	var lopts []func(*config.LoadOptions) error
	if a.region != "" {
		lopts = append(lopts, config.WithRegion(a.region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, lopts...)
	if err != nil {
		return nil, fmt.Errorf("awskms seal: load AWS config: %w", err)
	}
	a.crypter = &awsKMSRealClient{inner: awskms.NewFromConfig(cfg)}
	return a.crypter, nil
}
