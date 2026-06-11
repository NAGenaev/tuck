// Command tuck-agent is an init container injected by the Tuck webhook.
// It fetches secrets from a Tuck server and writes them as files to a
// shared in-memory volume, then exits. The application container starts
// only after tuck-agent completes successfully.
//
// Configuration via environment variables:
//
//	TUCK_ADDR        Tuck server base URL (e.g. https://tuck.svc:8200)
//	TUCK_TOKEN       Bearer token (alternative to TUCK_TOKEN_FILE)
//	TUCK_TOKEN_FILE  Path to a file containing the bearer token
//	TUCK_SECRETS     Comma-separated "tuckPath:filename" pairs
//	TUCK_OUTPUT_DIR  Directory to write secret files (default /tuck/secrets)
//	TUCK_INSECURE    Skip TLS certificate verification ("true" = skip)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/injector"
	"github.com/NAGenaev/tuck/pkg/client"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	addr := requiredEnv("TUCK_ADDR")
	token, err := resolveToken()
	if err != nil {
		log.Error("resolve token", "err", err)
		os.Exit(1)
	}

	secretsRaw := os.Getenv("TUCK_SECRETS")
	if secretsRaw == "" {
		log.Error("TUCK_SECRETS is not set")
		os.Exit(1)
	}

	outputDir := os.Getenv("TUCK_OUTPUT_DIR")
	if outputDir == "" {
		outputDir = "/tuck/secrets"
	}

	insecure := os.Getenv("TUCK_INSECURE") == "true"

	specs := injector.ParseSecretsList(secretsRaw)
	if len(specs) == 0 {
		log.Error("TUCK_SECRETS parsed to zero entries", "raw", secretsRaw)
		os.Exit(1)
	}

	if err := os.MkdirAll(outputDir, 0700); err != nil {
		log.Error("create output dir", "dir", outputDir, "err", err)
		os.Exit(1)
	}

	var opts []client.Option
	if insecure {
		opts = append(opts, client.WithInsecure())
	}
	c := client.New(addr, token, opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Info("tuck-agent: fetching secrets", "count", len(specs), "output", outputDir)

	var failed int
	for _, spec := range specs {
		val, err := c.GetSecret(ctx, spec.Path)
		if err != nil {
			log.Error("fetch secret", "path", spec.Path, "err", err)
			failed++
			continue
		}
		if val == nil {
			log.Error("secret not found", "path", spec.Path)
			failed++
			continue
		}

		dest := filepath.Join(outputDir, spec.Filename)
		if err := writeSecretFile(dest, val); err != nil {
			log.Error("write secret file", "path", spec.Path, "dest", dest, "err", err)
			failed++
			continue
		}
		log.Info("wrote secret", "path", spec.Path, "file", spec.Filename)
	}

	if failed > 0 {
		log.Error("tuck-agent: failed to fetch some secrets", "failed", failed, "total", len(specs))
		os.Exit(1)
	}
	log.Info("tuck-agent: all secrets written successfully", "count", len(specs))
}

// writeSecretFile writes val to dest atomically (write to .tmp then rename)
// with mode 0400 so only the owning process can read it.
func writeSecretFile(dest string, val []byte) error {
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, val, 0400); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	return os.Rename(tmp, dest)
}

// resolveToken reads the bearer token from TUCK_TOKEN or TUCK_TOKEN_FILE.
func resolveToken() (string, error) {
	if tok := os.Getenv("TUCK_TOKEN"); tok != "" {
		return tok, nil
	}
	f := os.Getenv("TUCK_TOKEN_FILE")
	if f == "" {
		return "", fmt.Errorf("neither TUCK_TOKEN nor TUCK_TOKEN_FILE is set")
	}
	data, err := os.ReadFile(f)
	if err != nil {
		return "", fmt.Errorf("read token file %q: %w", f, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func requiredEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required env var not set", "var", key)
		os.Exit(1)
	}
	return v
}
