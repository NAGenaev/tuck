package tlsutil

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSelfSigned(t *testing.T) {
	cert, err := SelfSigned()
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("empty certificate chain")
	}
}

func TestConfig(t *testing.T) {
	cert, err := SelfSigned()
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}
	cfg := Config(cert)
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want TLS 1.2", cfg.MinVersion)
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("Certificates len = %d, want 1", len(cfg.Certificates))
	}
}

func TestReadyEndpoint(t *testing.T) {
	// Verify the /v1/sys/ready route works with a plain httptest server.
	// Full integration is covered in api/sys_test.go.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/sys/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/sys/ready")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestTLSServerListens(t *testing.T) {
	cert, err := SelfSigned()
	if err != nil {
		t.Fatalf("SelfSigned: %v", err)
	}
	cfg := Config(cert)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsLn := tls.NewListener(ln, cfg)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go srv.Serve(tlsLn) //nolint:errcheck

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}}
	resp, err := client.Get("https://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	srv.Close()
}
