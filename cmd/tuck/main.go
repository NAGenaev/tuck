// Command tuck runs the Tuck secrets-manager server.
//
// Milestone 0: a single binary, a bbolt file, and a dev seal. Run `tuck` and
// it serves an encrypted KV with zero setup.
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
	if err := c.Start(context.Background()); err != nil {
		log.Fatalf("start core: %v", err)
	}
	log.Printf("tuck: unsealed (dev seal), serving on http://%s", *addr)
	log.Printf("tuck: WARNING dev seal stores the root key in plaintext at %q — dev use only", *sealKey)

	if err := http.ListenAndServe(*addr, api.New(c).Handler()); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
