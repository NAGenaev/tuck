package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/core"
	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/policy"
)

type putK8sRoleReq struct {
	Policies []string `json:"policies"`
	TTL      string   `json:"ttl"` // e.g. "24h"; "" falls back to a 1h token TTL, not an eternal one
	// TuckNamespace binds issued tokens to a Tuck namespace; "" = root.
	TuckNamespace string `json:"tuck_namespace"`
}

type k8sRoleResp struct {
	Cluster        string   `json:"cluster,omitempty"`
	Namespace      string   `json:"namespace"`
	ServiceAccount string   `json:"service_account"`
	Policies       []string `json:"policies"`
	TuckNamespace  string   `json:"tuck_namespace,omitempty"`
	TTL            string   `json:"ttl"` // "24h0m0s", or "" if the role itself has no TTL configured (logins still get the 1h default)
}

// loginK8s exchanges a Kubernetes ServiceAccount JWT for a Tuck token.
// Does not require X-Tuck-Token — this IS the auth endpoint.
func (s *Server) loginK8s(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req struct {
		Token string `json:"token"`
		Cluster string `json:"cluster"`
	}
	if !decodeJSON(w, body, &req) {
		return
	}
	if req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "field 'token' is required"})
		return
	}
	tok, err := s.core.LoginK8sCluster(r.Context(), req.Cluster, req.Token)
	if err != nil {
		if errors.Is(err, core.ErrK8sUnknownCluster) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown kubernetes cluster"})
			return
		}
		if errors.Is(err, core.ErrK8sAuthDisabled) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "kubernetes auth is not configured"})
			return
		}
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": tok.ID})
}

func (s *Server) putK8sRole(w http.ResponseWriter, r *http.Request) {
	cluster, namespace, sa := r.PathValue("cluster"), r.PathValue("namespace"), r.PathValue("sa")
	if cluster != "" && !k8sauth.ValidClusterName(cluster) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cluster name"})
		return
	}
	enforcePath := k8sRolePolicyPath(cluster, namespace, sa)
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), enforcePath, policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req putK8sRoleReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	var ttl time.Duration
	if req.TTL != "" {
		if ttl, err = time.ParseDuration(req.TTL); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl: " + err.Error()})
			return
		}
	}
	if req.Policies == nil {
		req.Policies = []string{}
	}
	tuckNS, ok := s.roleNamespace(w, r, req.TuckNamespace)
	if !ok {
		return
	}
	role := &k8sauth.K8sRole{
		Cluster:        cluster,
		Namespace:      namespace,
		ServiceAccount: sa,
		Policies:       req.Policies,
		TTL:            ttl,
		TuckNamespace:  tuckNS,
	}
	if err := s.core.CreateK8sRole(r.Context(), role); err != nil {
		if errors.Is(err, core.ErrK8sUnknownCluster) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown kubernetes cluster"})
			return
		}
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getK8sRole(w http.ResponseWriter, r *http.Request) {
	cluster, namespace, sa := r.PathValue("cluster"), r.PathValue("namespace"), r.PathValue("sa")
	if cluster != "" && !k8sauth.ValidClusterName(cluster) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cluster name"})
		return
	}
	enforcePath := k8sRolePolicyPath(cluster, namespace, sa)
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), enforcePath, policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	role, err := s.core.GetK8sRole(r.Context(), cluster, namespace, sa)
	if err != nil {
		if errors.Is(err, k8sauth.ErrRoleNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "k8s role not found"})
			return
		}
		writeErr(w, err)
		return
	}
	ttlStr := ""
	if role.TTL > 0 {
		ttlStr = role.TTL.String()
	}
	writeJSON(w, http.StatusOK, k8sRoleResp{
		Cluster:        role.Cluster,
		Namespace:      role.Namespace,
		ServiceAccount: role.ServiceAccount,
		Policies:       role.Policies,
		TuckNamespace:  role.TuckNamespace,
		TTL:            ttlStr,
	})
}

func (s *Server) deleteK8sRole(w http.ResponseWriter, r *http.Request) {
	cluster, namespace, sa := r.PathValue("cluster"), r.PathValue("namespace"), r.PathValue("sa")
	if cluster != "" && !k8sauth.ValidClusterName(cluster) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cluster name"})
		return
	}
	enforcePath := k8sRolePolicyPath(cluster, namespace, sa)
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), enforcePath, policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.DeleteK8sRole(r.Context(), cluster, namespace, sa); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// k8sRolePolicyPath is the ACL path guarding a role. Default-cluster roles keep
// the legacy path; named clusters get their own, so policies can be scoped per
// cluster.
func k8sRolePolicyPath(cluster, namespace, sa string) string {
	if cluster == "" {
		return "auth/k8s/role/" + namespace + "/" + sa
	}
	return "auth/k8s/cluster/" + cluster + "/role/" + namespace + "/" + sa
}

// listK8sClusters returns the names of the extra trusted clusters.
func (s *Server) listK8sClusters(w http.ResponseWriter, r *http.Request) {
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/k8s/cluster", policy.CapList); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"clusters": s.core.K8sClusters()})
}
