// Package api exposes Tuck's HTTP interface.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/NAGenaev/tuck/internal/audit"
	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/metrics"
	physraft "github.com/NAGenaev/tuck/internal/physical/raft"
	"github.com/NAGenaev/tuck/internal/ui"
)

const maxBodyBytes = 1 << 20 // 1 MiB

type contextKey int

const tokenCtxKey contextKey = iota

// Server adapts a core.Core to HTTP.
type Server struct {
	core  *core.Core
	audit *audit.Logger
}

// New returns an HTTP server over the given core with a no-op audit logger.
func New(c *core.Core) *Server { return NewWithAudit(c, audit.Nop()) }

// NewWithAudit returns an HTTP server with the given audit logger.
func NewWithAudit(c *core.Core, l *audit.Logger) *Server {
	return &Server{core: c, audit: l}
}

// Handler builds the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Embedded web dashboard — served at /ui/
	mux.Handle("/ui/", http.StripPrefix("/ui", ui.Handler()))

	mux.HandleFunc("GET /v1/health", s.health)

	// Sys endpoints: seal-status and unseal are unauthenticated; seal requires root.
	mux.HandleFunc("GET /v1/sys/seal-status", s.getSealStatus)
	mux.HandleFunc("GET /v1/sys/ready", s.getReady)
	mux.HandleFunc("POST /v1/sys/unseal", s.postUnseal)
	mux.HandleFunc("POST /v1/sys/seal", s.requireToken(s.postSeal))
	mux.HandleFunc("GET /v1/sys/snapshot", s.requireToken(s.getSnapshot))
	mux.HandleFunc("POST /v1/sys/rotate", s.requireToken(s.postRotate))

	mux.HandleFunc("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /v1/secret/{path...}", s.requireToken(s.getSecret))
	mux.HandleFunc("PUT /v1/secret/{path...}", s.requireToken(s.putSecret))
	mux.HandleFunc("DELETE /v1/secret/{path...}", s.requireToken(s.deleteSecret))
	mux.HandleFunc("LIST /v1/secret/{path...}", s.requireToken(s.listSecrets))

	mux.HandleFunc("POST /v1/auth/token", s.requireToken(s.createToken))
	mux.HandleFunc("GET /v1/auth/token/{id}", s.requireToken(s.lookupToken))
	mux.HandleFunc("DELETE /v1/auth/token/{id}", s.requireToken(s.revokeToken))
	mux.HandleFunc("POST /v1/auth/token/{id}/renew", s.requireToken(s.renewToken))
	mux.HandleFunc("LIST /v1/auth/token/", s.requireToken(s.listTokens))

	mux.HandleFunc("PUT /v1/policy/{name}", s.requireToken(s.putPolicy))
	mux.HandleFunc("GET /v1/policy/{name}", s.requireToken(s.getPolicy))
	mux.HandleFunc("DELETE /v1/policy/{name}", s.requireToken(s.deletePolicy))
	mux.HandleFunc("LIST /v1/policy/", s.requireToken(s.listPolicies))

	mux.HandleFunc("POST /v1/auth/kubernetes/login", s.loginK8s)
	mux.HandleFunc("PUT /v1/auth/kubernetes/role/{namespace}/{sa}", s.requireToken(s.putK8sRole))
	mux.HandleFunc("GET /v1/auth/kubernetes/role/{namespace}/{sa}", s.requireToken(s.getK8sRole))
	mux.HandleFunc("DELETE /v1/auth/kubernetes/role/{namespace}/{sa}", s.requireToken(s.deleteK8sRole))

	// JWT/OIDC auth — login is unauthenticated; config and role management require a token
	mux.HandleFunc("POST /v1/auth/jwt/login", s.loginJWT)
	mux.HandleFunc("GET /v1/auth/jwt/config", s.requireToken(s.getJWTConfig))
	mux.HandleFunc("PUT /v1/auth/jwt/config", s.requireToken(s.putJWTConfig))
	mux.HandleFunc("PUT /v1/auth/jwt/role/{name}", s.requireToken(s.putJWTRole))
	mux.HandleFunc("GET /v1/auth/jwt/role/{name}", s.requireToken(s.getJWTRole))
	mux.HandleFunc("DELETE /v1/auth/jwt/role/{name}", s.requireToken(s.deleteJWTRole))
	mux.HandleFunc("LIST /v1/auth/jwt/role/", s.requireToken(s.listJWTRoles))

	// SSH secrets engine — CA mode (signed SSH certificates)
	// CA public key is unauthenticated so hosts can fetch it for TrustedUserCAKeys.
	mux.HandleFunc("POST /v1/ssh/generate/ca", s.requireToken(s.sshGenerateCA))
	mux.HandleFunc("POST /v1/ssh/import/ca", s.requireToken(s.sshImportCA))
	mux.HandleFunc("GET /v1/ssh/ca/public-key", s.sshGetCAPublicKey)
	mux.HandleFunc("PUT /v1/ssh/roles/{name}", s.requireToken(s.sshPutRole))
	mux.HandleFunc("GET /v1/ssh/roles/{name}", s.requireToken(s.sshGetRole))
	mux.HandleFunc("DELETE /v1/ssh/roles/{name}", s.requireToken(s.sshDeleteRole))
	mux.HandleFunc("LIST /v1/ssh/roles/", s.requireToken(s.sshListRoles))
	mux.HandleFunc("POST /v1/ssh/sign/{role}", s.requireToken(s.sshSign))

	// Transit secrets engine — encryption-as-a-service
	mux.HandleFunc("POST /v1/transit/keys/{name}", s.requireToken(s.transitCreateKey))
	mux.HandleFunc("GET /v1/transit/keys/{name}", s.requireToken(s.transitGetKey))
	mux.HandleFunc("DELETE /v1/transit/keys/{name}", s.requireToken(s.transitDeleteKey))
	mux.HandleFunc("LIST /v1/transit/keys/", s.requireToken(s.transitListKeys))
	mux.HandleFunc("POST /v1/transit/keys/{name}/rotate", s.requireToken(s.transitRotate))
	mux.HandleFunc("POST /v1/transit/keys/{name}/config", s.requireToken(s.transitUpdateKey))
	mux.HandleFunc("POST /v1/transit/encrypt/{name}", s.requireToken(s.transitEncrypt))
	mux.HandleFunc("POST /v1/transit/decrypt/{name}", s.requireToken(s.transitDecrypt))
	mux.HandleFunc("POST /v1/transit/rewrap/{name}", s.requireToken(s.transitRewrap))
	mux.HandleFunc("POST /v1/transit/sign/{name}", s.requireToken(s.transitSign))
	mux.HandleFunc("POST /v1/transit/verify/{name}", s.requireToken(s.transitVerify))
	mux.HandleFunc("POST /v1/transit/hmac/{name}", s.requireToken(s.transitHMAC))

	// AppRole auth — login is unauthenticated; role/secret-id management requires a token
	mux.HandleFunc("POST /v1/auth/approle/login", s.loginAppRole)
	mux.HandleFunc("PUT /v1/auth/approle/role/{name}", s.requireToken(s.putAppRole))
	mux.HandleFunc("GET /v1/auth/approle/role/{name}", s.requireToken(s.getAppRole))
	mux.HandleFunc("DELETE /v1/auth/approle/role/{name}", s.requireToken(s.deleteAppRole))
	mux.HandleFunc("LIST /v1/auth/approle/role/", s.requireToken(s.listAppRoles))
	mux.HandleFunc("POST /v1/auth/approle/role/{name}/secret-id", s.requireToken(s.generateSecretID))
	mux.HandleFunc("GET /v1/auth/approle/role/{name}/secret-id/{id}", s.requireToken(s.lookupSecretID))
	mux.HandleFunc("DELETE /v1/auth/approle/role/{name}/secret-id/{id}", s.requireToken(s.destroySecretID))

	// Dynamic secrets: database engine
	mux.HandleFunc("PUT /v1/database/config/{name}", s.requireToken(s.putDBConfig))
	mux.HandleFunc("GET /v1/database/config/{name}", s.requireToken(s.getDBConfig))
	mux.HandleFunc("DELETE /v1/database/config/{name}", s.requireToken(s.deleteDBConfig))
	mux.HandleFunc("LIST /v1/database/config/", s.requireToken(s.listDBConfigs))
	mux.HandleFunc("PUT /v1/database/role/{name}", s.requireToken(s.putDBRole))
	mux.HandleFunc("GET /v1/database/role/{name}", s.requireToken(s.getDBRole))
	mux.HandleFunc("DELETE /v1/database/role/{name}", s.requireToken(s.deleteDBRole))
	mux.HandleFunc("LIST /v1/database/role/", s.requireToken(s.listDBRoles))
	mux.HandleFunc("POST /v1/database/creds/{role}", s.requireToken(s.generateDBCreds))
	mux.HandleFunc("GET /v1/database/lease/{id}", s.requireToken(s.getDBLease))
	mux.HandleFunc("DELETE /v1/database/lease/{id}", s.requireToken(s.revokeDBLease))
	mux.HandleFunc("LIST /v1/database/lease/", s.requireToken(s.listDBLeases))

	// PKI secrets engine
	// CA setup and CRL are unauthenticated so clients can verify certs without a token.
	mux.HandleFunc("POST /v1/pki/generate/root", s.requireToken(s.pkiGenerateRoot))
	mux.HandleFunc("POST /v1/pki/import/ca", s.requireToken(s.pkiImportCA))
	mux.HandleFunc("GET /v1/pki/ca/pem", s.pkiGetCACert)
	mux.HandleFunc("GET /v1/pki/crl/pem", s.pkiGetCRL)
	mux.HandleFunc("PUT /v1/pki/roles/{name}", s.requireToken(s.pkiPutRole))
	mux.HandleFunc("GET /v1/pki/roles/{name}", s.requireToken(s.pkiGetRole))
	mux.HandleFunc("DELETE /v1/pki/roles/{name}", s.requireToken(s.pkiDeleteRole))
	mux.HandleFunc("LIST /v1/pki/roles/", s.requireToken(s.pkiListRoles))
	mux.HandleFunc("POST /v1/pki/issue/{role}", s.requireToken(s.pkiIssueCert))
	mux.HandleFunc("POST /v1/pki/revoke/{serial}", s.requireToken(s.pkiRevokeCert))
	mux.HandleFunc("GET /v1/pki/certs/{serial}", s.requireToken(s.pkiGetCert))
	mux.HandleFunc("LIST /v1/pki/certs/", s.requireToken(s.pkiListCerts))

	// KV v2 — versioned secrets
	mux.HandleFunc("PUT /v2/secret/{path...}", s.requireToken(s.v2WriteSecret))
	mux.HandleFunc("GET /v2/secret/{path...}", s.requireToken(s.v2ReadSecret))
	mux.HandleFunc("DELETE /v2/secret/{path...}", s.requireToken(s.v2DeleteSecret))
	mux.HandleFunc("LIST /v2/secret/{path...}", s.requireToken(s.v2ListSecrets))
	mux.HandleFunc("POST /v2/secret/undelete/{path...}", s.requireToken(s.v2Undelete))
	mux.HandleFunc("POST /v2/secret/destroy/{path...}", s.requireToken(s.v2Destroy))
	mux.HandleFunc("GET /v2/secret/metadata/{path...}", s.requireToken(s.v2GetMeta))
	mux.HandleFunc("PUT /v2/secret/metadata/{path...}", s.requireToken(s.v2UpdateMeta))
	mux.HandleFunc("DELETE /v2/secret/metadata/{path...}", s.requireToken(s.v2DeleteMeta))
	mux.HandleFunc("LIST /v2/secret/metadata/{path...}", s.requireToken(s.v2ListMeta))

	// OpenAPI spec
	mux.HandleFunc("GET /openapi.json", serveOpenAPI)

	// Cluster management (Raft HA mode only)
	mux.HandleFunc("GET /v1/sys/cluster", s.requireToken(s.getClusterStatus))
	mux.HandleFunc("POST /v1/sys/cluster/join", s.requireToken(s.postClusterJoin))
	mux.HandleFunc("DELETE /v1/sys/cluster/node/{id}", s.requireToken(s.deleteClusterNode))

	return audit.Middleware(s.audit, mux)
}

// requireToken extracts and validates X-Tuck-Token, then stores the token ID
// in context. Returns 401 on missing/invalid token, 503 if the barrier is sealed.
func (s *Server) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Tuck-Token")
		if id == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing X-Tuck-Token header"})
			return
		}
		if _, err := s.core.Authenticate(r.Context(), id); err != nil {
			if errors.Is(err, barrier.ErrSealed) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sealed"})
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), tokenCtxKey, id)))
	}
}

func tokenFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(tokenCtxKey).(string)
	return id
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sealed": s.core.Sealed()})
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, barrier.ErrSealed):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sealed"})
	case errors.Is(err, core.ErrTokenInvalid):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
	case errors.Is(err, core.ErrUnauthorized):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
	case errors.Is(err, physraft.ErrNotLeader):
		// Return 503 so callers know to retry against the leader.
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "not leader — write to the cluster leader"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
