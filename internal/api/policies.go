package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/NAGenaev/tuck/internal/policy"
)

type policyRuleReq struct {
	Path         string   `json:"path"`
	Capabilities []string `json:"capabilities"`
}

type putPolicyReq struct {
	Rules []policyRuleReq `json:"rules"`
}

type policyRuleResp struct {
	Path         string   `json:"path"`
	Capabilities []string `json:"capabilities"`
}

type policyResp struct {
	Name  string           `json:"name"`
	Rules []policyRuleResp `json:"rules"`
}

func (s *Server) putPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/policy/"+name, policy.CapWrite); err != nil {
		writeErr(w, err)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	var req putPolicyReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	p := &policy.Policy{Name: name}
	for _, rule := range req.Rules {
		p.Rules = append(p.Rules, policy.Rule{
			Path:         rule.Path,
			Capabilities: parseCaps(rule.Capabilities),
		})
	}
	if err := s.core.PutPolicy(r.Context(), p); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/policy/"+name, policy.CapRead); err != nil {
		writeErr(w, err)
		return
	}
	p, err := s.core.GetPolicy(r.Context(), name)
	if err != nil {
		if errors.Is(err, policy.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "policy not found"})
			return
		}
		writeErr(w, err)
		return
	}
	resp := policyResp{Name: p.Name}
	for _, rule := range p.Rules {
		resp.Rules = append(resp.Rules, policyRuleResp{
			Path:         rule.Path,
			Capabilities: capsToStrings(rule.Capabilities),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) deletePolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.core.EnforceAccess(r.Context(), tokenFromCtx(r.Context()), "auth/policy/"+name, policy.CapDelete); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.core.DeletePolicy(r.Context(), name); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseCaps(ss []string) policy.Capability {
	var c policy.Capability
	for _, s := range ss {
		switch s {
		case "read":
			c |= policy.CapRead
		case "write":
			c |= policy.CapWrite
		case "delete":
			c |= policy.CapDelete
		case "list":
			c |= policy.CapList
		}
	}
	return c
}

func capsToStrings(c policy.Capability) []string {
	var ss []string
	if c&policy.CapRead != 0 {
		ss = append(ss, "read")
	}
	if c&policy.CapWrite != 0 {
		ss = append(ss, "write")
	}
	if c&policy.CapDelete != 0 {
		ss = append(ss, "delete")
	}
	if c&policy.CapList != 0 {
		ss = append(ss, "list")
	}
	return ss
}
