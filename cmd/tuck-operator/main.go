// Command tuck-operator watches TuckSecret CRD resources and syncs their
// values from the Tuck server into native Kubernetes Secrets.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/NAGenaev/tuck/internal/operator"
)

func main() {
	tuckAddr := flag.String("tuck-addr", "https://tuck.tuck.svc:8200",
		"address of the Tuck server")
	namespace := flag.String("namespace", "",
		"namespace to watch (empty = all namespaces)")
	saTokenFile := flag.String("sa-token-file",
		"/var/run/secrets/kubernetes.io/serviceaccount/token",
		"path to this pod's Kubernetes ServiceAccount token (used for both\n"+
			"Kubernetes API auth and Tuck /v1/auth/kubernetes/login)")
	k8sCAFile := flag.String("k8s-ca-file",
		"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		"path to the Kubernetes cluster CA certificate")
	k8sAPI := flag.String("k8s-api", "https://kubernetes.default.svc",
		"Kubernetes API server base URL")

	// Leader election flags (OP-1)
	leaderElect := flag.Bool("leader-elect", false,
		"enable Lease-based leader election for HA (multiple replicas)")
	leaderNamespace := flag.String("leader-namespace", "tuck-system",
		"namespace in which to create the leader election Lease")
	leaderID := flag.String("leader-id", "",
		"unique identity for this replica in leader election (default: hostname)")
	healthAddr := flag.String("health-addr", ":8081",
		"address to serve the /healthz liveness endpoint on")

	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// Validate required files exist before we start connecting.
	if _, err := os.Stat(*saTokenFile); err != nil {
		slog.Error("operator: SA token file not found", "file", *saTokenFile, "err", err)
		os.Exit(1)
	}

	kubeClient, err := operator.NewKubeClient(*k8sAPI, *saTokenFile, *k8sCAFile)
	if err != nil {
		slog.Error("operator: build kube client", "err", err)
		os.Exit(1)
	}

	tuckClient := operator.NewTuckClient(*tuckAddr, *saTokenFile)
	ctrl := operator.New(kubeClient, tuckClient, *namespace)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	slog.Info("operator: starting", "tuck", *tuckAddr, "k8s", *k8sAPI, "namespace", *namespace, "leaderElect", *leaderElect)

	startHealthServer(ctx, *healthAddr)

	if *leaderElect {
		elector, err := operator.NewLeaderElector(kubeClient, operator.LeaderConfig{
			LeaseName:      "tuck-operator-leader",
			LeaseNamespace: *leaderNamespace,
			HolderIdentity: *leaderID,
		})
		if err != nil {
			slog.Error("operator: create leader elector", "err", err)
			os.Exit(1)
		}
		if err := elector.Run(ctx, func(leadCtx context.Context) {
			slog.Info("operator: became leader — starting controller")
			if err := ctrl.Run(leadCtx); err != nil && leadCtx.Err() == nil {
				slog.Error("operator: controller error", "err", err)
			}
		}); err != nil && ctx.Err() == nil {
			slog.Error("operator: leader election error", "err", err)
			os.Exit(1)
		}
	} else {
		if err := ctrl.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("operator: fatal error", "err", err)
			os.Exit(1)
		}
	}
	slog.Info("operator: shutdown complete")
}

// startHealthServer serves a liveness endpoint on addr. The Helm chart's
// operator deployment probes http://:8081/healthz; without a listener there,
// kubelet kills the container on every liveness check and the pod
// crash-loops forever. Liveness only needs "is the process alive and
// serving", so this always returns 200 once the goroutine is running.
func startHealthServer(ctx context.Context, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("operator: health server failed", "err", err)
			os.Exit(1)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
}
