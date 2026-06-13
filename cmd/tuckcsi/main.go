// Command tuckcsi is the Tuck CSI Node driver.
//
// It registers as a CSI plugin with kubelet via a Unix domain socket and
// serves NodePublishVolume / NodeUnpublishVolume requests. On each publish,
// it fetches the requested secrets from a Tuck server and writes them as
// read-only files inside a per-pod tmpfs mount, so secrets never touch disk.
//
// Volume context attributes (set in StorageClass or CSIVolumeSource):
//
//	tuck.io/addr        — Tuck server base URL (required)
//	tuck.io/paths       — comma-separated secret paths to fetch (required)
//	tuck.io/namespace   — Tuck namespace (optional, defaults to root)
//	tuck.io/kv-version  — "1" or "2" (optional, defaults to "1")
//	tuck.io/insecure    — "true" to skip TLS verification (dev only)
//
// The Tuck token must be supplied as a Kubernetes Secret referenced via
// nodePublishSecretRef, with key "token".
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	csispec "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"

	"github.com/NAGenaev/tuck/internal/csi"
)

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

func main() {
	endpoint := flag.String("endpoint", "unix:///csi/csi.sock", "CSI endpoint (unix:// or tcp://)")
	nodeID   := flag.String("node-id", "", "node ID reported to kubelet (required)")
	version  := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *version {
		fmt.Fprintf(os.Stdout, "tuckcsi %s\n", Version)
		os.Exit(0)
	}
	if *nodeID == "" {
		// Fall back to hostname if not provided.
		h, err := os.Hostname()
		if err != nil {
			log.Fatalf("tuckcsi: --node-id not set and cannot determine hostname: %v", err)
		}
		*nodeID = h
	}

	network, addr, err := parseEndpoint(*endpoint)
	if err != nil {
		log.Fatalf("tuckcsi: invalid endpoint %q: %v", *endpoint, err)
	}

	// Remove stale socket file if present.
	if network == "unix" {
		_ = os.Remove(addr)
	}

	lis, err := net.Listen(network, addr)
	if err != nil {
		log.Fatalf("tuckcsi: listen %s://%s: %v", network, addr, err)
	}

	drv := csi.NewDriver(*nodeID, csi.NewMounter())
	srv := grpc.NewServer()
	csispec.RegisterIdentityServer(srv, drv)
	csispec.RegisterNodeServer(srv, drv)

	log.Printf("tuckcsi %s: listening on %s://%s (node=%s)", Version, network, addr, *nodeID)

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("tuckcsi: serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
	<-quit
	log.Printf("tuckcsi: shutting down")
	srv.GracefulStop()
}

func parseEndpoint(ep string) (network, addr string, err error) {
	switch {
	case len(ep) > 7 && ep[:7] == "unix://":
		return "unix", ep[7:], nil
	case len(ep) > 6 && ep[:6] == "tcp://":
		return "tcp", ep[6:], nil
	default:
		return "", "", fmt.Errorf("must start with unix:// or tcp://")
	}
}
