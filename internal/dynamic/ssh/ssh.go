// Package ssh implements Tuck's SSH Secrets Engine in CA (certificate authority)
// mode. Tuck generates or imports an SSH CA key pair, and signs user or host
// SSH public keys to produce short-lived SSH certificates.
//
// Target host setup (one-time):
//
//	# 1. Fetch Tuck CA public key
//	curl https://tuck:8200/v1/ssh/ca/public-key > /etc/ssh/trusted_user_ca_keys
//	# 2. Add to /etc/ssh/sshd_config:
//	#    TrustedUserCAKeys /etc/ssh/trusted_user_ca_keys
//	# 3. Reload sshd
//
// Client workflow:
//
//	# Sign your SSH public key against a role
//	curl -XPOST https://tuck:8200/v1/ssh/sign/my-role \
//	  -H "X-Tuck-Token: $TOKEN" \
//	  -d '{"public_key":"ssh-ed25519 AAAA...","valid_principals":["ubuntu"]}'
//	# Copy the signed_key into ~/.ssh/id_ed25519-cert.pub
//	# SSH: ssh -i ~/.ssh/id_ed25519 ubuntu@host
package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	ErrNotFound     = errors.New("ssh: not found")
	ErrNoCA         = errors.New("ssh: no CA configured — call generate/ca or import/ca first")
	ErrInvalidPEM   = errors.New("ssh: invalid PEM data")
	ErrPrincipalDenied = errors.New("ssh: requested principal not permitted by role")
)

const (
	caStorageKey = "dynamic/ssh/ca"
	rolesPrefix  = "dynamic/ssh/roles/"
)

// defaultExtensions are the standard extensions granted to user certificates.
var defaultExtensions = map[string]string{
	"permit-pty":              "",
	"permit-port-forwarding":  "",
	"permit-agent-forwarding": "",
	"permit-X11-forwarding":   "",
	"permit-user-rc":          "",
}

// caRecord is stored in the barrier (private key is protected by barrier encryption).
type caRecord struct {
	// PublicKey is the CA public key in OpenSSH authorized_keys format.
	PublicKey  string `json:"public_key"`
	// PrivateKey is the PKCS8 PEM-encoded CA private key.
	PrivateKey string `json:"private_key"`
}

// Role defines what certificates a role is allowed to issue.
type Role struct {
	Name string `json:"name"`
	// AllowedUsers lists the SSH usernames that may be requested as principals.
	// Empty list means any username is allowed.
	AllowedUsers []string `json:"allowed_users"`
	// DefaultExtensions are added to every certificate. Defaults to the five
	// standard permit-* extensions when nil.
	DefaultExtensions map[string]string `json:"default_extensions,omitempty"`
	// CertType: "user" (default) or "host".
	CertType   string        `json:"cert_type"`
	DefaultTTL time.Duration `json:"default_ttl"`
	MaxTTL     time.Duration `json:"max_ttl"`
}

// SignedCert is returned to the caller after a successful signing operation.
type SignedCert struct {
	Serial       uint64    `json:"serial"`
	// SignedKey is the signed SSH certificate in authorized_keys line format.
	// Write this to ~/.ssh/id_*-cert.pub.
	SignedKey    string    `json:"signed_key"`
	ValidAfter   time.Time `json:"valid_after"`
	ValidBefore  time.Time `json:"valid_before"`
	TTL          time.Duration `json:"ttl"`
}

type barrier interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Manager manages an SSH CA and role-based certificate signing.
type Manager struct{ b barrier }

// NewManager creates a Manager backed by b.
func NewManager(b barrier) *Manager { return &Manager{b: b} }

// GenerateCA creates a new SSH CA key pair and persists it.
// keyType: "ed25519" (default) or "rsa" (4096-bit).
// Returns the CA public key in OpenSSH authorized_keys format.
func (m *Manager) GenerateCA(ctx context.Context, keyType string) (string, error) {
	if keyType == "" {
		keyType = "ed25519"
	}
	privKey, pubKeyStr, err := generateSSHCAKey(keyType)
	if err != nil {
		return "", err
	}
	privPEM, err := marshalPrivKeyPEM(privKey)
	if err != nil {
		return "", err
	}
	rec := &caRecord{PublicKey: pubKeyStr, PrivateKey: privPEM}
	if err := m.put(ctx, caStorageKey, rec); err != nil {
		return "", err
	}
	return pubKeyStr, nil
}

// ImportCA imports an existing SSH CA from a PEM-encoded private key.
// The public key is derived automatically.
func (m *Manager) ImportCA(ctx context.Context, privateKeyPEM string) error {
	privKey, err := parsePrivKeyPEM(privateKeyPEM)
	if err != nil {
		return fmt.Errorf("ssh: import CA: %w", err)
	}
	sshPub, err := gossh.NewPublicKey(cryptoPublicKey(privKey))
	if err != nil {
		return fmt.Errorf("ssh: derive public key: %w", err)
	}
	pubKeyStr := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(sshPub)))
	rec := &caRecord{PublicKey: pubKeyStr, PrivateKey: privateKeyPEM}
	return m.put(ctx, caStorageKey, rec)
}

// GetCAPublicKey returns the CA public key in OpenSSH format.
// This is the value to put in TrustedUserCAKeys on target hosts.
// Intentionally unauthenticated so hosts can fetch it without a token.
func (m *Manager) GetCAPublicKey(ctx context.Context) (string, error) {
	var rec caRecord
	if err := m.get(ctx, caStorageKey, &rec); err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", ErrNoCA
		}
		return "", err
	}
	return rec.PublicKey, nil
}

// --- Role CRUD ---

func (m *Manager) PutRole(ctx context.Context, r *Role) error {
	if r.CertType == "" {
		r.CertType = "user"
	}
	if r.DefaultTTL <= 0 {
		r.DefaultTTL = 24 * time.Hour
	}
	if r.MaxTTL <= 0 {
		r.MaxTTL = 7 * 24 * time.Hour
	}
	return m.put(ctx, rolesPrefix+r.Name, r)
}

func (m *Manager) GetRole(ctx context.Context, name string) (*Role, error) {
	var r Role
	if err := m.get(ctx, rolesPrefix+name, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (m *Manager) DeleteRole(ctx context.Context, name string) error {
	return m.b.Delete(ctx, rolesPrefix+name)
}

func (m *Manager) ListRoles(ctx context.Context) ([]string, error) {
	keys, err := m.b.List(ctx, rolesPrefix)
	if err != nil {
		return nil, err
	}
	for i, k := range keys {
		keys[i] = strings.TrimPrefix(k, rolesPrefix)
	}
	return keys, nil
}

// --- Certificate signing ---

// SignPublicKey signs the given SSH public key (in authorized_keys format) with
// the CA, constrained by the named role.
// validPrincipals is the list of SSH usernames the cert should be valid for;
// defaults to role.AllowedUsers when nil.
// requestedTTL=0 uses role.DefaultTTL.
func (m *Manager) SignPublicKey(ctx context.Context, roleName, publicKeyStr string, validPrincipals []string, requestedTTL time.Duration) (*SignedCert, error) {
	role, err := m.GetRole(ctx, roleName)
	if err != nil {
		return nil, err
	}

	// Validate requested principals against role.
	if len(validPrincipals) == 0 {
		validPrincipals = role.AllowedUsers
	}
	if len(role.AllowedUsers) > 0 {
		for _, p := range validPrincipals {
			if !contains(role.AllowedUsers, p) {
				return nil, fmt.Errorf("%w: %q not in allowed_users", ErrPrincipalDenied, p)
			}
		}
	}

	ttl := requestedTTL
	if ttl <= 0 {
		ttl = role.DefaultTTL
	}
	if role.MaxTTL > 0 && ttl > role.MaxTTL {
		ttl = role.MaxTTL
	}

	// Load CA.
	var rec caRecord
	if err := m.get(ctx, caStorageKey, &rec); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNoCA
		}
		return nil, err
	}
	caPriv, err := parsePrivKeyPEM(rec.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("ssh: load CA key: %w", err)
	}
	signer, err := gossh.NewSignerFromKey(caPriv)
	if err != nil {
		return nil, fmt.Errorf("ssh: create signer: %w", err)
	}

	// Parse user's public key.
	userPub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(publicKeyStr))
	if err != nil {
		return nil, fmt.Errorf("ssh: parse public key: %w", err)
	}

	// Generate a random 64-bit serial.
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	validBefore := now.Add(ttl)

	extensions := role.DefaultExtensions
	if extensions == nil {
		extensions = defaultExtensions
	}

	certType := uint32(gossh.UserCert)
	if role.CertType == "host" {
		certType = gossh.HostCert
		extensions = nil // host certs don't use user extensions
	}

	cert := &gossh.Certificate{
		Key:             userPub,
		Serial:          serial,
		CertType:        certType,
		KeyId:           fmt.Sprintf("tuck-%s-%d", roleName, serial),
		ValidPrincipals: validPrincipals,
		ValidAfter:      uint64(now.Unix()),
		ValidBefore:     uint64(validBefore.Unix()),
		Permissions: gossh.Permissions{
			Extensions: extensions,
		},
	}

	if err := cert.SignCert(rand.Reader, signer); err != nil {
		return nil, fmt.Errorf("ssh: sign certificate: %w", err)
	}

	signedKeyStr := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(cert)))

	return &SignedCert{
		Serial:      serial,
		SignedKey:   signedKeyStr,
		ValidAfter:  now,
		ValidBefore: validBefore,
		TTL:         ttl,
	}, nil
}

// --- crypto helpers ---

func generateSSHCAKey(keyType string) (privKey interface{}, pubKeyStr string, err error) {
	var cryptoPriv interface{}
	switch strings.ToLower(keyType) {
	case "rsa":
		priv, e := rsa.GenerateKey(rand.Reader, 4096)
		if e != nil {
			return nil, "", fmt.Errorf("ssh: generate RSA CA key: %w", e)
		}
		cryptoPriv = priv
	default: // ed25519
		_, priv, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return nil, "", fmt.Errorf("ssh: generate Ed25519 CA key: %w", e)
		}
		cryptoPriv = priv
	}

	sshPub, err := gossh.NewPublicKey(cryptoPublicKey(cryptoPriv))
	if err != nil {
		return nil, "", fmt.Errorf("ssh: marshal CA public key: %w", err)
	}
	pubStr := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(sshPub)))
	return cryptoPriv, pubStr, nil
}

func cryptoPublicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case ed25519.PrivateKey:
		return k.Public()
	case *rsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func marshalPrivKeyPEM(priv interface{}) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", fmt.Errorf("ssh: marshal private key: %w", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block)), nil
}

func parsePrivKeyPEM(pemStr string) (interface{}, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, ErrInvalidPEM
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS1 RSA as fallback.
		rsaKey, err2 := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("ssh: parse private key: %w", err)
		}
		return rsaKey, nil
	}
	return key, nil
}

func randomSerial() (uint64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("ssh: generate serial: %w", err)
	}
	return binary.BigEndian.Uint64(b[:]), nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// --- barrier helpers ---

func (m *Manager) get(ctx context.Context, key string, dst interface{}) error {
	e, err := m.b.Get(ctx, key)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrNotFound
	}
	return json.Unmarshal(e.Value, dst)
}

func (m *Manager) put(ctx context.Context, key string, src interface{}) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return m.b.Put(ctx, &physical.Entry{Key: key, Value: data})
}
