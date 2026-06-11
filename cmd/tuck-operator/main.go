// Command tuck-operator watches TuckSecret CRD resources and syncs their
// values from the Tuck server into native Kubernetes Secrets.
package main

import (
	"context"
	"flag"
	"log"
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

	// Validate required files exist before we start connecting.
	if _, err := os.Stat(*saTokenFile); err != nil {
		log.Fatalf("operator: SA token file %q not found: %v", *saTokenFile, err)
	}

	kubeClient, err := operator.NewKubeClient(*k8sAPI, *saTokenFile, *k8sCAFile)
	if err != nil {
		log.Fatalf("operator: build kube client: %v", err)
	}

	tuckClient := operator.NewTuckClient(*tuckAddr, *saTokenFile)

	ctrl := operator.New(kubeClient, tuckClient, *namespace)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	log.Printf("operator: starting (tuck=%s, k8s=%s, namespace=%q)",
		*tuckAddr, *k8sAPI, *namespace)

	if err := ctrl.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatalf("operator: fatal error: %v", err)
	}
	log.Println("operator: shutdown complete")
}
