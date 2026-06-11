// Command tuck runs the Tuck secrets-manager server.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/NAGenaev/tuck/internal/api"
	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8200", "HTTP listen address")
	dataPath := flag.String("data", "tuck.db", "path to the bbolt data file")
	sealKey := flag.String("dev-seal-key", "tuck-dev-rootkey", "dev seal root key file (INSECURE, dev only)")
	flag.Parse()

	backend, err := physical.OpenBolt(*dataPath)
	if err != nil {
		log.Fatalf("open backend: %v", err)
	}
	defer backend.Close()

	c := core.New(backend, seal.NewDev(*sealKey))
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
