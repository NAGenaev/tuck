package raft

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	hraft "github.com/hashicorp/raft"

	"github.com/NAGenaev/tuck/internal/tlsutil"
)

// TLSConfig enables mutual TLS on the Raft transport. Every node presents a
// certificate signed by CAFile and requires the same from its peers; without
// it, Raft RPC is plain TCP.
type TLSConfig struct {
	CertFile string
	KeyFile  string
	// CAFile is the PEM bundle that signs all node certificates. Use a CA
	// dedicated to the cluster: any certificate it signs can join.
	CAFile string
	// ServerName, if set, is checked against the peer certificate. When empty
	// only the chain is verified, which suits nodes addressed by IP.
	ServerName string
}

type tlsStream struct {
	net.Listener
	advertise net.Addr
	client    *tls.Config
}

func (s *tlsStream) Addr() net.Addr { return s.advertise }

func (s *tlsStream) Dial(addr hraft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	return tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", string(addr), s.client)
}

func newTLSStream(bind string, advertise net.Addr, c *TLSConfig) (*tlsStream, error) {
	pem, err := os.ReadFile(c.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read raft tls ca: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("raft tls ca: no certificates found")
	}
	rl, err := tlsutil.NewReloader(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load raft tls cert/key: %w", err)
	}

	server := tlsutil.ReloadingConfig(rl)
	server.ClientAuth = tls.RequireAndVerifyClientCert
	server.ClientCAs = pool

	client := tlsutil.ReloadingConfig(rl)
	client.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		return rl.GetCertificate(nil)
	}
	client.RootCAs = pool
	if c.ServerName != "" {
		client.ServerName = c.ServerName
	} else {
		// Peers are usually addressed by IP: verify the chain ourselves.
		client.InsecureSkipVerify = true
		client.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("raft tls: peer presented no certificate")
			}
			inter := x509.NewCertPool()
			for _, ic := range cs.PeerCertificates[1:] {
				inter.AddCert(ic)
			}
			_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{
				Roots: pool, Intermediates: inter, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			})
			return err
		}
	}

	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, err
	}
	return &tlsStream{Listener: tls.NewListener(ln, server), advertise: advertise, client: client}, nil
}
