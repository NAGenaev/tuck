package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/NAGenaev/tuck/internal/namespace"
	"github.com/NAGenaev/tuck/internal/operator"
	"github.com/NAGenaev/tuck/internal/policy"
)

const maxTuckSecretRows = 5000

// clusterReadTimeout bounds reading one cluster for the TuckSecret page.
const clusterReadTimeout = 15 * time.Second

// TuckSecretSource reads TuckSecret resources and Namespace annotations from
// the Kubernetes API. *operator.KubeClient implements it.
type TuckSecretSource interface {
	List(ctx context.Context, namespace string) (*operator.TuckSecretList, error)
	ListNamespaceAnnotations(ctx context.Context) (map[string]map[string]string, error)
}

// SetTuckSecretSource enables GET /v1/sys/k8s/tucksecrets for the cluster the
// server itself runs in. Call before serving.
func (s *Server) SetTuckSecretSource(src TuckSecretSource) {
	s.SetTuckSecretSources(map[string]TuckSecretSource{"": src})
}

// SetTuckSecretSources sets the Kubernetes clusters the TuckSecret listing reads:
// the key is the cluster name as in kubernetes.clusters ("" = the cluster the
// server runs in). It replaces the previous set, so it can follow a reload of
// the trusted-cluster list.
func (s *Server) SetTuckSecretSources(srcs map[string]TuckSecretSource) {
	s.tuckSecretsMu.Lock()
	defer s.tuckSecretsMu.Unlock()
	s.tuckSecrets = srcs
}

func (s *Server) tuckSecretSources() map[string]TuckSecretSource {
	s.tuckSecretsMu.RLock()
	defer s.tuckSecretsMu.RUnlock()
	return s.tuckSecrets
}

type tuckSecretEntry struct {
	Key  string `json:"key"`
	Path string `json:"path"`
}

type tuckSecretRestartTarget struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type tuckSecretCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// tuckSecretView is the read-only projection shown in the UI. It never carries
// secret values: the API server only holds the references.
type tuckSecretView struct {
	Cluster         string                    `json:"cluster,omitempty"` // "" = the cluster Tuck runs in
	K8sNamespace    string                    `json:"k8s_namespace"`
	Name            string                    `json:"name"`
	TuckNamespace   string                    `json:"tuck_namespace"`
	SecretName      string                    `json:"secret_name"`
	Entries         []tuckSecretEntry         `json:"entries"`
	KVVersion       string                    `json:"kv_version"`
	RefreshInterval string                    `json:"refresh_interval"`
	DeletionPolicy  string                    `json:"deletion_policy"`
	RestartTargets  []tuckSecretRestartTarget `json:"restart_targets,omitempty"`
	State           string                    `json:"state"` // synced | error | pending | deleting
	LastSyncTime    *time.Time                `json:"last_sync_time,omitempty"`
	LastSyncError   string                    `json:"last_sync_error,omitempty"`
	SyncedRevision  string                    `json:"synced_revision,omitempty"`
	Conditions      []tuckSecretCondition     `json:"conditions,omitempty"`
}

func tuckSecretState(ts operator.TuckSecret) string {
	switch {
	case ts.Metadata.DeletionTimestamp != nil:
		return "deleting"
	case ts.Status.LastSyncError != "":
		return "error"
	case ts.Status.LastSyncTime.IsZero():
		return "pending"
	default:
		return "synced"
	}
}

func toTuckSecretView(ts operator.TuckSecret, tuckNS string) tuckSecretView {
	v := tuckSecretView{
		K8sNamespace:    ts.Metadata.Namespace,
		Name:            ts.Metadata.Name,
		TuckNamespace:   tuckNS,
		SecretName:      ts.Spec.SecretName,
		Entries:         []tuckSecretEntry{},
		KVVersion:       ts.Spec.KVVersion,
		RefreshInterval: ts.Spec.RefreshInterval,
		DeletionPolicy:  ts.Spec.DeletionPolicy,
		State:           tuckSecretState(ts),
		LastSyncError:   ts.Status.LastSyncError,
		SyncedRevision:  ts.Status.SyncedRevision,
	}
	if v.KVVersion == "" {
		v.KVVersion = "1"
	}
	if v.DeletionPolicy == "" {
		v.DeletionPolicy = "Retain"
	}
	if !ts.Status.LastSyncTime.IsZero() {
		t := ts.Status.LastSyncTime
		v.LastSyncTime = &t
	}
	if ts.Spec.TuckPath != "" {
		v.Entries = append(v.Entries, tuckSecretEntry{Key: ts.Spec.SecretKey, Path: ts.Spec.TuckPath})
	}
	for k, p := range ts.Spec.Data {
		v.Entries = append(v.Entries, tuckSecretEntry{Key: k, Path: p})
	}
	sort.Slice(v.Entries, func(i, j int) bool { return v.Entries[i].Key < v.Entries[j].Key })
	for _, rt := range ts.Spec.RestartTargets {
		v.RestartTargets = append(v.RestartTargets, tuckSecretRestartTarget{Kind: rt.Kind, Name: rt.Name})
	}
	for _, c := range ts.Status.Conditions {
		v.Conditions = append(v.Conditions, tuckSecretCondition{Type: c.Type, Status: c.Status, Reason: c.Reason, Message: c.Message})
	}
	return v
}

// boundTuckNamespace is the Tuck namespace a Kubernetes Namespace is bound to
// through the tuck.io/tuck-namespace annotation; "" means root / unbound.
func boundTuckNamespace(annotations map[string]string) string {
	v := strings.TrimSpace(annotations[operator.AnnotationTuckNamespace])
	if v == "" || namespace.ValidateName(v) != nil {
		return ""
	}
	return v
}

// clusterReport tells the UI how reading one cluster went, so that a cluster
// that is down or refuses the server does not hide the others.
type clusterReport struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Count int    `json:"count"`
	Code  string `json:"code,omitempty"` // k8s_error | k8s_forbidden
}

// GET /v1/sys/k8s/tucksecrets[?cluster=name]
//
// Lists the TuckSecret resources of every trusted Kubernetes cluster (or one,
// with ?cluster=) with their sync status. A token working inside a Tuck
// namespace sees only TuckSecrets of Kubernetes namespaces bound to it; a root
// token without X-Tuck-Namespace sees all. Clusters are read independently:
// the response is 200 as soon as one answered and lists the others with their
// error code under "clusters".
func (s *Server) listTuckSecrets(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "sys/k8s/tucksecrets", policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	srcs := s.tuckSecretSources()
	if only := r.URL.Query().Get("cluster"); only != "" {
		one, ok := srcs[only]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown cluster", "code": "k8s_unknown_cluster"})
			return
		}
		srcs = map[string]TuckSecretSource{only: one}
	}
	if len(srcs) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "kubernetes integration is not available on this server", "code": "k8s_unavailable"})
		return
	}

	names := make([]string, 0, len(srcs))
	for n := range srcs {
		names = append(names, n)
	}
	sort.Strings(names)

	want := nsFromCtx(r.Context())
	items := make([]tuckSecretView, 0)
	reports := make([]clusterReport, 0, len(names))
	truncated := false
	firstFailure := ""
	anySucceeded := false
	for _, name := range names {
		src := srcs[name]
		// One slow or hung cluster must not hold the whole page.
		cctx, cancel := context.WithTimeout(r.Context(), clusterReadTimeout)
		list, err := src.List(cctx, "")
		var annotations map[string]map[string]string
		if err == nil {
			annotations, err = src.ListNamespaceAnnotations(cctx)
		}
		cancel()
		if err != nil {
			log.Printf("api: list TuckSecrets (cluster %q): %v", name, err)
			code := "k8s_error"
			var se *operator.StatusError
			if errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusUnauthorized) {
				code = "k8s_forbidden"
			}
			if firstFailure == "" {
				firstFailure = code
			}
			reports = append(reports, clusterReport{Name: name, Code: code})
			continue
		}
		n := 0
		for _, ts := range list.Items {
			tuckNS := boundTuckNamespace(annotations[ts.Metadata.Namespace])
			if want != "" && tuckNS != want {
				continue
			}
			if len(items) >= maxTuckSecretRows {
				truncated = true
				break
			}
			v := toTuckSecretView(ts, tuckNS)
			v.Cluster = name
			items = append(items, v)
			n++
		}
		anySucceeded = true
		reports = append(reports, clusterReport{Name: name, OK: true, Count: n})
	}
	// Cluster names and their health are platform topology: only the root
	// namespace sees them; a team token gets just its own rows.
	if want != "" {
		reports = nil
	}
	if firstFailure != "" && !anySucceeded {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "cannot read TuckSecrets from the Kubernetes API", "code": firstFailure, "clusters": reports})
		return
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Cluster != items[j].Cluster {
			return items[i].Cluster < items[j].Cluster
		}
		if items[i].K8sNamespace != items[j].K8sNamespace {
			return items[i].K8sNamespace < items[j].K8sNamespace
		}
		return items[i].Name < items[j].Name
	})
	out := map[string]any{"items": items, "count": len(items), "truncated": truncated}
	if reports != nil {
		out["clusters"] = reports
	}
	writeJSON(w, http.StatusOK, out)
}
