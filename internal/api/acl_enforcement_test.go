package api

import (
	"net/http"
	"testing"
)

// TestACLEnforcement_UnprivilegedTokenDeniedAcrossEngines is a regression test
// for a session-wide finding: transit, pki, ssh, totp, database, aws, gcp,
// azure, namespaces, cluster, approle role/secret-id management, jwt/ldap
// role/config management, and the identity engine all called their core
// methods directly without ever checking s.core.EnforceAccess — any token
// with ANY valid, non-expired policy (regardless of what it granted) could
// read/write/delete anything in those engines, including generating a fresh
// AppRole secret_id for a role bound to more privileged policies (full
// privilege escalation) and attaching arbitrary policies to an identity
// entity. Only kv.go, kvv2.go, tokens.go, policies.go, mounts.go, plugins.go,
// sysconfig.go, replication.go, github.go, and k8s.go enforced anything.
//
// Confirmed live against a real minikube cluster before this fix (a token
// scoped to secret/data/* read/list only could decrypt an unrelated Transit
// ciphertext, create new Transit keys, read Database role SQL statements,
// and issue PKI certificates for any role).
func TestACLEnforcement_UnprivilegedTokenDeniedAcrossEngines(t *testing.T) {
	ts, _, root := newTestServer(t)

	// A policy that grants exactly one unrelated capability — read on a KV
	// path that has nothing to do with any engine under test — so the token
	// is unambiguously non-root and non-empty-policy.
	status, _ := doJSON(t, ts, http.MethodPut, "/v1/policy/acl-test-kv-only", `{"rules":[{"path":"secret/data/unrelated","capabilities":["read"]}]}`, root)
	if status != http.StatusNoContent {
		t.Fatalf("create policy: %d", status)
	}
	status, body := doJSON(t, ts, http.MethodPost, "/v1/auth/token", `{"policies":["acl-test-kv-only"],"ttl":"10m"}`, root)
	if status != http.StatusCreated {
		t.Fatalf("create limited token: %d %s", status, body)
	}
	limited := jsonField(t, body, "id")
	if limited == "" {
		t.Fatalf("no token id in response: %s", body)
	}

	// Seed one real resource per engine (as root) so a 404 can't masquerade
	// as the 403 we're actually checking for.
	doJSON(t, ts, http.MethodPost, "/v1/transit/keys/acl-test-key", "", root)
	doJSON(t, ts, http.MethodPost, "/v1/pki/generate/root", `{"common_name":"ACL Test CA"}`, root)
	doJSON(t, ts, http.MethodPost, "/v1/ssh/generate/ca", "", root)
	doJSON(t, ts, http.MethodPost, "/v1/totp/keys/acl-test-totp", "", root)
	doJSON(t, ts, http.MethodPut, "/v1/database/config/acl-test-db", `{"plugin_name":"postgresql","connection_url":"postgres://u:p@h/db","database":"db"}`, root)
	doJSON(t, ts, http.MethodPut, "/v1/aws/config", `{"region":"us-east-1"}`, root)
	doJSON(t, ts, http.MethodPut, "/v1/gcp/config", `{"credentials_json":"{}"}`, root)
	doJSON(t, ts, http.MethodPut, "/v1/azure/config", `{"tenant_id":"t","client_id":"c","client_secret":"s","subscription_id":"s"}`, root)
	doJSON(t, ts, http.MethodPost, "/v1/sys/namespaces", `{"name":"acl-test-ns"}`, root)
	doJSON(t, ts, http.MethodPut, "/v1/auth/approle/role/acl-test-role", `{"policies":["default"]}`, root)
	doJSON(t, ts, http.MethodPut, "/v1/auth/jwt/role/acl-test-role", `{"policies":["default"],"bound_subject":"x"}`, root)
	doJSON(t, ts, http.MethodPut, "/v1/identity/entity", ``, root)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"transit get key", http.MethodGet, "/v1/transit/keys/acl-test-key", ""},
		{"transit encrypt", http.MethodPost, "/v1/transit/encrypt/acl-test-key", `{"plaintext":"AQID"}`},
		{"transit rotate", http.MethodPost, "/v1/transit/keys/acl-test-key/rotate", ""},
		{"transit list keys", http.MethodGet, "/v1/transit/keys/?list=true", ""},
		{"pki list roles", http.MethodGet, "/v1/pki/roles/?list=true", ""},
		{"pki put role", http.MethodPut, "/v1/pki/roles/acl-test-role", `{"allowed_domains":["example.com"]}`},
		{"ssh list roles", http.MethodGet, "/v1/ssh/roles/?list=true", ""},
		{"ssh put role", http.MethodPut, "/v1/ssh/roles/acl-test-role", `{"cert_type":"user"}`},
		{"totp get key", http.MethodGet, "/v1/totp/keys/acl-test-totp", ""},
		{"database get config", http.MethodGet, "/v1/database/config/acl-test-db", ""},
		{"database put role", http.MethodPut, "/v1/database/role/acl-test-dbrole", `{"db_name":"acl-test-db"}`},
		{"aws get config", http.MethodGet, "/v1/aws/config", ""},
		{"gcp get config", http.MethodGet, "/v1/gcp/config", ""},
		{"azure get config", http.MethodGet, "/v1/azure/config", ""},
		{"namespaces list", http.MethodGet, "/v1/sys/namespaces/?list=true", ""},
		{"namespaces create", http.MethodPost, "/v1/sys/namespaces", `{"name":"should-be-denied"}`},
		{"cluster status", http.MethodGet, "/v1/sys/cluster", ""},
		{"approle get role", http.MethodGet, "/v1/auth/approle/role/acl-test-role", ""},
		{"approle generate secret-id (privesc)", http.MethodPost, "/v1/auth/approle/role/acl-test-role/secret-id", ""},
		{"jwt get role", http.MethodGet, "/v1/auth/jwt/role/acl-test-role", ""},
		{"jwt put config", http.MethodPut, "/v1/auth/jwt/config", `{"jwks_uri":"https://example.com/jwks"}`},
		{"ldap put config", http.MethodPut, "/v1/auth/ldap/config", `{"urls":["ldap://example.com"],"user_dn":"dc=example"}`},
		{"identity create entity (privesc)", http.MethodPost, "/v1/identity/entity", `{"name":"acl-test-entity","policies":["default"]}`},
		{"identity list entities", http.MethodGet, "/v1/identity/entity/?list=true", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := doJSON(t, ts, tc.method, tc.path, tc.body, limited)
			if status != http.StatusForbidden {
				t.Errorf("%s %s: status = %d %s, want 403 permission denied", tc.method, tc.path, status, body)
			}
		})
	}
}
