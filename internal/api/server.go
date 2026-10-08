// Package api exposes Tuck's HTTP interface.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/NAGenaev/tuck/internal/audit"
	"github.com/NAGenaev/tuck/internal/barrier"
	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/metrics"
	"github.com/NAGenaev/tuck/internal/namespace"
	physraft "github.com/NAGenaev/tuck/internal/physical/raft"
	"github.com/NAGenaev/tuck/internal/ratelimit"
	"github.com/NAGenaev/tuck/internal/sysconfig"
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
	core         *core.Core
	audit        audit.Loggable
	ipLimiter    *ratelimit.Limiter
	tokenLimiter *ratelimit.Limiter
	ldapThrottle loginThrottle
	// oidcLimiter bounds the unauthenticated OIDC start/finish endpoints per
	// source address; it is always on, independent of the sysconfig limiters.
	oidcLimiter *ratelimit.Limiter
	// tuckSecrets backs the read-only TuckSecret listing; nil when the server
	// has no Kubernetes API access.
	tuckSecrets TuckSecretSource
	// rateDefaults are the limits used where the stored runtime config is silent.
	rateDefaults sysconfig.Config
	// reloadClusters re-reads the trusted-cluster list from the config file; nil
	// when the server was started without one.
	reloadClusters func() (added, removed []string, err error)
}

// New returns an HTTP server wired to the core's audit Dispatcher.
// Rate limiters are disabled by default; call ApplyRateConfig to enable.
func New(c *core.Core) *Server {
	return &Server{
		core:         c,
		audit:        c.Dispatcher(),
		ipLimiter:    ratelimit.New(0, 0),
		tokenLimiter: ratelimit.New(0, 0),
		oidcLimiter:  ratelimit.New(5, 30),
	}
}

// NewWithAudit returns an HTTP server with a custom audit sink (used in tests).
func NewWithAudit(c *core.Core, l audit.Loggable) *Server {
	return &Server{
		core:         c,
		audit:        l,
		ipLimiter:    ratelimit.New(0, 0),
		tokenLimiter: ratelimit.New(0, 0),
		oidcLimiter:  ratelimit.New(5, 30),
	}
}

// SetRateDefaults sets the limits that apply wherever the stored runtime config
// says nothing (the --rate-* flags) and applies them right away.
func (s *Server) SetRateDefaults(d sysconfig.Config) {
	s.rateDefaults = d
	s.ApplyRateConfig(sysconfig.Config{})
}

// ApplyRateConfig updates the in-memory rate limiters from a sysconfig snapshot.
// A field left at 0 keeps the server default (the --rate-* flag); a negative
// value disables that limit.
func (s *Server) ApplyRateConfig(cfg sysconfig.Config) {
	rps := func(v, def float64) float64 {
		switch {
		case v < 0:
			return 0
		case v == 0:
			return def
		}
		return v
	}
	burst := func(v, def int) int {
		switch {
		case v < 0:
			return 0
		case v == 0:
			return def
		}
		return v
	}
	d := s.rateDefaults
	s.ipLimiter.Reconfigure(rps(cfg.IPRateLimitRPS, d.IPRateLimitRPS), burst(cfg.IPRateLimitBurst, d.IPRateLimitBurst))
	s.tokenLimiter.Reconfigure(rps(cfg.TokenRateLimitRPS, d.TokenRateLimitRPS), burst(cfg.TokenRateLimitBurst, d.TokenRateLimitBurst))
}

// StartLimiterCleanup periodically removes stale rate-limit buckets.
// It returns when ctx is cancelled.
func (s *Server) StartLimiterCleanup(ctx context.Context) {
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.ipLimiter.Cleanup(10 * time.Minute)
				s.tokenLimiter.Cleanup(10 * time.Minute)
				s.oidcLimiter.Cleanup(10 * time.Minute)
			}
		}
	}()
}

// Handler builds the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Embedded web dashboard — a React SPA forked from Remnawave's UI shell
	// (AGPL-3.0-only — see web/NOTICE), served at /ui/, root redirects there.
	mux.Handle("/ui/", http.StripPrefix("/ui", ui.Handler()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ui/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("GET /v1/health", s.health)

	// Sys endpoints: seal-status and unseal are unauthenticated; seal requires root.
	// Audit sink management
	mux.HandleFunc("PUT /v1/sys/audit/webhook/{name}", s.requireToken(s.putAuditWebhook))
	mux.HandleFunc("PUT /v1/sys/audit/file/{name}", s.requireToken(s.putAuditFile))
	mux.HandleFunc("PUT /v1/sys/audit/syslog/{name}", s.requireToken(s.putAuditSyslog))
	mux.HandleFunc("PUT /v1/sys/audit/state/{name}", s.requireToken(s.putAuditSinkState))
	// Readable audit log: recent records, live tail, stats, hash-chain verification
	mux.HandleFunc("GET /v1/sys/audit/events", s.requireToken(s.getAuditEvents))
	mux.HandleFunc("GET /v1/sys/audit/events/stream", s.requireToken(s.getAuditEventsStream))
	mux.HandleFunc("GET /v1/sys/audit/stats", s.requireToken(s.getAuditStats))
	mux.HandleFunc("POST /v1/sys/audit/verify", s.requireToken(s.postAuditVerify))
	mux.HandleFunc("DELETE /v1/sys/audit/{name}", s.requireToken(s.deleteAuditSink))
	s.registerListCompat(mux, "/v1/sys/audit/", s.listAuditSinks)

	// Namespace management. Names may contain "/" as an organizational naming
	// convention (e.g. "team-a/project-x"); the full name including slashes is
	// still one flat, atomic, fully-isolated namespace, so {name...} must
	// capture the whole remainder of the path, not just one segment. A GET
	// wildcard on this prefix cannot coexist with the usual GET+?list=true
	// list-compat shim registered via registerListCompat (both would match the
	// exact same request set and ServeMux panics on the conflict), so the
	// list-compat GET is registered directly here against the exact empty-path
	// pattern "{$}" instead of the bare prefix: "{$}" matches only
	// "/v1/sys/namespaces/" itself (any query string, e.g. ?list=true, still
	// applies — query is not part of path matching), which is a strict subset
	// of what "{name...}" matches, so the two coexist without ambiguity. The
	// "/stats" suffix (formerly its own route) is now dispatched from inside
	// getNamespace since {name...} would otherwise swallow it as part of name.
	mux.HandleFunc("POST /v1/sys/namespaces", s.requireToken(s.createNamespace))
	mux.HandleFunc("POST /v1/sys/teams", s.requireToken(s.onboardTeam))
	mux.HandleFunc("POST /v1/sys/teams/sync", s.requireToken(s.syncTeamPolicies))
	mux.HandleFunc("GET /v1/sys/namespaces/{name...}", s.requireToken(s.getNamespace))
	mux.HandleFunc("DELETE /v1/sys/namespaces/{name...}", s.requireToken(s.deleteNamespace))
	mux.HandleFunc("PUT /v1/sys/namespaces/{name...}", s.requireToken(s.updateNamespace))
	mux.HandleFunc("GET /v1/sys/usage/secrets", s.requireToken(s.usageSecrets))
	mux.HandleFunc("LIST /v1/sys/namespaces/", s.requireToken(s.listNamespaces))
	mux.HandleFunc("GET /v1/sys/namespaces/{$}", s.requireToken(func(w http.ResponseWriter, r *http.Request) {
		if !wantsList(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "collection listing requires ?list=true",
			})
			return
		}
		s.listNamespaces(w, r)
	}))

	// Runtime configuration
	mux.HandleFunc("GET /v1/sys/config", s.requireToken(s.getSysConfig))
	mux.HandleFunc("PUT /v1/sys/config", s.requireToken(s.putSysConfig))

	// Mount table — list, create, delete secret engine mounts
	mux.HandleFunc("GET /v1/sys/mounts", s.requireToken(s.listMounts))
	mux.HandleFunc("POST /v1/sys/mounts/{path...}", s.requireToken(s.createMount))
	mux.HandleFunc("DELETE /v1/sys/mounts/{path...}", s.requireToken(s.deleteMount))

	// Per-mount tuning (GET/POST /v1/sys/mounts-tune/{path...})
	mux.HandleFunc("GET /v1/sys/mounts-tune/{path...}", s.requireToken(s.getMountConfig))
	mux.HandleFunc("POST /v1/sys/mounts-tune/{path...}", s.requireToken(s.putMountConfig))

	mux.HandleFunc("GET /v1/sys/seal-status", s.getSealStatus)
	mux.HandleFunc("GET /v1/sys/ready", s.getReady)
	mux.HandleFunc("GET /v1/sys/leader", s.getLeader)
	mux.HandleFunc("POST /v1/sys/unseal", s.postUnseal)
	mux.HandleFunc("POST /v1/sys/seal", s.requireToken(s.postSeal))
	mux.HandleFunc("GET /v1/sys/snapshot", s.requireToken(s.getSnapshot))
	mux.HandleFunc("POST /v1/sys/rotate", s.requireToken(s.postRotate))
	mux.HandleFunc("POST /v1/sys/rotate-data-key", s.requireToken(s.postRotateDataKey))

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
	mux.HandleFunc("GET /v1/sys/wrapping/list", s.requireToken(s.listWrapped))
	mux.HandleFunc("GET /v1/sys/wrapping/stats", s.requireToken(s.wrappingStats))
	mux.HandleFunc("DELETE /v1/sys/wrapping/revoke-accessor/{accessor}", s.requireToken(s.revokeWrappedByAccessor))

	mux.HandleFunc("GET /metrics", metrics.Handler())

	mux.HandleFunc("GET /v1/secret/{path...}", s.requireToken(s.getSecret))
	mux.HandleFunc("PUT /v1/secret/{path...}", s.requireToken(s.putSecret))
	mux.HandleFunc("DELETE /v1/secret/{path...}", s.requireToken(s.deleteSecret))
	mux.HandleFunc("LIST /v1/secret/{path...}", s.requireToken(s.listSecrets))

	// Token roles
	mux.HandleFunc("PUT /v1/auth/token/roles/{name}", s.requireToken(s.putTokenRole))
	mux.HandleFunc("GET /v1/auth/token/roles/{name}", s.requireToken(s.getTokenRole))
	mux.HandleFunc("DELETE /v1/auth/token/roles/{name}", s.requireToken(s.deleteTokenRole))
	s.registerListCompat(mux, "/v1/auth/token/roles/", s.listTokenRoles)
	mux.HandleFunc("POST /v1/auth/token/roles/{role}/create", s.requireToken(s.createTokenFromRole))

	mux.HandleFunc("POST /v1/auth/token", s.requireToken(s.createToken))
	mux.HandleFunc("GET /v1/auth/token/{id}", s.requireToken(s.lookupToken))
	mux.HandleFunc("DELETE /v1/auth/token/{id}", s.requireToken(s.revokeToken))
	mux.HandleFunc("POST /v1/auth/token/{id}/renew", s.requireToken(s.renewToken))
	s.registerListCompat(mux, "/v1/auth/token/", s.listTokens)
	mux.HandleFunc("POST /v1/auth/token/lookup-accessor", s.requireToken(s.lookupByAccessor))
	mux.HandleFunc("DELETE /v1/auth/token/revoke-accessor", s.requireToken(s.revokeByAccessor))
	mux.HandleFunc("GET /v1/auth/token/lookup-self", s.requireToken(s.lookupSelf))
	mux.HandleFunc("POST /v1/auth/token/renew-self", s.requireToken(s.renewSelf))

	mux.HandleFunc("PUT /v1/policy/{name}", s.requireToken(s.putPolicy))
	mux.HandleFunc("GET /v1/policy/{name}", s.requireToken(s.getPolicy))
	mux.HandleFunc("DELETE /v1/policy/{name}", s.requireToken(s.deletePolicy))
	s.registerListCompat(mux, "/v1/policy/", s.listPolicies)

	mux.HandleFunc("POST /v1/auth/kubernetes/login", s.loginK8s)
	mux.HandleFunc("PUT /v1/auth/kubernetes/role/{namespace}/{sa}", s.requireToken(s.putK8sRole))
	mux.HandleFunc("GET /v1/auth/kubernetes/role/{namespace}/{sa}", s.requireToken(s.getK8sRole))
	mux.HandleFunc("DELETE /v1/auth/kubernetes/role/{namespace}/{sa}", s.requireToken(s.deleteK8sRole))
	mux.HandleFunc("GET /v1/auth/kubernetes/clusters", s.requireToken(s.listK8sClusters))
	mux.HandleFunc("POST /v1/auth/kubernetes/clusters/reload", s.requireToken(s.reloadK8sClusters))
	mux.HandleFunc("GET /v1/sys/k8s/tucksecrets", s.requireToken(s.listTuckSecrets))
	mux.HandleFunc("PUT /v1/auth/kubernetes/clusters/{cluster}/role/{namespace}/{sa}", s.requireToken(s.putK8sRole))
	mux.HandleFunc("GET /v1/auth/kubernetes/clusters/{cluster}/role/{namespace}/{sa}", s.requireToken(s.getK8sRole))
	mux.HandleFunc("DELETE /v1/auth/kubernetes/clusters/{cluster}/role/{namespace}/{sa}", s.requireToken(s.deleteK8sRole))

	// JWT/OIDC auth — login is unauthenticated; config and role management require a token
	mux.HandleFunc("POST /v1/auth/jwt/login", s.loginJWT)
	mux.HandleFunc("GET /v1/auth/oidc/status", s.oidcStatus)
	mux.HandleFunc("GET /v1/auth/oidc/auth-url", s.oidcAuthURL)
	mux.HandleFunc("POST /v1/auth/oidc/callback", s.oidcCallback)
	mux.HandleFunc("GET /v1/auth/jwt/config", s.requireToken(s.getJWTConfig))
	mux.HandleFunc("PUT /v1/auth/jwt/config", s.requireToken(s.putJWTConfig))
	mux.HandleFunc("PUT /v1/auth/jwt/role/{name}", s.requireToken(s.putJWTRole))
	mux.HandleFunc("GET /v1/auth/jwt/role/{name}", s.requireToken(s.getJWTRole))
	mux.HandleFunc("DELETE /v1/auth/jwt/role/{name}", s.requireToken(s.deleteJWTRole))
	s.registerListCompat(mux, "/v1/auth/jwt/role/", s.listJWTRoles)

	// LDAP / Active Directory auth — login is unauthenticated; config and role management require a token
	mux.HandleFunc("POST /v1/auth/ldap/login", s.loginLDAP)
	mux.HandleFunc("GET /v1/auth/ldap/config", s.requireToken(s.getLDAPConfig))
	mux.HandleFunc("PUT /v1/auth/ldap/config", s.requireToken(s.putLDAPConfig))
	mux.HandleFunc("POST /v1/auth/ldap/recheck", s.requireToken(s.recheckLDAP))
	mux.HandleFunc("POST /v1/auth/ldap/users/{username}/revoke", s.requireToken(s.revokeLDAPUser))
	mux.HandleFunc("PUT /v1/auth/ldap/role/{name}", s.requireToken(s.putLDAPRole))
	mux.HandleFunc("GET /v1/auth/ldap/role/{name}", s.requireToken(s.getLDAPRole))
	mux.HandleFunc("DELETE /v1/auth/ldap/role/{name}", s.requireToken(s.deleteLDAPRole))
	s.registerListCompat(mux, "/v1/auth/ldap/role/", s.listLDAPRoles)

	// AppRole auth — login is unauthenticated; role/secret-id management requires a token
	mux.HandleFunc("POST /v1/auth/approle/login", s.loginAppRole)
	mux.HandleFunc("PUT /v1/auth/approle/role/{name}", s.requireToken(s.putAppRole))
	mux.HandleFunc("GET /v1/auth/approle/role/{name}", s.requireToken(s.getAppRole))
	mux.HandleFunc("DELETE /v1/auth/approle/role/{name}", s.requireToken(s.deleteAppRole))
	s.registerListCompat(mux, "/v1/auth/approle/role/", s.listAppRoles)
	mux.HandleFunc("POST /v1/auth/approle/role/{name}/secret-id", s.requireToken(s.generateSecretID))
	mux.HandleFunc("GET /v1/auth/approle/role/{name}/secret-id/{id}", s.requireToken(s.lookupSecretID))
	mux.HandleFunc("DELETE /v1/auth/approle/role/{name}/secret-id/{id}", s.requireToken(s.destroySecretID))

	// KV v2 — versioned secrets
	mux.HandleFunc("PUT /v2/secret/{path...}", s.requireToken(s.v2WriteSecret))
	mux.HandleFunc("GET /v2/secret/{path...}", s.requireToken(s.v2ReadSecret))
	mux.HandleFunc("DELETE /v2/secret/{path...}", s.requireToken(s.v2DeleteSecret))
	mux.HandleFunc("LIST /v2/secret/{path...}", s.requireToken(s.v2ListSecrets))
	mux.HandleFunc("POST /v2/secret/undelete/{path...}", s.requireToken(s.v2Undelete))
	mux.HandleFunc("POST /v2/secret/destroy/{path...}", s.requireToken(s.v2Destroy))
	mux.HandleFunc("GET /v2/secret/metadata/{path...}", s.requireToken(s.v2GetMeta))
	mux.HandleFunc("PUT /v2/secret/metadata/{path...}", s.requireToken(s.v2UpdateMeta))
	mux.HandleFunc("PATCH /v2/secret/metadata/{path...}", s.requireToken(s.v2PatchMeta))
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
	s.registerListCompat(mux, "/v1/identity/entity/", s.identityListEntities)

	mux.HandleFunc("POST /v1/identity/entity-alias", s.requireToken(s.identityCreateAlias))
	mux.HandleFunc("GET /v1/identity/entity-alias/id/{id}", s.requireToken(s.identityGetAlias))
	mux.HandleFunc("POST /v1/identity/entity-alias/id/{id}", s.requireToken(s.identityUpdateAlias))
	mux.HandleFunc("DELETE /v1/identity/entity-alias/id/{id}", s.requireToken(s.identityDeleteAlias))
	s.registerListCompat(mux, "/v1/identity/entity-alias/id/", s.identityListAliases)

	mux.HandleFunc("POST /v1/identity/group", s.requireToken(s.identityCreateGroup))
	mux.HandleFunc("GET /v1/identity/group/id/{id}", s.requireToken(s.identityGetGroupByID))
	mux.HandleFunc("POST /v1/identity/group/id/{id}", s.requireToken(s.identityUpdateGroupByID))
	mux.HandleFunc("DELETE /v1/identity/group/id/{id}", s.requireToken(s.identityDeleteGroupByID))
	mux.HandleFunc("GET /v1/identity/group/name/{name}", s.requireToken(s.identityGetGroupByName))
	mux.HandleFunc("POST /v1/identity/group/name/{name}", s.requireToken(s.identityUpsertGroupByName))
	mux.HandleFunc("DELETE /v1/identity/group/name/{name}", s.requireToken(s.identityDeleteGroupByName))
	s.registerListCompat(mux, "/v1/identity/group/", s.identityListGroups)

	mux.HandleFunc("POST /v1/identity/group-alias", s.requireToken(s.identityCreateGroupAlias))
	mux.HandleFunc("GET /v1/identity/group-alias/id/{id}", s.requireToken(s.identityGetGroupAliasByID))
	mux.HandleFunc("DELETE /v1/identity/group-alias/id/{id}", s.requireToken(s.identityDeleteGroupAlias))
	s.registerListCompat(mux, "/v1/identity/group-alias/", s.identityListGroupAliases)

	mux.HandleFunc("POST /v1/identity/lookup/entity", s.requireToken(s.identityLookupEntity))
	mux.HandleFunc("POST /v1/identity/lookup/group", s.requireToken(s.identityLookupGroup))

	// OpenAPI spec
	mux.HandleFunc("GET /openapi.json", serveOpenAPI)

	// Cluster management (Raft HA mode only)
	mux.HandleFunc("GET /v1/sys/cluster", s.requireToken(s.getClusterStatus))
	mux.HandleFunc("POST /v1/sys/cluster/join", s.requireToken(s.postClusterJoin))
	mux.HandleFunc("POST /v1/sys/cluster/join-token", s.requireToken(s.postClusterJoinToken))
	mux.HandleFunc("DELETE /v1/sys/cluster/node/{id}", s.requireToken(s.deleteClusterNode))
	mux.HandleFunc("POST /v1/sys/cluster/step-down", s.requireToken(s.postClusterStepDown))
	mux.HandleFunc("GET /v1/sys/cluster/snapshot", s.requireToken(s.getClusterSnapshots))
	mux.HandleFunc("POST /v1/sys/cluster/snapshot", s.requireToken(s.postClusterSnapshot))
	mux.HandleFunc("POST /v1/sys/cluster/restore", s.requireToken(s.postClusterRestore))
	mux.HandleFunc("GET /v1/sys/cluster/snapshot/{id}", s.requireToken(s.getClusterSnapshot))

	h := http.Handler(mux)
	h = s.tokenLimiter.TokenMiddleware(h)
	h = s.ipLimiter.Middleware(h)
	return audit.Middleware(s.audit, h)
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
		tok, err := s.core.Authenticate(r.Context(), id)
		if err != nil {
			if errors.Is(err, barrier.ErrSealed) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sealed"})
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
			return
		}
		audit.SetIdentity(r.Context(), tok.DisplayName, tok.EntityID)
		if err := s.core.TrackUse(r.Context(), id); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token has exceeded its use limit"})
			return
		}
		ctx := context.WithValue(r.Context(), tokenCtxKey, id)
		ns := r.Header.Get("X-Tuck-Namespace")
		if tok.Namespace != "" && tok.Namespace != namespace.RootNamespace {
			// A namespace-bound token is never usable outside its namespace;
			// the client-supplied header may only repeat it.
			if ns != "" && ns != tok.Namespace {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
				return
			}
			ns = tok.Namespace
		}
		if ns != "" && ns != namespace.RootNamespace {
			if _, err := s.core.GetNamespace(r.Context(), ns); err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "namespace not found"})
				return
			}
			ctx = context.WithValue(ctx, nsCtxKey, ns)
			audit.SetNamespace(ctx, ns) // validated namespace goes into the audit record
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

// decodeJSON unmarshals body into v, writing a 400 response naming the real
// decode error (e.g. a field with the wrong JSON type) on failure. Handlers
// used to fold this into `err != nil || req.Field == ""` and report every
// failure as "Field required" — including type-mismatch errors that have
// nothing to do with Field being empty, which sent debugging in the wrong
// direction. Returns true on success; on false the response is already
// written and the caller should return immediately.
func decodeJSON(w http.ResponseWriter, body []byte, v any) bool {
	if err := json.Unmarshal(body, v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return false
	}
	return true
}
