package raft

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	hraft "github.com/hashicorp/raft"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestCA(t *testing.T, cn string) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return &testCA{cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// issue writes a node certificate valid for 127.0.0.1 and returns a TLSConfig
// that trusts only this CA.
func (ca *testCA) issue(t *testing.T, name string) *TLSConfig {
	t.Helper()
	dir := t.TempDir()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	kd, _ := x509.MarshalECPrivateKey(key)
	c := &TLSConfig{CertFile: filepath.Join(dir, "n.crt"), KeyFile: filepath.Join(dir, "n.key"), CAFile: filepath.Join(dir, "ca.crt")}
	for p, b := range map[string][]byte{
		c.CertFile: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		c.KeyFile:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd}),
		c.CAFile:   ca.pem,
	} {
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func TestRaftClusterOverMutualTLS(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	ca := newTestCA(t, "cluster-ca")
	ids := []string{"n1", "n2", "n3"}
	addrs := map[string]string{}
	for _, id := range ids {
		addrs[id] = freeAddr(t)
	}
	dirs := map[string]string{}
	cfgs := map[string]*TLSConfig{}
	for _, id := range ids {
		dirs[id] = t.TempDir()
		cfgs[id] = ca.issue(t, id)
	}
	nodes := map[string]*Backend{}
	t.Cleanup(func() {
		for _, b := range nodes {
			_ = b.Close()
		}
	})
	for _, id := range ids {
		cfg := nodeConfig(id, addrs[id], dirs[id])
		cfg.Bootstrap = true
		cfg.TLS = cfgs[id]
		for _, o := range ids {
			if o != id {
				cfg.Peers = append(cfg.Peers, Peer{ID: o, Addr: addrs[o]})
			}
		}
		b, err := Open(cfg)
		if err != nil {
			t.Fatalf("open %s: %v", id, err)
		}
		nodes[id] = b
	}
	var leader *Backend
	eventually(t, "leader", func() bool { _, leader = findLeader(nodes); return leader != nil })
	if err := leader.Put(context.Background(), entry("k", "v")); err != nil {
		t.Fatal(err)
	}
	for id, b := range nodes {
		b := b
		eventually(t, id+" replicated", func() bool { return hasValue(b, "k", "v") })
	}
}

func TestRaftTLSRejectsForeignCA(t *testing.T) {
	good, rogue := newTestCA(t, "good"), newTestCA(t, "rogue")
	srv, err := newTLSStream("127.0.0.1:0", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, good.issue(t, "srv"))
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	go func() {
		for {
			c, err := srv.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); buf := make([]byte, 1); _, _ = c.Read(buf); _, _ = c.Write([]byte("k")) }()
		}
	}()
	addr := hraft.ServerAddress(srv.Listener.Addr().String())

	roundTrip := func(c *TLSConfig) error {
		cl, err := newTLSStream("127.0.0.1:0", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, c)
		if err != nil {
			return err
		}
		defer cl.Close()
		conn, err := cl.Dial(addr, 2*time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := conn.Write([]byte("x")); err != nil {
			return err
		}
		buf := make([]byte, 1)
		if _, err := conn.Read(buf); err != nil {
			return fmt.Errorf("read: %w", err)
		}
		return nil
	}
	if err := roundTrip(good.issue(t, "peer")); err != nil {
		t.Fatalf("peer from the same CA must connect: %v", err)
	}
	if err := roundTrip(rogue.issue(t, "evil")); err == nil {
		t.Fatal("peer from a foreign CA must be rejected")
	}
}
