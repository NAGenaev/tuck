// Package pki implements a Tuck PKI secrets engine. It acts as an internal
// Certificate Authority: generates or imports a root CA, defines roles that
// constrain what certificates may be issued, and signs leaf certificates on
// demand. Issued certs are tracked so they can appear in the generated CRL.
//
// No external dependencies — uses only stdlib crypto packages.
package pki

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/physical"
)

var (
	// ErrNotFound is returned when a role or cert record does not exist.
	ErrNotFound = errors.New("pki: not found")
	// ErrNoCA is returned when no CA has been configured yet.
	ErrNoCA = errors.New("pki: no CA configured — call generate/root or import/ca first")
	// ErrDomainDenied is returned when a CN or SAN violates the role's allowed_domains.
	ErrDomainDenied = errors.New("pki: common name or SAN not permitted by role")
	// ErrInvalidPEM is returned when PEM data cannot be decoded.
	ErrInvalidPEM = errors.New("pki: invalid PEM data")
)

const (
	caStorageKey = "dynamic/pki/ca"
	rolesPrefix  = "dynamic/pki/roles/"
	certsPrefix  = "dynamic/pki/certs/"
)

// CAConfig controls root CA generation.
type CAConfig struct {
	CommonName   string        `json:"common_name"`
	TTL          time.Duration `json:"ttl"`           // default 10 years
	KeyType      string        `json:"key_type"`      // "ec" (default) | "rsa"
	KeyBits      int           `json:"key_bits"`      // EC: 256/384; RSA: 2048/4096
	Organization []string      `json:"organization,omitempty"`
	Country      []string      `json:"country,omitempty"`
}

// caRecord is stored in the barrier; the private key is protected by envelope
// encryption at the barrier layer.
type caRecord struct {
	CertPEM   string    `json:"cert_pem"`
	KeyPEM    string    `json:"key_pem"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Role defines what certificates a PKI role is allowed to issue.
type Role struct {
	Name            string        `json:"name"`
	AllowedDomains  []string      `json:"allowed_domains"`
	AllowSubdomains bool          `json:"allow_subdomains"`
	AllowIPSANs     bool          `json:"allow_ip_sans"`
	AllowLocalhost  bool          `json:"allow_localhost"`
	KeyType         string        `json:"key_type"`  // "ec" | "rsa"
	KeyBits         int           `json:"key_bits"`
	DefaultTTL      time.Duration `json:"default_ttl"`
	MaxTTL          time.Duration `json:"max_ttl"`
	// ServerFlag adds ExtKeyUsageServerAuth to issued certs.
	ServerFlag bool `json:"server_flag"`
	// ClientFlag adds ExtKeyUsageClientAuth to issued certs.
	ClientFlag bool `json:"client_flag"`
}

// CertRecord tracks an issued certificate (metadata only, no private key).
type CertRecord struct {
	Serial     string    `json:"serial"`
	RoleName   string    `json:"role_name"`
	CommonName string    `json:"common_name"`
	SANs       []string  `json:"sans"`
	IssuedAt   time.Time `json:"issued_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Revoked    bool      `json:"revoked"`
	RevokedAt  time.Time `json:"revoked_at,omitempty"`
}

// IssuedCert is returned to the caller after a successful issuance.
// The private key is returned once and never stored by Tuck.
type IssuedCert struct {
	Serial        string        `json:"serial"`
	CertPEM       string        `json:"certificate"`
	PrivateKeyPEM string        `json:"private_key"`
	IssuingCAPEM  string        `json:"issuing_ca"`
	ExpiresAt     time.Time     `json:"expires_at"`
	TTL           time.Duration `json:"ttl"`
}

type barrier interface {
	Get(ctx context.Context, key string) (*physical.Entry, error)
	Put(ctx context.Context, entry *physical.Entry) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// Manager manages a PKI CA, roles, and issued certificate records.
type Manager struct{ b barrier }

// NewManager creates a Manager backed by b.
func NewManager(b barrier) *Manager { return &Manager{b: b} }

// GenerateCA creates a new self-signed root CA, persists it, and returns the
// CA certificate PEM. The private key is stored inside the barrier.
func (m *Manager) GenerateCA(ctx context.Context, cfg *CAConfig) (string, error) {
	if cfg.CommonName == "" {
		return "", errors.New("pki: common_name is required")
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 10 * 365 * 24 * time.Hour
	}
	if cfg.KeyType == "" {
		cfg.KeyType = "ec"
	}

	priv, err := generateKey(cfg.KeyType, cfg.KeyBits)
	if err != nil {
		return "", fmt.Errorf("pki: generate CA key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
			Country:      cfg.Country,
		},
		NotBefore:             now,
		NotAfter:              now.Add(cfg.TTL),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, priv.(crypto.Signer).Public(), priv.(crypto.Signer))
	if err != nil {
		return "", fmt.Errorf("pki: create CA cert: %w", err)
	}

	certPEM := encodeCert(certDER)
	keyPEM, err := encodeKey(priv)
	if err != nil {
		return "", err
	}

	rec := &caRecord{
		CertPEM:   certPEM,
		KeyPEM:    keyPEM,
		IssuedAt:  now,
		ExpiresAt: now.Add(cfg.TTL),
	}
	if err := m.put(ctx, caStorageKey, rec); err != nil {
		return "", err
	}
	return certPEM, nil
}

// ImportCA imports an existing CA certificate + private key (both PEM-encoded).
// The key is validated but not exported again.
func (m *Manager) ImportCA(ctx context.Context, certPEM, keyPEM string) error {
	cert, err := parseCert(certPEM)
	if err != nil {
		return fmt.Errorf("pki: import CA cert: %w", err)
	}
	if _, err := parseKey(keyPEM); err != nil {
		return fmt.Errorf("pki: import CA key: %w", err)
	}
	rec := &caRecord{
		CertPEM:   certPEM,
		KeyPEM:    keyPEM,
		IssuedAt:  cert.NotBefore,
		ExpiresAt: cert.NotAfter,
	}
	return m.put(ctx, caStorageKey, rec)
}

// GetCACert returns the CA certificate PEM. Does not expose the private key.
func (m *Manager) GetCACert(ctx context.Context) (string, error) {
	var rec caRecord
	if err := m.get(ctx, caStorageKey, &rec); err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", ErrNoCA
		}
		return "", err
	}
	return rec.CertPEM, nil
}

// GetCRL generates and returns a PEM-encoded CRL signed by the CA.
// All revoked certificates are included.
func (m *Manager) GetCRL(ctx context.Context) (string, error) {
	var rec caRecord
	if err := m.get(ctx, caStorageKey, &rec); err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", ErrNoCA
		}
		return "", err
	}
	caCert, err := parseCert(rec.CertPEM)
	if err != nil {
		return "", err
	}
	caPriv, err := parseKey(rec.KeyPEM)
	if err != nil {
		return "", err
	}

	keys, err := m.b.List(ctx, certsPrefix)
	if err != nil {
		return "", err
	}
	var entries []x509.RevocationListEntry
	for _, k := range keys {
		var cr CertRecord
		if err := m.get(ctx, k, &cr); err != nil {
			continue
		}
		if cr.Revoked {
			sn := new(big.Int)
			sn.SetString(cr.Serial, 16)
			entries = append(entries, x509.RevocationListEntry{
				SerialNumber:   sn,
				RevocationTime: cr.RevokedAt,
			})
		}
	}

	now := time.Now().UTC()
	tmpl := &x509.RevocationList{
		Number:                    big.NewInt(now.Unix()),
		ThisUpdate:                now,
		NextUpdate:                now.Add(24 * time.Hour),
		RevokedCertificateEntries: entries,
	}
	crlDER, err := x509.CreateRevocationList(rand.Reader, tmpl, caCert, caPriv.(crypto.Signer))
	if err != nil {
		return "", fmt.Errorf("pki: create CRL: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crlDER})), nil
}

// --- Role CRUD ---

func (m *Manager) PutRole(ctx context.Context, r *Role) error {
	if r.KeyType == "" {
		r.KeyType = "ec"
	}
	if r.DefaultTTL <= 0 {
		r.DefaultTTL = 72 * time.Hour
	}
	if r.MaxTTL <= 0 {
		r.MaxTTL = 8760 * time.Hour
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
	return m.listTrimmed(ctx, rolesPrefix)
}

// --- Certificate issuance ---

// IssueCert generates a TLS certificate signed by the CA, constrained by role.
// altNames may contain DNS names and IP addresses; IPs are only allowed when
// role.AllowIPSANs is true. requestedTTL=0 uses role.DefaultTTL.
// The private key is returned in IssuedCert and is never persisted by Tuck.
func (m *Manager) IssueCert(ctx context.Context, roleName, commonName string, altNames []string, requestedTTL time.Duration) (*IssuedCert, error) {
	role, err := m.GetRole(ctx, roleName)
	if err != nil {
		return nil, err
	}
	if err := m.validateNames(role, commonName, altNames); err != nil {
		return nil, err
	}

	ttl := requestedTTL
	if ttl <= 0 {
		ttl = role.DefaultTTL
	}
	if role.MaxTTL > 0 && ttl > role.MaxTTL {
		ttl = role.MaxTTL
	}

	var caRec caRecord
	if err := m.get(ctx, caStorageKey, &caRec); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNoCA
		}
		return nil, err
	}
	caCert, err := parseCert(caRec.CertPEM)
	if err != nil {
		return nil, err
	}
	caPriv, err := parseKey(caRec.KeyPEM)
	if err != nil {
		return nil, err
	}

	leafPriv, err := generateKey(role.KeyType, role.KeyBits)
	if err != nil {
		return nil, fmt.Errorf("pki: generate leaf key: %w", err)
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	var dnsNames []string
	var ipAddrs []net.IP
	if net.ParseIP(commonName) == nil && commonName != "localhost" {
		dnsNames = append(dnsNames, commonName)
	} else if ip := net.ParseIP(commonName); ip != nil {
		ipAddrs = append(ipAddrs, ip)
	}
	for _, san := range altNames {
		if ip := net.ParseIP(san); ip != nil {
			ipAddrs = append(ipAddrs, ip)
		} else {
			dnsNames = append(dnsNames, san)
		}
	}
	// Deduplicate DNS names.
	dnsNames = uniqueStrings(dnsNames)

	keyUsage := x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	var extKeyUsage []x509.ExtKeyUsage
	if role.ServerFlag || (!role.ServerFlag && !role.ClientFlag) {
		extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageServerAuth)
	}
	if role.ClientFlag {
		extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageClientAuth)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddrs,
		NotBefore:    now,
		NotAfter:     expiresAt,
		KeyUsage:     keyUsage,
		ExtKeyUsage:  extKeyUsage,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, leafPriv.(crypto.Signer).Public(), caPriv.(crypto.Signer))
	if err != nil {
		return nil, fmt.Errorf("pki: sign cert: %w", err)
	}

	certPEM := encodeCert(certDER)
	leafKeyPEM, err := encodeKey(leafPriv)
	if err != nil {
		return nil, err
	}

	serialHex := fmt.Sprintf("%x", serial)
	cr := &CertRecord{
		Serial:     serialHex,
		RoleName:   roleName,
		CommonName: commonName,
		SANs:       altNames,
		IssuedAt:   now,
		ExpiresAt:  expiresAt,
	}
	if err := m.put(ctx, certsPrefix+serialHex, cr); err != nil {
		return nil, err
	}

	return &IssuedCert{
		Serial:        serialHex,
		CertPEM:       certPEM,
		PrivateKeyPEM: leafKeyPEM,
		IssuingCAPEM:  caRec.CertPEM,
		ExpiresAt:     expiresAt,
		TTL:           ttl,
	}, nil
}

// RevokeCert marks a certificate as revoked. It will appear in subsequent CRLs.
func (m *Manager) RevokeCert(ctx context.Context, serial string) error {
	var cr CertRecord
	if err := m.get(ctx, certsPrefix+serial, &cr); err != nil {
		return err
	}
	cr.Revoked = true
	cr.RevokedAt = time.Now().UTC()
	return m.put(ctx, certsPrefix+serial, &cr)
}

// GetCert returns the metadata record for a certificate (no private key).
func (m *Manager) GetCert(ctx context.Context, serial string) (*CertRecord, error) {
	var cr CertRecord
	if err := m.get(ctx, certsPrefix+serial, &cr); err != nil {
		return nil, err
	}
	return &cr, nil
}

// ListCerts returns all issued certificate serial numbers.
func (m *Manager) ListCerts(ctx context.Context) ([]string, error) {
	return m.listTrimmed(ctx, certsPrefix)
}

// --- domain validation ---

func (m *Manager) validateNames(role *Role, cn string, altNames []string) error {
	all := append([]string{cn}, altNames...)
	for _, name := range all {
		if name == "" {
			continue
		}
		if ip := net.ParseIP(name); ip != nil {
			if !role.AllowIPSANs {
				return fmt.Errorf("%w: IP SANs not allowed by role", ErrDomainDenied)
			}
			if ip.IsLoopback() && !role.AllowLocalhost {
				return fmt.Errorf("%w: loopback IP not allowed by role", ErrDomainDenied)
			}
			continue
		}
		if name == "localhost" {
			if !role.AllowLocalhost {
				return fmt.Errorf("%w: localhost not allowed by role", ErrDomainDenied)
			}
			continue
		}
		if !domainAllowed(role.AllowedDomains, role.AllowSubdomains, name) {
			return fmt.Errorf("%w: %q not permitted by allowed_domains", ErrDomainDenied, name)
		}
	}
	return nil
}

func domainAllowed(allowed []string, allowSub bool, name string) bool {
	name = strings.ToLower(name)
	for _, d := range allowed {
		d = strings.ToLower(d)
		if name == d {
			return true
		}
		if allowSub && strings.HasSuffix(name, "."+d) {
			return true
		}
	}
	return false
}

// --- crypto helpers ---

func generateKey(keyType string, bits int) (crypto.PrivateKey, error) {
	switch strings.ToLower(keyType) {
	case "rsa":
		if bits == 0 {
			bits = 2048
		}
		return rsa.GenerateKey(rand.Reader, bits)
	default: // "ec"
		curve := elliptic.P256()
		if bits == 384 {
			curve = elliptic.P384()
		}
		return ecdsa.GenerateKey(curve, rand.Reader)
	}
}

func encodeCert(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func encodeKey(key crypto.PrivateKey) (string, error) {
	var block *pem.Block
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		der, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return "", fmt.Errorf("pki: marshal EC key: %w", err)
		}
		block = &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}
	case *rsa.PrivateKey:
		block = &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	default:
		return "", fmt.Errorf("pki: unsupported key type %T", key)
	}
	return string(pem.EncodeToMemory(block)), nil
}

func parseCert(pemStr string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, ErrInvalidPEM
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseKey(pemStr string) (crypto.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, ErrInvalidPEM
	}
	switch block.Type {
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	}
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	n, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("pki: generate serial: %w", err)
	}
	return n, nil
}

func uniqueStrings(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	var out []string
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
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

func (m *Manager) listTrimmed(ctx context.Context, prefix string) ([]string, error) {
	keys, err := m.b.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	for i, k := range keys {
		keys[i] = strings.TrimPrefix(k, prefix)
	}
	return keys, nil
}

