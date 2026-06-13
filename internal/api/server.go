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
	"github.com/NAGenaev/tuck/internal/version"
)

const maxBodyBytes = 1 << 20 // 1 MiB

type contextKey int

const (
	tokenCtxKey contextKey = iota
	nsCtxKey
)

// Server adapts a core.Core to HTTP.
type Server struct {
	core  *core.Core
	audit audit.Loggable
}

// New returns an HTTP server wired to the core's audit Dispatcher.
func New(c *core.Core) *Server { return &Server{core: c, audit: c.Dispatcher()} }

// NewWithAudit returns an HTTP server with a custom audit sink (used in tests).
func NewWithAudit(c *core.Core, l audit.Loggable) *Server {
	return &Server{core: c, audit: l}
}

// Handler builds the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Embedded web dashboard — served at /ui/
	mux.Handle("/ui/", http.StripPrefix("/ui", ui.Handler()))

	mux.HandleFunc("GET /v1/health", s.health)

	// Sys endpoints: seal-status and unseal are unauthenticated; seal requires root.
	// Audit sink management
	mux.HandleFunc("PUT /v1/sys/audit/webhook/{name}", s.requireToken(s.putAuditWebhook))
	mux.HandleFunc("DELETE /v1/sys/audit/{name}", s.requireToken(s.deleteAuditSink))
	mux.HandleFunc("LIST /v1/sys/audit/", s.requireToken(s.listAuditSinks))

	// Namespace management
	mux.HandleFunc("POST /v1/sys/namespaces", s.requireToken(s.createNamespace))
	mux.HandleFunc("GET /v1/sys/namespaces/{name}", s.requireToken(s.getNamespace))
	mux.HandleFunc("DELETE /v1/sys/namespaces/{name}", s.requireToken(s.deleteNamespace))
	mux.HandleFunc("LIST /v1/sys/namespaces/", s.requireToken(s.listNamespaces))

	// Runtime configuration
	mux.HandleFunc("GET /v1/sys/config", s.requireToken(s.getSysConfig))
	mux.HandleFunc("PUT /v1/sys/config", s.requireToken(s.putSysConfig))

	// Unified lease management — GET/DELETE /v1/sys/leases/<backend>/<id>
	mux.HandleFunc("GET /v1/sys/leases/{id...}", s.requireToken(s.getLease))
	mux.HandleFunc("DELETE /v1/sys/leases/{id...}", s.requireToken(s.revokeLease))
	mux.HandleFunc("LIST /v1/sys/leases/", s.requireToken(s.listLeases))

	// Mount table — list, create, delete secret engine mounts
	mux.HandleFunc("GET /v1/sys/mounts", s.requireToken(s.listMounts))
	mux.HandleFunc("POST /v1/sys/mounts/{path...}", s.requireToken(s.createMount))
	mux.HandleFunc("DELETE /v1/sys/mounts/{path...}", s.requireToken(s.deleteMount))

	// Per-mount tuning (GET/POST /v1/sys/mounts-tune/{path...})
	mux.HandleFunc("GET /v1/sys/mounts-tune/{path...}", s.requireToken(s.getMountConfig))
	mux.HandleFunc("POST /v1/sys/mounts-tune/{path...}", s.requireToken(s.putMountConfig))

	// Plugin catalog — register, inspect, delete external plugins
	mux.HandleFunc("GET /v1/sys/plugins/catalog/{type}/{name}", s.requireToken(s.getPlugin))
	mux.HandleFunc("POST /v1/sys/plugins/catalog/{type}/{name}", s.requireToken(s.registerPlugin))
	mux.HandleFunc("DELETE /v1/sys/plugins/catalog/{type}/{name}", s.requireToken(s.deletePlugin))
	mux.HandleFunc("LIST /v1/sys/plugins/catalog/{type}/", s.requireToken(s.listPlugins))
	mux.HandleFunc("LIST /v1/sys/plugins/catalog/", s.requireToken(s.listPlugins))

	// Replication — WAL and mode management
	mux.HandleFunc("GET /v1/sys/replication/status", s.requireToken(s.replicationStatus))
	mux.HandleFunc("POST /v1/sys/replication/primary/enable", s.requireToken(s.enablePrimary))
	mux.HandleFunc("POST /v1/sys/replication/secondary/enable", s.requireToken(s.enableSecondary))
	mux.HandleFunc("POST /v1/sys/replication/disable", s.requireToken(s.disableReplication))
	mux.HandleFunc("GET /v1/sys/replication/wal", s.requireToken(s.walEntries))
	mux.HandleFunc("POST /v1/sys/replication/wal/trim", s.requireToken(s.trimWAL))

	mux.HandleFunc("GET /v1/sys/seal-status", s.getSealStatus)
	mux.HandleFunc("GET /v1/sys/ready", s.getReady)
	mux.HandleFunc("POST /v1/sys/unseal", s.postUnseal)
	mux.HandleFunc("POST /v1/sys/seal", s.requireToken(s.postSeal))
	mux.HandleFunc("GET /v1/sys/snapshot", s.requireToken(s.getSnapshot))
	mux.HandleFunc("POST /v1/sys/rotate", s.requireToken(s.postRotate))

	// Cubbyhole — per-token private storage, purged on token revocation/expiry
	mux.HandleFunc("GET /v1/cubbyhole/{path...}", s.requireToken(s.cubbyholeGet))
	mux.HandleFunc("PUT /v1/cubbyhole/{path...}", s.requireToken(s.cubbyholePut))
	mux.HandleFunc("DELETE /v1/cubbyhole/{path...}", s.requireToken(s.cubbyholeDelete))
	mux.HandleFunc("LIST /v1/cubbyhole/{path...}", s.requireToken(s.cubbyholeList))

	// Response wrapping — single-use tokens for secure secret delivery
	mux.HandleFunc("POST /v1/sys/wrapping/wrap", s.requireToken(s.wrapPayload))
	mux.HandleFunc("POST /v1/sys/wrapping/unwrap", s.requireToken(s.unwrapPayload))
	mux.HandleFunc("POST /v1/sys/wrapping/lookup", s.requireToken(s.lookupWrappingToken))
	mux.HandleFunc("DELETE /v1/sys/wrapping/revoke", s.requireToken(s.revokeWrappingToken))

	mux.HandleFunc("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /v1/secret/{path...}", s.requireToken(s.getSecret))
	mux.HandleFunc("PUT /v1/secret/{path...}", s.requireToken(s.putSecret))
	mux.HandleFunc("DELETE /v1/secret/{path...}", s.requireToken(s.deleteSecret))
	mux.HandleFunc("LIST /v1/secret/{path...}", s.requireToken(s.listSecrets))

	// Token roles
	mux.HandleFunc("PUT /v1/auth/token/roles/{name}", s.requireToken(s.putTokenRole))
	mux.HandleFunc("GET /v1/auth/token/roles/{name}", s.requireToken(s.getTokenRole))
	mux.HandleFunc("DELETE /v1/auth/token/roles/{name}", s.requireToken(s.deleteTokenRole))
	mux.HandleFunc("LIST /v1/auth/token/roles/", s.requireToken(s.listTokenRoles))
	mux.HandleFunc("POST /v1/auth/token/roles/{role}/create", s.requireToken(s.createTokenFromRole))

	mux.HandleFunc("POST /v1/auth/token", s.requireToken(s.createToken))
	mux.HandleFunc("GET /v1/auth/token/{id}", s.requireToken(s.lookupToken))
	mux.HandleFunc("DELETE /v1/auth/token/{id}", s.requireToken(s.revokeToken))
	mux.HandleFunc("POST /v1/auth/token/{id}/renew", s.requireToken(s.renewToken))
	mux.HandleFunc("LIST /v1/auth/token/", s.requireToken(s.listTokens))
	mux.HandleFunc("POST /v1/auth/token/lookup-accessor", s.requireToken(s.lookupByAccessor))
	mux.HandleFunc("DELETE /v1/auth/token/revoke-accessor", s.requireToken(s.revokeByAccessor))
	mux.HandleFunc("GET /v1/auth/token/lookup-self", s.requireToken(s.lookupSelf))
	mux.HandleFunc("POST /v1/auth/token/renew-self", s.requireToken(s.renewSelf))

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

	// GitHub Actions OIDC auth — login is unauthenticated; role management requires a token
	mux.HandleFunc("POST /v1/auth/github/login", s.loginGitHub)
	mux.HandleFunc("PUT /v1/auth/github/role/{name}", s.requireToken(s.putGitHubRole))
	mux.HandleFunc("GET /v1/auth/github/role/{name}", s.requireToken(s.getGitHubRole))
	mux.HandleFunc("DELETE /v1/auth/github/role/{name}", s.requireToken(s.deleteGitHubRole))
	mux.HandleFunc("LIST /v1/auth/github/role/", s.requireToken(s.listGitHubRoles))

	// TOTP secrets engine — store and validate time-based OTP codes
	mux.HandleFunc("POST /v1/totp/keys/{name}", s.requireToken(s.totpCreateKey))
	mux.HandleFunc("GET /v1/totp/keys/{name}", s.requireToken(s.totpGetKey))
	mux.HandleFunc("DELETE /v1/totp/keys/{name}", s.requireToken(s.totpDeleteKey))
	mux.HandleFunc("LIST /v1/totp/keys/", s.requireToken(s.totpListKeys))
	mux.HandleFunc("GET /v1/totp/code/{name}", s.requireToken(s.totpGenerateCode))
	mux.HandleFunc("POST /v1/totp/code/{name}", s.requireToken(s.totpValidateCode))

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

	// Azure dynamic secrets engine — generate Azure AD client secrets for app registrations
	mux.HandleFunc("PUT /v1/azure/config", s.requireToken(s.putAzureConfig))
	mux.HandleFunc("GET /v1/azure/config", s.requireToken(s.getAzureConfig))
	mux.HandleFunc("DELETE /v1/azure/config", s.requireToken(s.deleteAzureConfig))
	mux.HandleFunc("PUT /v1/azure/roles/{name}", s.requireToken(s.putAzureRole))
	mux.HandleFunc("GET /v1/azure/roles/{name}", s.requireToken(s.getAzureRole))
	mux.HandleFunc("DELETE /v1/azure/roles/{name}", s.requireToken(s.deleteAzureRole))
	mux.HandleFunc("LIST /v1/azure/roles/", s.requireToken(s.listAzureRoles))
	mux.HandleFunc("POST /v1/azure/creds/{role}", s.requireToken(s.generateAzureCreds))
	mux.HandleFunc("GET /v1/azure/lease/{id}", s.requireToken(s.getAzureLease))
	mux.HandleFunc("DELETE /v1/azure/lease/{id}", s.requireToken(s.revokeAzureLease))
	mux.HandleFunc("LIST /v1/azure/lease/", s.requireToken(s.listAzureLeases))

	// GCP dynamic secrets engine — generate service account keys or OAuth2 access tokens
	mux.HandleFunc("PUT /v1/gcp/config", s.requireToken(s.putGCPConfig))
	mux.HandleFunc("GET /v1/gcp/config", s.requireToken(s.getGCPConfig))
	mux.HandleFunc("DELETE /v1/gcp/config", s.requireToken(s.deleteGCPConfig))
	mux.HandleFunc("PUT /v1/gcp/roles/{name}", s.requireToken(s.putGCPRole))
	mux.HandleFunc("GET /v1/gcp/roles/{name}", s.requireToken(s.getGCPRole))
	mux.HandleFunc("DELETE /v1/gcp/roles/{name}", s.requireToken(s.deleteGCPRole))
	mux.HandleFunc("LIST /v1/gcp/roles/", s.requireToken(s.listGCPRoles))
	mux.HandleFunc("POST /v1/gcp/creds/{role}", s.requireToken(s.generateGCPCreds))
	mux.HandleFunc("GET /v1/gcp/lease/{id}", s.requireToken(s.getGCPLease))
	mux.HandleFunc("DELETE /v1/gcp/lease/{id}", s.requireToken(s.revokeGCPLease))
	mux.HandleFunc("LIST /v1/gcp/lease/", s.requireToken(s.listGCPLeases))

	// AWS dynamic secrets engine — generate IAM user credentials or STS assumed-role sessions
	mux.HandleFunc("PUT /v1/aws/config", s.requireToken(s.putAWSConfig))
	mux.HandleFunc("GET /v1/aws/config", s.requireToken(s.getAWSConfig))
	mux.HandleFunc("DELETE /v1/aws/config", s.requireToken(s.deleteAWSConfig))
	mux.HandleFunc("PUT /v1/aws/roles/{name}", s.requireToken(s.putAWSRole))
	mux.HandleFunc("GET /v1/aws/roles/{name}", s.requireToken(s.getAWSRole))
	mux.HandleFunc("DELETE /v1/aws/roles/{name}", s.requireToken(s.deleteAWSRole))
	mux.HandleFunc("LIST /v1/aws/roles/", s.requireToken(s.listAWSRoles))
	mux.HandleFunc("POST /v1/aws/creds/{role}", s.requireToken(s.generateAWSCreds))
	mux.HandleFunc("GET /v1/aws/lease/{id}", s.requireToken(s.getAWSLease))
	mux.HandleFunc("DELETE /v1/aws/lease/{id}", s.requireToken(s.revokeAWSLease))
	mux.HandleFunc("LIST /v1/aws/lease/", s.requireToken(s.listAWSLeases))

	// LDAP / Active Directory auth — login is unauthenticated; config and role management require a token
	mux.HandleFunc("POST /v1/auth/ldap/login", s.loginLDAP)
	mux.HandleFunc("GET /v1/auth/ldap/config", s.requireToken(s.getLDAPConfig))
	mux.HandleFunc("PUT /v1/auth/ldap/config", s.requireToken(s.putLDAPConfig))
	mux.HandleFunc("PUT /v1/auth/ldap/role/{name}", s.requireToken(s.putLDAPRole))
	mux.HandleFunc("GET /v1/auth/ldap/role/{name}", s.requireToken(s.getLDAPRole))
	mux.HandleFunc("DELETE /v1/auth/ldap/role/{name}", s.requireToken(s.deleteLDAPRole))
	mux.HandleFunc("LIST /v1/auth/ldap/role/", s.requireToken(s.listLDAPRoles))

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

	// Entity & Identity system
	mux.HandleFunc("POST /v1/identity/entity", s.requireToken(s.identityCreateEntity))
	mux.HandleFunc("GET /v1/identity/entity/id/{id}", s.requireToken(s.identityGetEntityByID))
	mux.HandleFunc("POST /v1/identity/entity/id/{id}", s.requireToken(s.identityUpdateEntityByID))
	mux.HandleFunc("DELETE /v1/identity/entity/id/{id}", s.requireToken(s.identityDeleteEntityByID))
	mux.HandleFunc("GET /v1/identity/entity/name/{name}", s.requireToken(s.identityGetEntityByName))
	mux.HandleFunc("POST /v1/identity/entity/name/{name}", s.requireToken(s.identityUpsertEntityByName))
	mux.HandleFunc("DELETE /v1/identity/entity/name/{name}", s.requireToken(s.identityDeleteEntityByName))
	mux.HandleFunc("LIST /v1/identity/entity/", s.requireToken(s.identityListEntities))

	mux.HandleFunc("POST /v1/identity/entity-alias", s.requireToken(s.identityCreateAlias))
	mux.HandleFunc("GET /v1/identity/entity-alias/id/{id}", s.requireToken(s.identityGetAlias))
	mux.HandleFunc("POST /v1/identity/entity-alias/id/{id}", s.requireToken(s.identityUpdateAlias))
	mux.HandleFunc("DELETE /v1/identity/entity-alias/id/{id}", s.requireToken(s.identityDeleteAlias))
	mux.HandleFunc("LIST /v1/identity/entity-alias/id/", s.requireToken(s.identityListAliases))

	mux.HandleFunc("POST /v1/identity/group", s.requireToken(s.identityCreateGroup))
	mux.HandleFunc("GET /v1/identity/group/id/{id}", s.requireToken(s.identityGetGroupByID))
	mux.HandleFunc("POST /v1/identity/group/id/{id}", s.requireToken(s.identityUpdateGroupByID))
	mux.HandleFunc("DELETE /v1/identity/group/id/{id}", s.requireToken(s.identityDeleteGroupByID))
	mux.HandleFunc("GET /v1/identity/group/name/{name}", s.requireToken(s.identityGetGroupByName))
	mux.HandleFunc("POST /v1/identity/group/name/{name}", s.requireToken(s.identityUpsertGroupByName))
	mux.HandleFunc("DELETE /v1/identity/group/name/{name}", s.requireToken(s.identityDeleteGroupByName))
	mux.HandleFunc("LIST /v1/identity/group/", s.requireToken(s.identityListGroups))

	mux.HandleFunc("POST /v1/identity/group-alias", s.requireToken(s.identityCreateGroupAlias))
	mux.HandleFunc("GET /v1/identity/group-alias/id/{id}", s.requireToken(s.identityGetGroupAliasByID))
	mux.HandleFunc("DELETE /v1/identity/group-alias/id/{id}", s.requireToken(s.identityDeleteGroupAlias))
	mux.HandleFunc("LIST /v1/identity/group-alias/", s.requireToken(s.identityListGroupAliases))

	mux.HandleFunc("POST /v1/identity/lookup/entity", s.requireToken(s.identityLookupEntity))
	mux.HandleFunc("POST /v1/identity/lookup/group", s.requireToken(s.identityLookupGroup))

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
// TrackUse is called here — exactly once per authenticated HTTP request —
// to enforce MaxUses limits.
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
		if err := s.core.TrackUse(r.Context(), id); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token has exceeded its use limit"})
			return
		}
		ctx := context.WithValue(r.Context(), tokenCtxKey, id)
		if ns := r.Header.Get("X-Tuck-Namespace"); ns != "" {
			ctx = context.WithValue(ctx, nsCtxKey, ns)
		}
		next(w, r.WithContext(ctx))
	}
}

func tokenFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(tokenCtxKey).(string)
	return id
}

func nsFromCtx(ctx context.Context) string {
	ns, _ := ctx.Value(nsCtxKey).(string)
	return ns
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	sealed := s.core.Sealed()
	haEnabled := s.core.ClusterBackend() != nil
	writeJSON(w, http.StatusOK, map[string]any{
		"version":        version.Version,
		"commit":         version.Commit,
		"build_date":     version.BuildDate,
		"sealed":         sealed,
		"ha_enabled":     haEnabled,
		"uptime_seconds": metrics.UptimeSeconds(),
	})
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, barrier.ErrSealed):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sealed"})
	case errors.Is(err, core.ErrTokenInvalid):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
	case errors.Is(err, core.ErrUnauthorized):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
	case errors.Is(err, core.ErrNotRenewable):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is not renewable"})
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
