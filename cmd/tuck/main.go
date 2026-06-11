// Command tuck runs the Tuck secrets-manager server.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/NAGenaev/tuck/internal/api"
	"github.com/NAGenaev/tuck/internal/core"
	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8200", "HTTP listen address")
	dataPath := flag.String("data", "tuck.db", "path to the bbolt data file")
	sealKey := flag.String("dev-seal-key", "tuck-dev-rootkey", "dev seal root key file (INSECURE, dev only)")

	k8sAPI := flag.String("k8s-api", "https://kubernetes.default.svc", "Kubernetes API server URL; empty = disable k8s auth")
	k8sTokenFile := flag.String("k8s-token-file", "/var/run/secrets/kubernetes.io/serviceaccount/token", "path to Tuck's own k8s SA token")
	k8sCaFile := flag.String("k8s-ca-file", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "path to k8s CA certificate")
	flag.Parse()

	backend, err := physical.OpenBolt(*dataPath)
	if err != nil {
		log.Fatalf("open backend: %v", err)
	}
	defer backend.Close()

	var reviewer k8sauth.Reviewer
	if *k8sAPI != "" {
		if _, err := os.Stat(*k8sTokenFile); err == nil {
			r, err := k8sauth.NewClientFromFiles(*k8sAPI, *k8sTokenFile, *k8sCaFile)
			if err != nil {
				log.Printf("k8s auth disabled: build client: %v", err)
			} else {
				reviewer = r
				log.Printf("tuck: kubernetes auth enabled (%s)", *k8sAPI)
			}
		} else {
			log.Printf("tuck: kubernetes auth disabled (token file %q not found)", *k8sTokenFile)
		}
	}

	c := core.NewWithK8s(backend, seal.NewDev(*sealKey), reviewer)
	rootTok, err := c.Start(context.Background())
	if err != nil {
		log.Fatalf("start core: %v", err)
	}
	if rootTok != nil {
		log.Printf("==========================================================")
		log.Printf("ROOT TOKEN (shown once — store it securely):")
		log.Printf("  %s", rootTok.ID)
		log.Printf("==========================================================")
	}

	log.Printf("tuck: unsealed (dev seal), serving on http://%s", *addr)
	log.Printf("tuck: WARNING dev seal stores the root key in plaintext at %q — dev use only", *sealKey)

	if err := http.ListenAndServe(*addr, api.New(c).Handler()); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
