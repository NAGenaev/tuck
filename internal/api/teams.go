package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/namespace"
	"github.com/NAGenaev/tuck/internal/policy"
)

// POST /v1/sys/teams
//
// Team onboarding: creates the namespace, its policies, the LDAP roles that
// map domain groups onto them and, optionally, Kubernetes ServiceAccount roles
// in one all-or-nothing step. With "dry_run": true nothing is changed and the
// response lists what would be created and which objects already exist.
//
// Only a caller in the root namespace may use it: it writes global auth roles.
func (s *Server) onboardTeam(w http.ResponseWriter, r *http.Request) {
	if nsFromCtx(r.Context()) != "" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}
	var body struct {
		core.TeamSpec
		TTL    string `json:"ttl"`
		DryRun bool   `json:"dry_run"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	spec := body.TeamSpec
	if body.TTL != "" {
		d, err := time.ParseDuration(body.TTL)
		if err != nil || d < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl"})
			return
		}
		spec.TTL = d
	}

	// The caller needs every right the individual steps would need.
	tok := tokenFromCtx(r.Context())
	paths := []string{"sys/namespaces"}
	for access := range spec.Groups {
		paths = append(paths, "auth/ldap/role/"+namespaceRoleName(spec.Name, access))
	}
	for _, b := range spec.K8s {
		paths = append(paths, k8sRolePolicyPath(b.Cluster, b.Namespace, b.ServiceAccount))
	}
	for _, p := range paths {
		if err := s.core.EnforceAccess(r.Context(), tok, p, policy.CapWrite); err != nil {
			writeErr(w, err)
			return
		}
	}

	var (
		res *core.OnboardResult
		err error
	)
	if body.DryRun {
		res, err = s.core.PlanTeam(r.Context(), spec)
	} else {
		res, err = s.core.OnboardTeam(r.Context(), spec)
	}
	if err != nil {
		switch {
		case errors.Is(err, core.ErrOnboardInvalid), errors.Is(err, namespace.ErrInvalidName), errors.Is(err, namespace.ErrInvalidMetadata):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, core.ErrOnboardConflict):
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "result": res})
		default:
			writeErr(w, err)
		}
		return
	}
	status := http.StatusCreated
	if body.DryRun {
		status = http.StatusOK
	}
	writeJSON(w, status, res)
}

// namespaceRoleName mirrors the name the onboarding gives an LDAP role.
func namespaceRoleName(ns, access string) string {
	return core.TeamLDAPRoleName(ns, access)
}
