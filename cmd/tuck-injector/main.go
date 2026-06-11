// Command tuck-injector is the Tuck mutating admission webhook server.
// It intercepts Pod creation requests and injects a tuck-agent init container
// that fetches secrets from Tuck before the application starts.
//
// The webhook server must be reachable by the Kubernetes API server and serve
// TLS. Provide cert/key via --tls-cert/--tls-key (e.g. from cert-manager).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NAGenaev/tuck/internal/injector"
)

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	addr         := flag.String("addr", ":8443", "HTTPS listen address")
	tlsCert      := flag.String("tls-cert", "/tuck/certs/tls.crt", "path to TLS certificate (PEM)")
	tlsKey       := flag.String("tls-key", "/tuck/certs/tls.key", "path to TLS private key (PEM)")
	agentImage   := flag.String("agent-image", "ghcr.io/nagenaev/tuck-agent:latest", "default tuck-agent container image")
	flag.Parse()

	if *versionFlag {
		fmt.Fprintf(os.Stdout, "tuck-injector %s\n", Version)
		os.Exit(0)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	mux := http.NewServeMux()

	webhook := injector.NewHandler(*agentImage, log)
	mux.Handle("/mutate", webhook)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	go func() {
		log.Info("tuck-injector: listening", "addr", *addr, "agent-image", *agentImage)
		if err := srv.ListenAndServeTLS(*tlsCert, *tlsKey); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()

	log.Info("tuck-injector: shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("shutdown", "err", err)
	}
	log.Info("tuck-injector: shutdown complete")
}
