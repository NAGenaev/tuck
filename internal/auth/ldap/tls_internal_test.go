package ldap

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func TestConfigValidate(t *testing.T) {
	good := Config{URLs: []string{"ldaps://ldap.bank.local:636"}, UserDN: "ou=u,dc=b"}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	cases := map[string]Config{
		"no urls":    {UserDN: "ou=u"},
		"bad scheme": {URLs: []string{"http://x"}, UserDN: "ou=u"},
		"no host":    {URLs: []string{"ldaps://"}, UserDN: "ou=u"},
		"no user dn": {URLs: []string{"ldap://x"}},
		"bad ca":     {URLs: []string{"ldaps://x"}, UserDN: "ou=u", CACert: "not a pem"},
	}
	for name, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestTLSConfigServerName(t *testing.T) {
	c := Config{}
	cfg, err := c.tlsConfig("ldap://dc1.bank.local:389")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName != "dc1.bank.local" {
		t.Fatalf("ServerName = %q", cfg.ServerName)
	}
}

func testCA(t *testing.T) (caPEM string, serverCert tls.Certificate) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test bank CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)

	srvKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "ldap"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caCert, &srvKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})),
		tls.Certificate{Certificate: [][]byte{srvDER}, PrivateKey: srvKey}
}

func TestDialPinnedCA(t *testing.T) {
	caPEM, cert := testCA(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = c.(*tls.Conn).Handshake()
				time.Sleep(200 * time.Millisecond)
				_ = c.Close()
			}()
		}
	}()
	url := "ldaps://" + ln.Addr().String()

	// Without the CA the private certificate must be rejected.
	if _, err := defaultDial(context.Background(), Config{URLs: []string{url}}); err == nil {
		t.Fatal("dial succeeded without trusting the CA")
	} else if !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("unexpected error: %v", err)
	}

	// With ca_cert pinned the handshake verifies.
	conn, err := defaultDial(context.Background(), Config{URLs: []string{url}, CACert: caPEM})
	if err != nil {
		t.Fatalf("dial with pinned CA: %v", err)
	}
	_ = conn.Close()
}
