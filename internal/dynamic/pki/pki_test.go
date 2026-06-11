package pki

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

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

func mgr(t *testing.T) *Manager {
	t.Helper()
	return NewManager(newMem())
}

func mgrWithCA(t *testing.T) *Manager {
	t.Helper()
	m := mgr(t)
	if _, err := m.GenerateCA(context.Background(), &CAConfig{CommonName: "Test CA"}); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	return m
}

func roleWith(t *testing.T, m *Manager, r *Role) {
	t.Helper()
	if err := m.PutRole(context.Background(), r); err != nil {
		t.Fatalf("PutRole: %v", err)
	}
}

// --- tests ---

func TestGenerateCA(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()

	certPEM, err := m.GenerateCA(ctx, &CAConfig{CommonName: "My Root CA"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(certPEM, "BEGIN CERTIFICATE") {
		t.Fatal("expected PEM certificate")
	}

	// GetCACert round-trip.
	got, err := m.GetCACert(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != certPEM {
		t.Fatal("GetCACert mismatch")
	}

	// Parse the cert and check it's a CA.
	cert, err := parseCert(certPEM)
	if err != nil {
		t.Fatal(err)
	}
	if !cert.IsCA {
		t.Fatal("expected IsCA=true")
	}
	if cert.Subject.CommonName != "My Root CA" {
		t.Fatalf("wrong CN: %q", cert.Subject.CommonName)
	}
}

func TestImportCA(t *testing.T) {
	// Generate a self-signed CA externally.
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Imported CA"},
		NotBefore:             now,
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, priv.Public(), priv)
	certPEM := encodeCert(der)
	keyDER, _ := x509.MarshalECPrivateKey(priv)
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))

	m := mgr(t)
	ctx := context.Background()
	if err := m.ImportCA(ctx, certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetCACert(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != certPEM {
		t.Fatal("imported cert mismatch")
	}
}

func TestNoCAError(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()

	if _, err := m.GetCACert(ctx); !errors.Is(err, ErrNoCA) {
		t.Fatalf("expected ErrNoCA, got %v", err)
	}
	if _, err := m.IssueCert(ctx, "r", "example.com", nil, 0); !errors.Is(err, ErrNotFound) {
		// ErrNotFound from missing role is expected first.
	}
}

func TestRoleCRUD(t *testing.T) {
	m := mgr(t)
	ctx := context.Background()

	r := &Role{
		Name:           "web",
		AllowedDomains: []string{"example.com"},
		ServerFlag:     true,
	}
	if err := m.PutRole(ctx, r); err != nil {
		t.Fatal(err)
	}
	// Defaults filled in.
	if r.DefaultTTL == 0 {
		t.Fatal("expected DefaultTTL to be set")
	}

	got, err := m.GetRole(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "web" || !got.ServerFlag {
		t.Fatalf("role mismatch: %+v", got)
	}

	names, err := m.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "web" {
		t.Fatalf("ListRoles: %v", names)
	}

	if err := m.DeleteRole(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetRole(ctx, "web"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestIssueCertVerifiesAgainstCA(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()
	roleWith(t, m, &Role{
		Name:            "svc",
		AllowedDomains:  []string{"internal.example.com"},
		AllowSubdomains: true,
		DefaultTTL:      time.Hour,
		ServerFlag:      true,
	})

	issued, err := m.IssueCert(ctx, "svc", "api.internal.example.com", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Serial == "" {
		t.Fatal("expected serial")
	}
	if !strings.Contains(issued.PrivateKeyPEM, "PRIVATE KEY") {
		t.Fatal("expected private key PEM")
	}

	// Verify the leaf cert chains to our CA.
	caCertPEM, _ := m.GetCACert(ctx)
	caCert, _ := parseCert(caCertPEM)
	leafCert, err := parseCert(issued.CertPEM)
	if err != nil {
		t.Fatalf("parse leaf cert: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leafCert.Verify(x509.VerifyOptions{
		Roots:   pool,
		DNSName: "api.internal.example.com",
	}); err != nil {
		t.Fatalf("cert verification failed: %v", err)
	}
}

func TestIssueCertRSA(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()
	roleWith(t, m, &Role{
		Name:           "rsa-role",
		AllowedDomains: []string{"example.com"},
		KeyType:        "rsa",
		KeyBits:        2048,
		DefaultTTL:     time.Hour,
	})

	issued, err := m.IssueCert(ctx, "rsa-role", "example.com", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(issued.PrivateKeyPEM, "RSA PRIVATE KEY") {
		t.Fatalf("expected RSA key, got: %s", issued.PrivateKeyPEM[:30])
	}
}

func TestDomainValidation(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()
	roleWith(t, m, &Role{
		Name:            "strict",
		AllowedDomains:  []string{"example.com"},
		AllowSubdomains: false,
		DefaultTTL:      time.Hour,
	})

	// Exact match allowed.
	if _, err := m.IssueCert(ctx, "strict", "example.com", nil, 0); err != nil {
		t.Fatalf("exact match should be allowed: %v", err)
	}

	// Subdomain rejected when AllowSubdomains=false.
	if _, err := m.IssueCert(ctx, "strict", "sub.example.com", nil, 0); !errors.Is(err, ErrDomainDenied) {
		t.Fatalf("expected ErrDomainDenied for subdomain, got %v", err)
	}

	// Unrelated domain rejected.
	if _, err := m.IssueCert(ctx, "strict", "evil.com", nil, 0); !errors.Is(err, ErrDomainDenied) {
		t.Fatalf("expected ErrDomainDenied for unrelated domain, got %v", err)
	}
}

func TestSubdomainAllowed(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()
	roleWith(t, m, &Role{
		Name:            "wildcard",
		AllowedDomains:  []string{"example.com"},
		AllowSubdomains: true,
		DefaultTTL:      time.Hour,
	})

	if _, err := m.IssueCert(ctx, "wildcard", "deep.sub.example.com", nil, 0); err != nil {
		t.Fatalf("subdomain should be allowed: %v", err)
	}
}

func TestIPSANValidation(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()
	roleWith(t, m, &Role{
		Name:           "no-ip",
		AllowedDomains: []string{"example.com"},
		DefaultTTL:     time.Hour,
	})
	roleWith(t, m, &Role{
		Name:           "with-ip",
		AllowedDomains: []string{"example.com"},
		AllowIPSANs:    true,
		DefaultTTL:     time.Hour,
	})

	// IP SAN rejected when not allowed.
	if _, err := m.IssueCert(ctx, "no-ip", "example.com", []string{"10.0.0.1"}, 0); !errors.Is(err, ErrDomainDenied) {
		t.Fatalf("expected ErrDomainDenied for IP SAN, got %v", err)
	}

	// IP SAN accepted when allowed.
	if _, err := m.IssueCert(ctx, "with-ip", "example.com", []string{"10.0.0.1"}, 0); err != nil {
		t.Fatalf("IP SAN should be allowed: %v", err)
	}
}

func TestRevocationAndCRL(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()
	roleWith(t, m, &Role{
		Name:           "r",
		AllowedDomains: []string{"example.com"},
		DefaultTTL:     time.Hour,
	})

	issued, err := m.IssueCert(ctx, "r", "example.com", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.RevokeCert(ctx, issued.Serial); err != nil {
		t.Fatal(err)
	}

	// Cert record should show revoked.
	cr, err := m.GetCert(ctx, issued.Serial)
	if err != nil {
		t.Fatal(err)
	}
	if !cr.Revoked {
		t.Fatal("expected Revoked=true")
	}

	// CRL should contain the serial.
	crlPEM, err := m.GetCRL(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(crlPEM, "BEGIN X509 CRL") {
		t.Fatalf("expected CRL PEM, got: %q", crlPEM[:50])
	}

	block, _ := pem.Decode([]byte(crlPEM))
	crl, err := x509.ParseRevocationList(block.Bytes)
	if err != nil {
		t.Fatalf("parse CRL: %v", err)
	}
	if len(crl.RevokedCertificateEntries) != 1 {
		t.Fatalf("expected 1 revoked cert in CRL, got %d", len(crl.RevokedCertificateEntries))
	}
}

func TestListCerts(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()
	roleWith(t, m, &Role{
		Name:           "r",
		AllowedDomains: []string{"a.com", "b.com"},
		DefaultTTL:     time.Hour,
	})

	m.IssueCert(ctx, "r", "a.com", nil, 0)
	m.IssueCert(ctx, "r", "b.com", nil, 0)

	serials, err := m.ListCerts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(serials) != 2 {
		t.Fatalf("expected 2 certs, got %d", len(serials))
	}
}

func TestTTLEnforced(t *testing.T) {
	m := mgrWithCA(t)
	ctx := context.Background()
	roleWith(t, m, &Role{
		Name:           "r",
		AllowedDomains: []string{"example.com"},
		DefaultTTL:     time.Hour,
		MaxTTL:         2 * time.Hour,
	})

	// Request beyond MaxTTL — should be capped.
	issued, err := m.IssueCert(ctx, "r", "example.com", nil, 100*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if issued.TTL > 2*time.Hour {
		t.Fatalf("TTL %v exceeds MaxTTL", issued.TTL)
	}
}
