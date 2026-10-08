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
	"sync"
	"syscall"
	"time"

	"github.com/NAGenaev/tuck/internal/api"
	"github.com/NAGenaev/tuck/internal/audit"
	"github.com/NAGenaev/tuck/internal/clientip"
	"github.com/NAGenaev/tuck/internal/config"
	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/haregister"
	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/operator"
	"github.com/NAGenaev/tuck/internal/physical"
	physraft "github.com/NAGenaev/tuck/internal/physical/raft"
	"github.com/NAGenaev/tuck/internal/sysconfig"
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
	sealType := flag.String("seal-type", "dev", "seal type: dev, shamir, transit")

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

	// Cluster (Raft HA)
	clusterMode      := flag.Bool("cluster", false, "enable Raft HA backend (replaces bbolt with a replicated Raft log)")
	clusterNodeID    := flag.String("cluster-node-id", "", "unique node ID for this instance (defaults to hostname)")
	clusterBindAddr  := flag.String("cluster-bind-addr", "0.0.0.0:8201", "Raft RPC listen address")
	clusterAdvertise := flag.String("cluster-advertise", "", "Raft RPC advertise address (defaults to --cluster-bind-addr)")
	clusterDir       := flag.String("cluster-dir", "tuck-raft", "directory for Raft logs, snapshots, and FSM state")
	clusterBootstrap := flag.Bool("cluster-bootstrap", false, "bootstrap a new cluster (first node only; ignored on restarts)")
	clusterJoin      := flag.String("cluster-join", "", "leader HTTP address to join an existing cluster (e.g. https://leader:8200)")
	clusterTLSCert   := flag.String("cluster-tls-cert", "", "Raft mTLS: node certificate (PEM); enables TLS on Raft RPC together with --cluster-tls-key and --cluster-tls-ca")
	clusterTLSKey    := flag.String("cluster-tls-key", "", "Raft mTLS: node private key (PEM)")
	clusterTLSCA     := flag.String("cluster-tls-ca", "", "Raft mTLS: CA bundle (PEM) that signs all node certificates")
	clusterTLSName   := flag.String("cluster-tls-server-name", "", "Raft mTLS: expected peer certificate name (empty = verify chain only)")
	clusterLabelLeader := flag.Bool("cluster-label-leader", false, "Kubernetes: label this pod tuck.io/leader=true|false by Raft role so a Service can select the leader (needs POD_NAMESPACE, POD_NAME and RBAC pods/patch)")
	clusterPeers     := flag.String("cluster-peers", "", "comma-separated bootstrap peers: id=raftAddr,... (used with --cluster-bootstrap)")

	// Barrier cipher
	barrierCipher := flag.String("barrier-cipher", "", "at-rest cipher suite for a NEW store: aes-256-gcm (default) | gost-kuznyechik-mgm; ignored for an existing store")

	trustedProxies := flag.String("trusted-proxies", "", "comma-separated IPs/CIDRs of load balancers whose X-Forwarded-For is believed (client address in audit log, rate limits, AppRole CIDRs); empty = header ignored")

	requireMlock := flag.Bool("require-mlock", false, "refuse to start unless process memory can be locked (no swap); Linux only")

	// Audit log
	auditLog        := flag.String("audit-log", "", "path for the file audit log (empty = discard); rotated at --audit-max-size bytes")
	auditMaxSize    := flag.Int64("audit-max-size", audit.DefaultMaxSize, "max audit log file size in bytes before rotation")
	auditMaxBackups := flag.Int("audit-max-backups", audit.DefaultMaxBackups, "number of rotated audit log files to keep")

	// Rate limiting
	ipRateRPS   := flag.Float64("rate-ip-rps", 100, "per-IP request rate limit (requests/second); 0 = disabled")
	ipRateBurst := flag.Int("rate-ip-burst", 200, "per-IP burst size for rate limiting")
	tokRateRPS   := flag.Float64("rate-token-rps", 50, "per-token request rate limit (requests/second); 0 = disabled")
	tokRateBurst := flag.Int("rate-token-burst", 100, "per-token burst size for rate limiting")

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

	// Load config file and apply values for flags that were NOT set on the CLI.
	// Priority: CLI flag > env var > config file > built-in default.
	var k8sClusters []config.K8sCluster
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

		applyBool("cluster", cfg.Cluster.Enabled)
		apply("cluster-node-id", cfg.Cluster.NodeID)
		apply("cluster-bind-addr", cfg.Cluster.BindAddr)
		apply("cluster-advertise", cfg.Cluster.Advertise)
		apply("cluster-dir", cfg.Cluster.Dir)
		applyBool("cluster-bootstrap", cfg.Cluster.Bootstrap)
		apply("cluster-join", cfg.Cluster.Join)
		apply("cluster-peers", cfg.Cluster.Peers)
		applyBool("cluster-label-leader", cfg.Cluster.LabelLeader)
		apply("cluster-tls-cert", cfg.Cluster.TLSCert)
		apply("cluster-tls-key", cfg.Cluster.TLSKey)
		apply("cluster-tls-ca", cfg.Cluster.TLSCA)
		apply("cluster-tls-server-name", cfg.Cluster.TLSServerName)

		apply("k8s-api", cfg.Kubernetes.API)
		apply("k8s-token-file", cfg.Kubernetes.TokenFile)
		apply("k8s-ca-file", cfg.Kubernetes.CAFile)
		k8sClusters = cfg.Kubernetes.Clusters

		apply("otel-endpoint", cfg.Telemetry.OtelEndpoint)
		apply("barrier-cipher", cfg.Barrier.Cipher)
		applyBool("require-mlock", cfg.RequireMlock)
		apply("trusted-proxies", strings.Join(cfg.TrustedProxies, ","))

		log.Printf("tuck: loaded config from %q", resolvedCfgPath)
	}

	if err := lockMemory(); err != nil {
		if *requireMlock {
			log.Fatalf("tuck: --require-mlock set but %v", err)
		}
		log.Printf("tuck: WARNING: %v — the root key and DEK may be swapped to disk (set --require-mlock to make this fatal)", err)
	} else {
		log.Printf("tuck: process memory locked, core dumps disabled")
	}

	if err := clientip.SetTrusted(strings.Split(*trustedProxies, ",")); err != nil {
		log.Fatalf("tuck: --trusted-proxies: %v", err)
	}
	if strings.TrimSpace(*trustedProxies) != "" {
		log.Printf("tuck: trusting X-Forwarded-For from %s", *trustedProxies)
	}

	// --- Backend ---
	var (
		backend physical.Backend
		setRestoreHook func(func())
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
		var raftTLS *physraft.TLSConfig
		switch {
		case *clusterTLSCert != "" || *clusterTLSKey != "" || *clusterTLSCA != "":
			if *clusterTLSCert == "" || *clusterTLSKey == "" || *clusterTLSCA == "" {
				log.Fatalf("--cluster-tls-cert, --cluster-tls-key and --cluster-tls-ca must be provided together")
			}
			raftTLS = &physraft.TLSConfig{CertFile: *clusterTLSCert, KeyFile: *clusterTLSKey, CAFile: *clusterTLSCA, ServerName: *clusterTLSName}
			log.Printf("tuck: Raft RPC secured with mutual TLS")
		default:
			log.Printf("tuck: WARNING Raft RPC is plain TCP; set --cluster-tls-cert/-key/-ca on untrusted networks")
		}
		rb, raftErr := physraft.Open(physraft.Config{
			TLS:           raftTLS,
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
		defer func() { _ = rb.Close() }()

		// Auto-join an existing cluster via the leader's HTTP API.
		if *clusterJoin != "" {
			if joinErr := clusterJoinLeader(*clusterJoin, nodeID, *clusterBindAddr); joinErr != nil {
				log.Fatalf("cluster join: %v", joinErr)
			}
			log.Printf("tuck: joined cluster via %s", *clusterJoin)
		}

		backend = rb
		setRestoreHook = rb.OnRestore
		log.Printf("tuck: Raft HA backend (node=%s bind=%s dir=%s)", nodeID, *clusterBindAddr, *clusterDir)
	} else {
		bb, bbErr := physical.OpenBolt(*dataPath)
		if bbErr != nil {
			log.Fatalf("open backend: %v", bbErr)
		}
		defer func() { _ = bb.Close() }()
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

	default:
		log.Fatalf("unknown seal type %q; valid: dev, shamir, transit", *sealType)
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
	if setRestoreHook != nil {
		// A snapshot restore swaps the data under the barrier; seal so the node
		// re-reads its key ring when it is unsealed again.
		setRestoreHook(func() {
			log.Printf("tuck: cluster data restored from a snapshot; sealing — unseal with the key shares of the snapshot's cluster")
			c.Seal()
		})
	}
	if err := c.SetBarrierCipher(*barrierCipher); err != nil {
		log.Fatalf("barrier cipher: %v", err)
	}
	buildClusters := func(list []config.K8sCluster) (map[string]k8sauth.Reviewer, error) {
		out := make(map[string]k8sauth.Reviewer, len(list))
		for _, cl := range list {
			if cl.API == "" || cl.TokenFile == "" {
				return nil, fmt.Errorf("kubernetes cluster %q: api and token_file are required", cl.Name)
			}
			if _, dup := out[cl.Name]; dup {
				return nil, fmt.Errorf("duplicate kubernetes cluster %q", cl.Name)
			}
			r, err := k8sauth.NewClientFromFiles(cl.API, cl.TokenFile, cl.CAFile)
			if err != nil {
				return nil, fmt.Errorf("kubernetes cluster %q: %w", cl.Name, err)
			}
			out[cl.Name] = r
		}
		return out, nil
	}
	if initial, buildErr := buildClusters(k8sClusters); buildErr != nil {
		log.Fatalf("%v", buildErr)
	} else if _, _, replErr := c.ReplaceK8sClusters(initial); replErr != nil {
		log.Fatalf("kubernetes clusters: %v", replErr)
	}
	for _, cl := range k8sClusters {
		log.Printf("tuck: kubernetes cluster %q enabled (%s)", cl.Name, cl.API)
	}
	// reloadClusters re-reads kubernetes.clusters from the config file (SIGHUP
	// or POST /v1/auth/kubernetes/clusters/reload) and swaps the set in place.
	var reloadClusters func() ([]string, []string, error)
	var reloadMu sync.Mutex // SIGHUP and the API may reload at once: apply one file version at a time
	// onClustersChanged lets the API server follow the cluster list (set below, once it exists).
	var onClustersChanged func([]config.K8sCluster)
	if resolvedCfgPath != "" {
		reloadClusters = func() ([]string, []string, error) {
			reloadMu.Lock()
			defer reloadMu.Unlock()
			fresh, _, err := config.Load(resolvedCfgPath)
			if err != nil {
				return nil, nil, err
			}
			next, err := buildClusters(fresh.Kubernetes.Clusters)
			if err != nil {
				return nil, nil, err
			}
			added, removed, err := c.ReplaceK8sClusters(next)
			if err == nil {
				log.Printf("tuck: kubernetes clusters reloaded: added %v, removed %v", added, removed)
				if onClustersChanged != nil {
					onClustersChanged(fresh.Kubernetes.Clusters)
				}
			}
			return added, removed, err
		}
	}

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
	c.StartLDAPRecheck(ctx)

	if *clusterMode {
		log.Printf("tuck: cluster mode — followers wait for the leader to initialize the store")
	}
	startResult, startErr := c.StartWhenReady(ctx, time.Second)
	sealRetry := errors.Is(startErr, core.ErrSealUnavailable)
	if startErr != nil && !errors.Is(startErr, core.ErrNeedsUnseal) && !sealRetry {
		log.Fatalf("start core: %v", startErr)
	}
	if sealRetry {
		log.Printf("tuck: WARNING auto-unseal failed, staying SEALED and retrying every 5s: %v", startErr)
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
		reloader, certErr := tlsutil.NewReloader(*tlsCert, *tlsKey)
		if certErr != nil {
			log.Fatalf("load TLS cert/key: %v", certErr)
		}
		tlsCfg = tlsutil.ReloadingConfig(reloader)
		go reloader.Watch(ctx, 15*time.Second, log.Printf)
		log.Printf("tuck: TLS enabled (cert=%s, hot-reload every 15s)", *tlsCert)
	case *tlsCert != "" || *tlsKey != "":
		log.Fatalf("--tls-cert and --tls-key must be provided together")
	}

	// --- API server + rate limiters ---
	apiSrv := api.New(c)
	// TuckSecret view: the cluster Tuck runs in (if any) plus every trusted
	// cluster whose reviewer token may list TuckSecrets and namespaces.
	var localTuckSecrets api.TuckSecretSource
	applyTuckSecretSources := func(list []config.K8sCluster) {
		srcs := map[string]api.TuckSecretSource{}
		if localTuckSecrets != nil {
			srcs[""] = localTuckSecrets
		}
		for _, cl := range list {
			kc, err := operator.NewKubeClient(cl.API, cl.TokenFile, cl.CAFile)
			if err != nil {
				log.Printf("tuck: TuckSecret view of cluster %q disabled: %v", cl.Name, err)
				continue
			}
			srcs[cl.Name] = kc
		}
		apiSrv.SetTuckSecretSources(srcs)
	}
	onClustersChanged = applyTuckSecretSources
	// The --rate-* flags are the defaults; the runtime config (PUT /v1/sys/config,
	// persisted in the barrier) overrides them without a restart.
	apiSrv.SetRateDefaults(sysconfig.Config{
		IPRateLimitRPS: *ipRateRPS, IPRateLimitBurst: *ipRateBurst,
		TokenRateLimitRPS: *tokRateRPS, TokenRateLimitBurst: *tokRateBurst,
	})
	if reloadClusters != nil {
		apiSrv.SetClusterReloader(reloadClusters)
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		go func() {
			for range hup {
				if _, _, err := reloadClusters(); err != nil {
					log.Printf("tuck: SIGHUP: reloading kubernetes clusters failed (kept the current set): %v", err)
				}
			}
		}()
	}
	if *k8sAPI != "" {
		if _, statErr := os.Stat(*k8sTokenFile); statErr == nil {
			if kc, kcErr := operator.NewKubeClient(*k8sAPI, *k8sTokenFile, *k8sCaFile); kcErr == nil {
				localTuckSecrets = kc
			} else {
				log.Printf("tuck: TuckSecret view disabled: %v", kcErr)
			}
		}
	}
	applyTuckSecretSources(k8sClusters)
	// Restore barrier-persisted rate-limit config when the barrier is already
	// open (dev seal, auto-unseal). Shamir-sealed servers restore in postUnseal.
	if !errors.Is(startErr, core.ErrNeedsUnseal) && !sealRetry {
		if persisted, cfgErr := c.GetSysConfig(ctx); cfgErr == nil {
			apiSrv.ApplyRateConfig(persisted)
		}
	}
	if sealRetry {
		go func() {
			tick := time.NewTicker(5 * time.Second)
			defer tick.Stop()
			for attempt := 1; ; attempt++ {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
				}
				if ok, err := c.UnsealShard(ctx, ""); err != nil {
					if attempt%12 == 1 {
						log.Printf("tuck: auto-unseal still failing (node stays sealed): %v", err)
					}
				} else if ok {
					log.Printf("tuck: auto-unseal succeeded — node is unsealed")
					if persisted, cfgErr := c.GetSysConfig(ctx); cfgErr == nil {
						apiSrv.ApplyRateConfig(persisted)
					}
					return
				}
			}
		}()
	}
	apiSrv.StartLimiterCleanup(ctx)
	apiSrv.StartSysConfigSync(ctx, 10*time.Second)
	handler := apiSrv.Handler()

	// --- HTTP server with production timeouts ---
	srv := &http.Server{
		Handler:           telemetry.Middleware(handler),
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

	if *clusterLabelLeader {
		cb := c.ClusterBackend()
		if cb == nil {
			log.Fatalf("--cluster-label-leader requires --cluster")
		}
		pod := os.Getenv("POD_NAME")
		if pod == "" {
			pod = *clusterNodeID
		}
		labeler, lblErr := haregister.InCluster(os.Getenv("POD_NAMESPACE"), pod)
		if lblErr != nil {
			log.Fatalf("--cluster-label-leader: %v", lblErr)
		}
		go labeler.Run(ctx, cb.IsLeader, time.Second, 30*time.Second, log.Printf)
	}

	scheme := "http"
	if tlsCfg != nil {
		scheme = "https"
	}
	sealState := "unsealed"
	if errors.Is(startErr, core.ErrNeedsUnseal) || sealRetry {
		sealState = "sealed"
	}
	log.Printf(`{"level":"info","msg":"tuck started","version":%q,"commit":%q,"seal":%q,"addr":%q,"state":%q}`,
		version.Version, version.Commit, *sealType, *addr, sealState)
	if errors.Is(startErr, core.ErrNeedsUnseal) {
		log.Printf("tuck: SEALED — provide shards via POST %s://%s/v1/sys/unseal", scheme, *addr)
	} else if sealRetry {
		log.Printf("tuck: SEALED — waiting for the %s seal to become available", *sealType)
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
	defer func() { _ = resp.Body.Close() }()
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

