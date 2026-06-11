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
	TTL      string   `json:"ttl"` // e.g. "24h", "" = never expires
}

type k8sRoleResp struct {
	Namespace      string   `json:"namespace"`
	ServiceAccount string   `json:"service_account"`
	Policies       []string `json:"policies"`
	TTL            string   `json:"ttl"` // "24h0m0s" or "" if no expiry
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
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "field 'token' is required"})
		return
	}
	tok, err := s.core.LoginK8s(r.Context(), req.Token)
	if err != nil {
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
	namespace, sa := r.PathValue("namespace"), r.PathValue("sa")
	enforcePath := "auth/k8s/role/" + namespace + "/" + sa
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
	role := &k8sauth.K8sRole{
		Namespace:      namespace,
		ServiceAccount: sa,
		Policies:       req.Policies,
		TTL:            ttl,
	}
	if err := s.core.CreateK8sRole(r.Context(), role); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getK8sRole(w http.ResponseWriter, r *http.Request) {
	namespace, sa := r.PathValue("namespace"), r.PathValue("sa")
	enforcePath := "auth/k8s/role/" + namespace + "/" + sa
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), enforcePath, policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	role, err := s.core.GetK8sRole(r.Context(), namespace, sa)
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
		Namespace:      role.Namespace,
		ServiceAccount: role.ServiceAccount,
		Policies:       role.Policies,
		TTL:            ttlStr,
	})
}

func (s *Server) deleteK8sRole(w http.ResponseWriter, r *http.Request) {
	namespace, sa := r.PathValue("namespace"), r.PathValue("sa")
	enforcePath := "auth/k8s/role/" + namespace + "/" + sa
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), enforcePath, policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.DeleteK8sRole(r.Context(), namespace, sa); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
