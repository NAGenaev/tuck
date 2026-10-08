package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
)

const teamBody = `{
  "name":"payments","description":"Payments","owner":"pay-lead",
  "groups":{"admin":["CN=pay-admins"],"developer":["CN=pay-dev"],"reader":["CN=pay-audit"]},
  "k8s":[{"namespace":"payments","service_account":"api","access":"developer"}],
  "ttl":"1h"}`

func teamToken(t *testing.T, ts *httptest.Server, root, ns, pol string) string {
	t.Helper()
	s, b := nsReq(t, ts, http.MethodPost, "/v1/auth/token", `{"policies":["`+pol+`"]}`, root, ns)
	if s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("token for %s: %d %s", pol, s, b)
	}
	return jsonField(t, []byte(b), "id")
}

func TestOnboardTeam(t *testing.T) {
	ts, _, root := newTestServerWithK8s(t, &mockReviewer{result: &k8sauth.ReviewResult{Authenticated: true, Username: "system:serviceaccount:payments:api"}})

	// Dry run changes nothing.
	s, b := nsReq(t, ts, http.MethodPost, "/v1/sys/teams", strings.Replace(teamBody, `"ttl"`, `"dry_run":true,"ttl"`, 1), root, "")
	if s != http.StatusOK || !strings.Contains(b, `"dry_run":true`) || strings.Contains(b, `"conflict"`) {
		t.Fatalf("dry run: %d %s", s, b)
	}
	if s, _ := nsReq(t, ts, http.MethodGet, "/v1/sys/namespaces/payments", "", root, ""); s != http.StatusNotFound {
		t.Fatalf("dry run created the namespace: %d", s)
	}

	s, b = nsReq(t, ts, http.MethodPost, "/v1/sys/teams", teamBody, root, "")
	if s != http.StatusCreated {
		t.Fatalf("onboard: %d %s", s, b)
	}
	if !strings.Contains(b, "tuck.io/tuck-namespace: payments") {
		t.Errorf("missing namespace annotation hint: %s", b)
	}
	for _, role := range []string{"payments-admin", "payments-developer", "payments-reader"} {
		s, b := nsReq(t, ts, http.MethodGet, "/v1/auth/ldap/role/"+role, "", root, "")
		if s != http.StatusOK || !strings.Contains(b, `"tuck_namespace":"payments"`) {
			t.Fatalf("ldap role %s: %d %s", role, s, b)
		}
	}
	if s, b := nsReq(t, ts, http.MethodGet, "/v1/auth/kubernetes/role/payments/api", "", root, ""); s != http.StatusOK || !strings.Contains(b, "team-developer") {
		t.Fatalf("k8s role: %d %s", s, b)
	}

	// Access levels behave as advertised.
	admin := teamToken(t, ts, root, "payments", "team-admin")
	dev := teamToken(t, ts, root, "payments", "team-developer")
	reader := teamToken(t, ts, root, "payments", "team-reader")
	put := func(tok string) int {
		s, _ := nsReq(t, ts, http.MethodPut, "/v1/secret/app/db", `{"value":"x"}`, tok, "payments")
		return s
	}
	if got := put(admin); got != http.StatusNoContent {
		t.Errorf("admin write: %d", got)
	}
	if got := put(dev); got != http.StatusNoContent {
		t.Errorf("developer write: %d", got)
	}
	if got := put(reader); got != http.StatusForbidden {
		t.Errorf("reader write: %d, want 403", got)
	}
	if s, _ := nsReq(t, ts, http.MethodGet, "/v1/secret/app/db", "", reader, "payments"); s != http.StatusOK {
		t.Errorf("reader read: %d", s)
	}
	if s, _ := nsReq(t, ts, http.MethodDelete, "/v1/secret/app/db", "", dev, "payments"); s != http.StatusForbidden {
		t.Errorf("developer delete: %d, want 403", s)
	}
	// The Policies page lists policies with LIST on auth/policy, so team admins need it.
	if s, b := nsReq(t, ts, "LIST", "/v1/policy/", "", admin, "payments"); s != http.StatusOK {
		t.Errorf("team admin cannot list policies: %d %s", s, b)
	}
	// A team admin cannot bind more groups to its namespace.
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/ldap/role/evil", `{"groups":["CN=x"],"policies":["team-admin"]}`, admin, "payments"); s != http.StatusForbidden {
		t.Errorf("team admin created an LDAP role: %d %s", s, b)
	}
	// ... nor onboard teams.
	if s, _ := nsReq(t, ts, http.MethodPost, "/v1/sys/teams", teamBody, admin, "payments"); s != http.StatusForbidden {
		t.Errorf("team admin used onboarding: %d", s)
	}

	// Repeating it conflicts and leaves the first run intact.
	s, b = nsReq(t, ts, http.MethodPost, "/v1/sys/teams", teamBody, root, "")
	if s != http.StatusConflict || !strings.Contains(b, `"conflict"`) {
		t.Fatalf("repeat: %d %s", s, b)
	}
	if s, _ := nsReq(t, ts, http.MethodGet, "/v1/sys/namespaces/payments", "", root, ""); s != http.StatusOK {
		t.Errorf("repeat removed the namespace: %d", s)
	}
}

func TestOnboardTeamRollbackAndValidation(t *testing.T) {
	ts, _, root := newTestServerWithK8s(t, &mockReviewer{result: &k8sauth.ReviewResult{}})

	// A role name taken by someone else blocks the whole onboarding up front.
	nsReq(t, ts, http.MethodPost, "/v1/sys/namespaces", `{"name":"other"}`, root, "")
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/auth/ldap/role/billing-reader", `{"groups":["CN=z"],"policies":["default"]}`, root, ""); s != http.StatusNoContent {
		t.Fatalf("seed role: %d %s", s, b)
	}
	s, b := nsReq(t, ts, http.MethodPost, "/v1/sys/teams", `{"name":"billing","groups":{"reader":["CN=r"]}}`, root, "")
	if s != http.StatusConflict {
		t.Fatalf("role conflict: %d %s", s, b)
	}
	if s, _ := nsReq(t, ts, http.MethodGet, "/v1/sys/namespaces/billing", "", root, ""); s != http.StatusNotFound {
		t.Errorf("namespace left behind after conflict: %d", s)
	}

	// A step that fails midway (unknown cluster) undoes what came before.
	s, b = nsReq(t, ts, http.MethodPost, "/v1/sys/teams", `{"name":"risk","groups":{"admin":["CN=a"]},"k8s":[{"cluster":"nope","namespace":"n","service_account":"s"}]}`, root, "")
	if s != http.StatusBadRequest {
		t.Fatalf("unknown cluster: %d %s", s, b)
	}
	if s, _ := nsReq(t, ts, http.MethodGet, "/v1/sys/namespaces/risk", "", root, ""); s != http.StatusNotFound {
		t.Errorf("namespace left behind after failure: %d", s)
	}
	if s, _ := nsReq(t, ts, http.MethodGet, "/v1/auth/ldap/role/risk-admin", "", root, ""); s == http.StatusOK {
		t.Error("ldap role left behind after failure")
	}

	for name, body := range map[string]string{
		"no groups":  `{"name":"x1"}`,
		"bad name":   `{"name":"Bad Name","groups":{"admin":["g"]}}`,
		"bad access": `{"name":"x2","groups":{"root":["g"]}}`,
		"bad k8s":    `{"name":"x3","groups":{"admin":["g"]},"k8s":[{"namespace":"n"}]}`,
	} {
		if s, b := nsReq(t, ts, http.MethodPost, "/v1/sys/teams", body, root, ""); s != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, s, b)
		}
	}
}

func TestSyncTeamPoliciesBringsOldTeamsUpToDate(t *testing.T) {
	ts, _, root := newTestServerWithK8s(t, &mockReviewer{result: &k8sauth.ReviewResult{}})
	if s, b := nsReq(t, ts, http.MethodPost, "/v1/sys/teams", `{"name":"payments","groups":{"admin":["pay-admins"],"reader":["pay-ro"]}}`, root, ""); s != http.StatusCreated {
		t.Fatalf("onboard: %d %s", s, b)
	}
	// An older template: no right to list policies. A team that renamed or removed
	// a policy (team-developer was never created here) must be left alone.
	old := `{"rules":[{"path":"secret/**","capabilities":["read","write","delete","list"]}]}`
	if s, b := nsReq(t, ts, http.MethodPut, "/v1/policy/team-admin", old, root, "payments"); s != http.StatusNoContent {
		t.Fatalf("age the policy: %d %s", s, b)
	}
	admin := teamToken(t, ts, root, "payments", "team-admin")
	if s, _ := nsReq(t, ts, "LIST", "/v1/policy/", "", admin, "payments"); s != http.StatusForbidden {
		t.Fatalf("the aged policy should not list policies, got %d", s)
	}

	s, b := nsReq(t, ts, http.MethodPost, "/v1/sys/teams/sync", `{"dry_run":true}`, root, "")
	if s != http.StatusOK || !strings.Contains(b, "would_update") || strings.Contains(b, "team-developer") {
		t.Fatalf("dry run: %d %s", s, b)
	}
	if s, _ := nsReq(t, ts, "LIST", "/v1/policy/", "", admin, "payments"); s != http.StatusForbidden {
		t.Fatal("a dry run changed a policy")
	}
	s, b = nsReq(t, ts, http.MethodPost, "/v1/sys/teams/sync", `{"name":"payments"}`, root, "")
	if s != http.StatusOK || !strings.Contains(b, `"updated"`) {
		t.Fatalf("sync: %d %s", s, b)
	}
	if s, _ := nsReq(t, ts, "LIST", "/v1/policy/", "", admin, "payments"); s != http.StatusOK {
		t.Errorf("after the sync the team admin still cannot list policies: %d", s)
	}
	// Idempotent, and a team admin cannot run it.
	if s, b := nsReq(t, ts, http.MethodPost, "/v1/sys/teams/sync", ``, root, ""); s != http.StatusOK || strings.Contains(b, `"updated"`) {
		t.Errorf("second sync: %d %s", s, b)
	}
	if s, _ := nsReq(t, ts, http.MethodPost, "/v1/sys/teams/sync", ``, admin, "payments"); s != http.StatusForbidden {
		t.Errorf("team admin ran the sync: %d", s)
	}
	if s, _ := nsReq(t, ts, http.MethodPost, "/v1/sys/teams/sync", `{"name":"nope"}`, root, ""); s != http.StatusNotFound {
		t.Errorf("unknown namespace: %d, want 404", s)
	}
}
