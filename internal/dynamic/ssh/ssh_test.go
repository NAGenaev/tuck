package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/NAGenaev/tuck/internal/physical"
)

// --- in-memory barrier ---

type memBarrier struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMem() *memBarrier { return &memBarrier{data: make(map[string][]byte)} }

func (m *memBarrier) Get(_ context.Context, key string) (*physical.Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	c := make([]byte, len(v))
	copy(c, v)
	return &physical.Entry{Key: key, Value: c}, nil
}
func (m *memBarrier) Put(_ context.Context, e *physical.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := make([]byte, len(e.Value))
	copy(c, e.Value)
	m.data[e.Key] = c
	return nil
}
func (m *memBarrier) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}
func (m *memBarrier) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

// --- helpers ---

func mgrWithCA(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(newMem())
	if _, err := m.GenerateCA(context.Background(), "ed25519"); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	return m
}

// genUserPubKey generates a fresh Ed25519 key pair and returns the public key
// in OpenSSH authorized_keys format.
func genUserPubKey(t *testing.T) (pubKeyStr string, priv ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(gossh.MarshalAuthorizedKey(sshPub))), priv
}

// --- tests ---

func TestGenerateCA_Ed25519(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()

	pubKey, err := m.GenerateCA(ctx, "ed25519")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pubKey, "ssh-ed25519 ") {
		t.Fatalf("expected ed25519 public key, got: %q", pubKey)
	}

	// Round-trip via GetCAPublicKey.
	got, err := m.GetCAPublicKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != pubKey {
		t.Fatal("GetCAPublicKey mismatch")
	}
}

func TestGenerateCA_RSA(t *testing.T) {
	m := NewManager(newMem())
	pubKey, err := m.GenerateCA(context.Background(), "rsa")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pubKey, "ssh-rsa ") {
		t.Fatalf("expected RSA public key, got: %q", pubKey)
	}
}

func TestImportCA(t *testing.T) {
	// Generate an ed25519 key externally and import it.
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	privPEM, err := marshalPrivKeyPEM(priv)
	if err != nil {
		t.Fatal(err)
	}

	m := NewManager(newMem())
	ctx := context.Background()
	if err := m.ImportCA(ctx, privPEM); err != nil {
		t.Fatalf("ImportCA: %v", err)
	}
	pubKey, err := m.GetCAPublicKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pubKey, "ssh-ed25519 ") {
		t.Fatalf("unexpected CA public key: %q", pubKey)
	}
}

func TestNoCAError(t *testing.T) {
	m := NewManager(newMem())
	if _, err := m.GetCAPublicKey(context.Background()); !errors.Is(err, ErrNoCA) {
		t.Fatalf("expected ErrNoCA, got %v", err)
	}
}

func TestRoleCRUD(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()

	r := &Role{
		Name:         "ops",
		AllowedUsers: []string{"ubuntu", "ec2-user"},
		DefaultTTL:   24 * time.Hour,
	}
	if err := m.PutRole(ctx, r); err != nil {
		t.Fatal(err)
	}

	got, err := m.GetRole(ctx, "ops")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "ops" || len(got.AllowedUsers) != 2 {
		t.Fatalf("role mismatch: %+v", got)
	}

	names, err := m.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "ops" {
		t.Fatalf("ListRoles: %v", names)
	}

	if err := m.DeleteRole(ctx, "ops"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetRole(ctx, "ops"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSignCertificateUserCert(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()

	m.PutRole(ctx, &Role{
		Name:         "dev",
		AllowedUsers: []string{"ubuntu"},
		DefaultTTL:   time.Hour,
	})

	pubKeyStr, _ := genUserPubKey(t)
	signed, err := m.SignPublicKey(ctx, "dev", pubKeyStr, []string{"ubuntu"}, 0)
	if err != nil {
		t.Fatal(err)
	}

	if signed.Serial == 0 {
		t.Fatal("expected non-zero serial")
	}
	if !strings.Contains(signed.SignedKey, "-cert-v01@openssh.com") {
		t.Fatalf("unexpected signed key format: %q", signed.SignedKey)
	}

	// Parse the certificate and validate structure.
	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(signed.SignedKey))
	if err != nil {
		t.Fatalf("parse signed cert: %v", err)
	}
	cert, ok := pub.(*gossh.Certificate)
	if !ok {
		t.Fatalf("expected *gossh.Certificate, got %T", pub)
	}
	if cert.CertType != gossh.UserCert {
		t.Fatalf("expected UserCert, got %d", cert.CertType)
	}
	if len(cert.ValidPrincipals) != 1 || cert.ValidPrincipals[0] != "ubuntu" {
		t.Fatalf("wrong principals: %v", cert.ValidPrincipals)
	}
	// Verify cert is signed by our CA.
	caPubStr, _ := m.GetCAPublicKey(ctx)
	caPub, _, _, _, _ := gossh.ParseAuthorizedKey([]byte(caPubStr))
	checker := gossh.CertChecker{
		IsUserAuthority: func(k gossh.PublicKey) bool {
			return gossh.FingerprintSHA256(k) == gossh.FingerprintSHA256(caPub)
		},
	}
	if err := checker.CheckCert("ubuntu", cert); err != nil {
		t.Fatalf("certificate verification failed: %v", err)
	}
}

func TestSignCertificateTTLCap(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()

	m.PutRole(ctx, &Role{
		Name:       "r",
		DefaultTTL: time.Hour,
		MaxTTL:     2 * time.Hour,
	})

	pubKeyStr, _ := genUserPubKey(t)
	signed, err := m.SignPublicKey(ctx, "r", pubKeyStr, nil, 100*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if signed.TTL > 2*time.Hour {
		t.Fatalf("TTL %v should be capped at MaxTTL (2h)", signed.TTL)
	}
}

func TestSignCertificatePrincipalDenied(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()

	m.PutRole(ctx, &Role{
		Name:         "strict",
		AllowedUsers: []string{"ubuntu"},
		DefaultTTL:   time.Hour,
	})

	pubKeyStr, _ := genUserPubKey(t)
	_, err := m.SignPublicKey(ctx, "strict", pubKeyStr, []string{"root"}, 0)
	if !errors.Is(err, ErrPrincipalDenied) {
		t.Fatalf("expected ErrPrincipalDenied, got %v", err)
	}
}

func TestSignCertificateAnyPrincipalAllowed(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()

	// Role with empty AllowedUsers = any principal is OK.
	m.PutRole(ctx, &Role{
		Name:       "open",
		DefaultTTL: time.Hour,
	})

	pubKeyStr, _ := genUserPubKey(t)
	signed, err := m.SignPublicKey(ctx, "open", pubKeyStr, []string{"any-user", "another"}, 0)
	if err != nil {
		t.Fatalf("any principal should be allowed when AllowedUsers is empty: %v", err)
	}
	pub, _, _, _, _ := gossh.ParseAuthorizedKey([]byte(signed.SignedKey))
	cert := pub.(*gossh.Certificate)
	if len(cert.ValidPrincipals) != 2 {
		t.Fatalf("expected 2 principals, got %v", cert.ValidPrincipals)
	}
}

func TestSignHostCertificate(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()

	m.PutRole(ctx, &Role{
		Name:       "hosts",
		CertType:   "host",
		DefaultTTL: 7 * 24 * time.Hour,
	})

	pubKeyStr, _ := genUserPubKey(t)
	signed, err := m.SignPublicKey(ctx, "hosts", pubKeyStr, []string{"myhost.internal"}, 0)
	if err != nil {
		t.Fatal(err)
	}

	pub, _, _, _, _ := gossh.ParseAuthorizedKey([]byte(signed.SignedKey))
	cert := pub.(*gossh.Certificate)
	if cert.CertType != gossh.HostCert {
		t.Fatalf("expected HostCert, got %d", cert.CertType)
	}
}

func TestSignWithRSACA(t *testing.T) {
	m := NewManager(newMem())
	ctx := context.Background()

	// Generate RSA CA (slower).
	if _, err := m.GenerateCA(ctx, "rsa"); err != nil {
		t.Fatal(err)
	}
	m.PutRole(ctx, &Role{
		Name:       "r",
		DefaultTTL: time.Hour,
	})
	pubKeyStr, _ := genUserPubKey(t)
	signed, err := m.SignPublicKey(ctx, "r", pubKeyStr, []string{"user"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(signed.SignedKey, "-cert-v01@openssh.com") {
		t.Fatalf("unexpected cert format: %q", signed.SignedKey)
	}
}

func TestImportRSACAAndSign(t *testing.T) {
	// Generate RSA key externally and import.
	rsaKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	privPEM, _ := marshalPrivKeyPEM(rsaKey)

	m := NewManager(newMem())
	ctx := context.Background()
	if err := m.ImportCA(ctx, privPEM); err != nil {
		t.Fatalf("ImportCA RSA: %v", err)
	}
	m.PutRole(ctx, &Role{Name: "r", DefaultTTL: time.Hour})
	pubKeyStr, _ := genUserPubKey(t)
	if _, err := m.SignPublicKey(ctx, "r", pubKeyStr, []string{"user"}, 0); err != nil {
		t.Fatalf("sign with imported RSA CA: %v", err)
	}
}
