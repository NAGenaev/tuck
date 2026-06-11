// Command tuck-operator watches TuckSecret CRD resources and syncs their
// values from the Tuck server into native Kubernetes Secrets.
package main

import (
	"context"
	"flag"
	"log/slog"
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

	slog.Info("operator: starting", "tuck", *tuckAddr, "k8s", *k8sAPI, "namespace", *namespace)

	if err := ctrl.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("operator: fatal error", "err", err)
		os.Exit(1)
	}
	slog.Info("operator: shutdown complete")
}
