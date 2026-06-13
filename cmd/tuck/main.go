// Command tuck runs the Tuck secrets-manager server.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/NAGenaev/tuck/internal/api"
	"github.com/NAGenaev/tuck/internal/audit"
	"github.com/NAGenaev/tuck/internal/config"
	"github.com/NAGenaev/tuck/internal/core"
	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/physical"
	physraft "github.com/NAGenaev/tuck/internal/physical/raft"
	"github.com/NAGenaev/tuck/internal/seal"
	"github.com/NAGenaev/tuck/internal/telemetry"
	"github.com/NAGenaev/tuck/internal/tlsutil"
	"github.com/NAGenaev/tuck/internal/version"
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	configFile := flag.String("config", "", "path to YAML config file (default: $TUCK_CONFIG, then tuck.yaml if it exists)")

	// Network
	addr := flag.String("addr", "127.0.0.1:8200", "HTTP(S) listen address")
	dataPath := flag.String("data", "tuck.db", "path to the bbolt data file")

	// TLS
	tlsCert := flag.String("tls-cert", "", "path to TLS certificate file (PEM)")
	tlsKey := flag.String("tls-key", "", "path to TLS private key file (PEM)")
	tlsAuto := flag.Bool("tls-auto", false, "generate a self-signed certificate (dev/testing only)")

	// Seal type
	sealType := flag.String("seal-type", "dev", "seal type: dev, shamir, transit, awskms, gcpkms")

	// Dev seal
	devSealKey := flag.String("dev-seal-key", "tuck-dev-rootkey", "dev seal: root key file path (INSECURE)")

	// Shamir seal
	shamirConfig := flag.String("seal-shamir-config", "tuck-shamir.json", "Shamir seal: path to N/K config file")
	shamirN := flag.Int("seal-shamir-n", 5, "Shamir seal: total number of key shares")
	shamirK := flag.Int("seal-shamir-k", 3, "Shamir seal: minimum shares to unseal")

	// Transit seal (Vault-compatible)
	transitAddr := flag.String("seal-transit-addr", "", "Transit seal: server base URL (e.g. https://vault:8200)")
	transitKey := flag.String("seal-transit-key", "tuck-seal", "Transit seal: Transit encryption key name")
	transitToken := flag.String("seal-transit-token", "", "Transit seal: bearer token (use env TUCK_TRANSIT_TOKEN to avoid ps exposure)")
	transitKeyFile := flag.String("seal-transit-key-file", "tuck-transit.enc", "Transit seal: file to store wrapped root key ciphertext")

	// AWS KMS seal
	awsKMSKeyID      := flag.String("seal-awskms-key-id", "", "AWS KMS seal: CMK ARN or alias (e.g. alias/tuck-seal)")
	awsKMSRegion     := flag.String("seal-awskms-region", "", "AWS KMS seal: region (e.g. us-east-1); empty = from AWS_DEFAULT_REGION")
	awsKMSKeyFile    := flag.String("seal-awskms-key-file", "tuck-awskms.enc", "AWS KMS seal: file to store wrapped root key ciphertext")

	// GCP Cloud KMS seal
	gcpKMSKeyName := flag.String("seal-gcpkms-key-name", "", "GCP KMS seal: full CryptoKey resource name (projects/.../cryptoKeys/...)")
	gcpKMSKeyFile := flag.String("seal-gcpkms-key-file", "tuck-gcpkms.enc", "GCP KMS seal: file to store wrapped root key ciphertext")

	// Azure Key Vault seal
	azureKVVaultURL   := flag.String("seal-azurekv-vault-url", "", "Azure Key Vault seal: vault URL (e.g. https://my-vault.vault.azure.net)")
	azureKVKeyName    := flag.String("seal-azurekv-key-name", "", "Azure Key Vault seal: RSA key name in the vault")
	azureKVAlgorithm  := flag.String("seal-azurekv-algorithm", "", "Azure Key Vault seal: encryption algorithm (default RSA-OAEP-256)")
	azureKVKeyFile    := flag.String("seal-azurekv-key-file", "tuck-azurekv.enc", "Azure Key Vault seal: file to store wrapped root key ciphertext")

	// Cluster (Raft HA)
	clusterMode      := flag.Bool("cluster", false, "enable Raft HA backend (replaces bbolt with a replicated Raft log)")
	clusterNodeID    := flag.String("cluster-node-id", "", "unique node ID for this instance (defaults to hostname)")
	clusterBindAddr  := flag.String("cluster-bind-addr", "0.0.0.0:8201", "Raft RPC listen address")
	clusterAdvertise := flag.String("cluster-advertise", "", "Raft RPC advertise address (defaults to --cluster-bind-addr)")
	clusterDir       := flag.String("cluster-dir", "tuck-raft", "directory for Raft logs, snapshots, and FSM state")
	clusterBootstrap := flag.Bool("cluster-bootstrap", false, "bootstrap a new cluster (first node only; ignored on restarts)")
	clusterJoin      := flag.String("cluster-join", "", "leader HTTP address to join an existing cluster (e.g. https://leader:8200)")
	clusterPeers     := flag.String("cluster-peers", "", "comma-separated bootstrap peers: id=raftAddr,... (used with --cluster-bootstrap)")

	// Audit log
	auditLog        := flag.String("audit-log", "", "path for the file audit log (empty = discard); rotated at --audit-max-size bytes")
	auditMaxSize    := flag.Int64("audit-max-size", audit.DefaultMaxSize, "max audit log file size in bytes before rotation")
	auditMaxBackups := flag.Int("audit-max-backups", audit.DefaultMaxBackups, "number of rotated audit log files to keep")

	// Observability
	otelEndpoint := flag.String("otel-endpoint", "", "OpenTelemetry OTLP HTTP endpoint (e.g. http://otel-collector:4318); empty = disabled")

	// Kubernetes auth
	k8sAPI := flag.String("k8s-api", "https://kubernetes.default.svc", "Kubernetes API server URL; empty = disable k8s auth")
	k8sTokenFile := flag.String("k8s-token-file", "/var/run/secrets/kubernetes.io/serviceaccount/token", "path to Tuck's own k8s SA token")
	k8sCaFile := flag.String("k8s-ca-file", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt", "path to k8s CA certificate")

	flag.Parse()

	if *versionFlag {
		fmt.Fprintf(os.Stdout, "tuck %s\n", version.String())
		os.Exit(0)
	}

	lockMemory()

	// Load config file and apply values for flags that were NOT set on the CLI.
	// Priority: CLI flag > env var > config file > built-in default.
	cfg, resolvedCfgPath, cfgErr := config.Load(*configFile)
	if cfgErr != nil {
		log.Fatalf("config: %v", cfgErr)
	}
	if cfg != nil {
		// Track which flags were explicitly provided on the command line.
		explicit := make(map[string]bool)
		flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

		apply := func(name, val string) {
			if !explicit[name] && val != "" {
				_ = flag.Set(name, val)
			}
		}
		applyBool := func(name string, val bool) {
			if !explicit[name] && val {
				_ = flag.Set(name, "true")
			}
		}
		applyInt := func(name string, val int) {
			if !explicit[name] && val != 0 {
				_ = flag.Set(name, fmt.Sprintf("%d", val))
			}
		}

		apply("addr", cfg.Addr)
		apply("data", cfg.Data)

		apply("tls-cert", cfg.TLS.Cert)
		apply("tls-key", cfg.TLS.Key)
		applyBool("tls-auto", cfg.TLS.Auto)

		apply("seal-type", cfg.Seal.Type)
		apply("dev-seal-key", cfg.Seal.Dev.Key)

		apply("seal-shamir-config", cfg.Seal.Shamir.Config)
		applyInt("seal-shamir-n", cfg.Seal.Shamir.N)
		applyInt("seal-shamir-k", cfg.Seal.Shamir.K)

		apply("seal-transit-addr", cfg.Seal.Transit.Addr)
		apply("seal-transit-key", cfg.Seal.Transit.Key)
		apply("seal-transit-key-file", cfg.Seal.Transit.KeyFile)
		// Transit token: config file value only if env is also absent.
		if !explicit["seal-transit-token"] && os.Getenv("TUCK_TRANSIT_TOKEN") == "" && cfg.Seal.Transit.Token != "" {
			_ = flag.Set("seal-transit-token", cfg.Seal.Transit.Token)
		}

		apply("seal-awskms-key-id", cfg.Seal.AWSKMS.KeyID)
		apply("seal-awskms-region", cfg.Seal.AWSKMS.Region)
		apply("seal-awskms-key-file", cfg.Seal.AWSKMS.KeyFile)

		apply("seal-gcpkms-key-name", cfg.Seal.GCPKMS.KeyName)
		apply("seal-gcpkms-key-file", cfg.Seal.GCPKMS.KeyFile)

		apply("seal-azurekv-vault-url", cfg.Seal.AzureKV.VaultURL)
		apply("seal-azurekv-key-name", cfg.Seal.AzureKV.KeyName)
		apply("seal-azurekv-algorithm", cfg.Seal.AzureKV.Algorithm)
		apply("seal-azurekv-key-file", cfg.Seal.AzureKV.KeyFile)

		applyBool("cluster", cfg.Cluster.Enabled)
		apply("cluster-node-id", cfg.Cluster.NodeID)
		apply("cluster-bind-addr", cfg.Cluster.BindAddr)
		apply("cluster-advertise", cfg.Cluster.Advertise)
		apply("cluster-dir", cfg.Cluster.Dir)
		applyBool("cluster-bootstrap", cfg.Cluster.Bootstrap)
		apply("cluster-join", cfg.Cluster.Join)
		apply("cluster-peers", cfg.Cluster.Peers)

		apply("k8s-api", cfg.Kubernetes.API)
		apply("k8s-token-file", cfg.Kubernetes.TokenFile)
		apply("k8s-ca-file", cfg.Kubernetes.CAFile)

		apply("otel-endpoint", cfg.Telemetry.OtelEndpoint)

		log.Printf("tuck: loaded config from %q", resolvedCfgPath)
	}

	// --- Backend ---
	var (
		backend physical.Backend
		err     error
	)
	if *clusterMode {
		nodeID := *clusterNodeID
		if nodeID == "" {
			if h, herr := os.Hostname(); herr == nil {
				nodeID = h
			} else {
				nodeID = "node1"
			}
		}
		var peers []physraft.Peer
		if *clusterPeers != "" {
			for _, p := range splitComma(*clusterPeers) {
				if idx := indexOf(p, '='); idx >= 0 {
					peers = append(peers, physraft.Peer{ID: p[:idx], Addr: p[idx+1:]})
				}
			}
		}
		rb, raftErr := physraft.Open(physraft.Config{
			NodeID:        nodeID,
			BindAddr:      *clusterBindAddr,
			AdvertiseAddr: *clusterAdvertise,
			DataDir:       *clusterDir,
			Bootstrap:     *clusterBootstrap,
			Peers:         peers,
		})
		if raftErr != nil {
			log.Fatalf("open raft backend: %v", raftErr)
		}
		defer rb.Close()

		// Auto-join an existing cluster via the leader's HTTP API.
		if *clusterJoin != "" {
			if joinErr := clusterJoinLeader(*clusterJoin, nodeID, *clusterBindAddr); joinErr != nil {
				log.Fatalf("cluster join: %v", joinErr)
			}
			log.Printf("tuck: joined cluster via %s", *clusterJoin)
		}

		backend = rb
		log.Printf("tuck: Raft HA backend (node=%s bind=%s dir=%s)", nodeID, *clusterBindAddr, *clusterDir)
	} else {
		bb, bbErr := physical.OpenBolt(*dataPath)
		if bbErr != nil {
			log.Fatalf("open backend: %v", bbErr)
		}
		defer bb.Close()
		backend = bb
	}

	// --- Seal ---
	var s seal.Seal
	switch *sealType {
	case "dev":
		s = seal.NewDev(*devSealKey)

	case "shamir":
		var sh *seal.ShamirSeal
		if _, statErr := os.Stat(*shamirConfig); statErr == nil {
			// Config file exists → restart: load N/K from it.
			sh, err = seal.NewShamirFromConfig(*shamirConfig)
			if err != nil {
				log.Fatalf("shamir seal: load config: %v", err)
			}
		} else {
			// First init: use flags, write config.
			if *shamirK > *shamirN || *shamirK < 2 {
				log.Fatalf("shamir seal: invalid k=%d n=%d (need 2 <= k <= n)", *shamirK, *shamirN)
			}
			sh, err = seal.NewShamir(*shamirConfig, *shamirN, *shamirK)
			if err != nil {
				log.Fatalf("shamir seal: init: %v", err)
			}
		}
		s = sh
		log.Printf("tuck: Shamir seal (%d-of-%d, config=%s)", sh.K(), sh.N(), *shamirConfig)

	case "transit":
		if *transitAddr == "" {
			log.Fatalf("transit seal requires --seal-transit-addr")
		}
		tok := *transitToken
		if tok == "" {
			tok = os.Getenv("TUCK_TRANSIT_TOKEN")
		}
		s = seal.NewTransit(*transitAddr, *transitKey, tok, *transitKeyFile)
		log.Printf("tuck: Transit seal (addr=%s key=%s)", *transitAddr, *transitKey)

	case "awskms":
		if *awsKMSKeyID == "" {
			log.Fatalf("awskms seal requires --seal-awskms-key-id")
		}
		s = seal.NewAWSKMS(*awsKMSKeyID, *awsKMSRegion, *awsKMSKeyFile)
		log.Printf("tuck: AWS KMS seal (key-id=%s region=%s)", *awsKMSKeyID, *awsKMSRegion)

	case "gcpkms":
		if *gcpKMSKeyName == "" {
			log.Fatalf("gcpkms seal requires --seal-gcpkms-key-name")
		}
		s = seal.NewGCPKMS(*gcpKMSKeyName, *gcpKMSKeyFile)
		log.Printf("tuck: GCP Cloud KMS seal (key-name=%s)", *gcpKMSKeyName)

	case "azurekv":
		if *azureKVVaultURL == "" {
			log.Fatalf("azurekv seal requires --seal-azurekv-vault-url")
		}
		if *azureKVKeyName == "" {
			log.Fatalf("azurekv seal requires --seal-azurekv-key-name")
		}
		s = seal.NewAzureKV(*azureKVVaultURL, *azureKVKeyName, *azureKVAlgorithm, *azureKVKeyFile)
		log.Printf("tuck: Azure Key Vault seal (vault=%s key=%s)", *azureKVVaultURL, *azureKVKeyName)

	default:
		log.Fatalf("unknown seal type %q; valid: dev, shamir, transit, awskms, gcpkms, azurekv", *sealType)
	}

	// --- Kubernetes auth ---
	var reviewer k8sauth.Reviewer
	if *k8sAPI != "" {
		if _, statErr := os.Stat(*k8sTokenFile); statErr == nil {
			r, buildErr := k8sauth.NewClientFromFiles(*k8sAPI, *k8sTokenFile, *k8sCaFile)
			if buildErr != nil {
				log.Printf("tuck: kubernetes auth disabled: %v", buildErr)
			} else {
				reviewer = r
				log.Printf("tuck: kubernetes auth enabled (%s)", *k8sAPI)
			}
		} else {
			log.Printf("tuck: kubernetes auth disabled (token file %q not found)", *k8sTokenFile)
		}
	}

	// --- Core ---
	c := core.NewWithK8s(backend, s, reviewer)

	// --- Audit log ---
	if *auditLog != "" {
		rl, auditErr := audit.NewRotatingFileLogger(*auditLog, *auditMaxSize, *auditMaxBackups)
		if auditErr != nil {
			log.Fatalf("audit log: %v", auditErr)
		}
		c.SetDispatcher(audit.NewDispatcher(rl))
		log.Printf("tuck: audit log → %s (max %d MiB, keep %d)", *auditLog, *auditMaxSize>>20, *auditMaxBackups)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)

	// --- OpenTelemetry ---
	otelShutdown, err := telemetry.Init(ctx, *otelEndpoint)
	if err != nil {
		log.Fatalf("init telemetry: %v", err)
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = otelShutdown(shutCtx)
	}()
	defer stop()

	c.StartGC(ctx)

	startResult, startErr := c.Start(ctx)
	if startErr != nil && !errors.Is(startErr, core.ErrNeedsUnseal) {
		log.Fatalf("start core: %v", startErr)
	}
	if startResult != nil {
		log.Printf("==========================================================")
		log.Printf("ROOT TOKEN (shown once — store it securely):")
		log.Printf("  %s", startResult.RootToken.ID)
		if len(startResult.Shares) > 0 {
			log.Printf("SHAMIR SHARES (distribute to operators — never store together):")
			for i, sh := range startResult.Shares {
				log.Printf("  [%d] %s", i+1, sh)
			}
		}
		log.Printf("==========================================================")
	}

	// --- TLS ---
	var tlsCfg *tls.Config
	switch {
	case *tlsAuto:
		cert, certErr := tlsutil.SelfSigned()
		if certErr != nil {
			log.Fatalf("generate self-signed cert: %v", certErr)
		}
		tlsCfg = tlsutil.Config(cert)
		log.Printf("tuck: TLS enabled (auto-generated self-signed — dev only)")
	case *tlsCert != "" && *tlsKey != "":
		cert, certErr := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
		if certErr != nil {
			log.Fatalf("load TLS cert/key: %v", certErr)
		}
		tlsCfg = tlsutil.Config(cert)
		log.Printf("tuck: TLS enabled (cert=%s)", *tlsCert)
	case *tlsCert != "" || *tlsKey != "":
		log.Fatalf("--tls-cert and --tls-key must be provided together")
	}

	// --- HTTP server with production timeouts ---
	srv := &http.Server{
		Handler:           telemetry.Middleware(api.New(c).Handler()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       5 * time.Minute,
		MaxHeaderBytes:    1 << 20, // 1 MiB
	}

	ln, listenErr := net.Listen("tcp", *addr)
	if listenErr != nil {
		log.Fatalf("listen %s: %v", *addr, listenErr)
	}

	go func() {
		var serveErr error
		if tlsCfg != nil {
			serveErr = srv.Serve(tls.NewListener(ln, tlsCfg))
		} else {
			serveErr = srv.Serve(ln)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Fatalf("serve: %v", serveErr)
		}
	}()

	scheme := "http"
	if tlsCfg != nil {
		scheme = "https"
	}
	sealState := "unsealed"
	if errors.Is(startErr, core.ErrNeedsUnseal) {
		sealState = "sealed"
	}
	log.Printf(`{"level":"info","msg":"tuck started","version":%q,"commit":%q,"seal":%q,"addr":%q,"state":%q}`,
		version.Version, version.Commit, *sealType, *addr, sealState)
	if errors.Is(startErr, core.ErrNeedsUnseal) {
		log.Printf("tuck: SEALED — provide shards via POST %s://%s/v1/sys/unseal", scheme, *addr)
	} else if *sealType == "dev" {
		log.Printf("tuck: unsealed (dev seal) — %s://%s", scheme, *addr)
		log.Printf("tuck: WARNING dev seal stores root key in plaintext at %q — dev only", *devSealKey)
	} else {
		log.Printf("tuck: unsealed (%s seal) — %s://%s", *sealType, scheme, *addr)
	}

	// Block until SIGTERM/SIGINT.
	<-ctx.Done()
	stop()

	log.Printf("tuck: shutting down (30s grace period)...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("tuck: shutdown error: %v", err)
	}
	c.Seal() // drop barrier key from memory before exit
	log.Printf("tuck: shutdown complete")
}

// clusterJoinLeader POSTs a join request to the leader's HTTP API.
func clusterJoinLeader(leaderHTTP, nodeID, raftAddr string) error {
	import_bytes := fmt.Sprintf(`{"node_id":%q,"raft_addr":%q}`, nodeID, raftAddr)
	req, err := http.NewRequest(http.MethodPost, leaderHTTP+"/v1/sys/cluster/join",
		strings.NewReader(import_bytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// Use the root token env var if set so the join can authenticate.
	if tok := os.Getenv("TUCK_TOKEN"); tok != "" {
		req.Header.Set("X-Tuck-Token", tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("leader returned %s", resp.Status)
	}
	return nil
}

// splitComma splits s by commas, trimming spaces.
func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// indexOf returns the index of byte b in s, or -1.
func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
