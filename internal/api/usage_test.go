package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// usageDo issues a request and returns status and body.
func usageDo(t *testing.T, base string) func(method, path, body, tok, ns string) (int, string) {
	t.Helper()
	return func(method, path, body, tok, ns string) (int, string) {
		t.Helper()
		resp, err := http.DefaultClient.Do(authedNsReq(t, method, base+path, body, tok, ns))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
}

type statsResp struct {
	Name  string         `json:"name"`
	Stats map[string]any `json:"stats"`
}

func (r statsResp) num(t *testing.T, key string) int {
	t.Helper()
	v, ok := r.Stats[key]
	if !ok {
		t.Fatalf("stats has no %q: %v", key, r.Stats)
	}
	return int(v.(float64))
}

func mustOK(t *testing.T, want int, s int, b string) {
	t.Helper()
	if s != want {
		t.Fatalf("status = %d, want %d: %s", s, want, b)
	}
}

func TestNamespaceMetadataAndStats(t *testing.T) {
	ts, _, root := newTestServer(t)
	do := usageDo(t, ts.URL)

	s, b := do("POST", "/v1/sys/namespaces", `{"name":"team-a","description":"Платежи","owner":"payments","labels":{"env":"prod"}}`, root, "")
	mustOK(t, 201, s, b)
	if jsonField(t, []byte(b), "description") != "Платежи" || jsonField(t, []byte(b), "owner") != "payments" {
		t.Fatalf("create dropped metadata: %s", b)
	}
	// Invalid metadata is rejected before anything is stored.
	s, b = do("POST", "/v1/sys/namespaces", `{"name":"bad","owner":"a\nb"}`, root, "")
	mustOK(t, 400, s, b)
	if s, _ = do("GET", "/v1/sys/namespaces/bad", "", root, ""); s != 404 {
		t.Fatalf("rejected namespace must not exist, got %d", s)
	}

	// Content of the namespace: 2 KV v1, 3 KV v2 (one with extra versions),
	// one policy and two tokens (only the ones bound to the namespace count).
	for _, p := range []string{"a", "b/c"} {
		s, b = do("PUT", "/v1/secret/"+p, `{"value":"x"}`, root, "team-a")
		mustOK(t, 204, s, b)
	}
	for _, p := range []string{"k1", "k2", "k3"} {
		s, b = do("PUT", "/v2/secret/"+p, `{"value":"x"}`, root, "team-a")
		mustOK(t, 200, s, b)
	}
	s, b = do("PUT", "/v2/secret/k1", `{"value":"y"}`, root, "team-a")
	mustOK(t, 200, s, b)
	// A secret in root must not leak into the namespace count.
	s, b = do("PUT", "/v1/secret/rootonly", `{"value":"x"}`, root, "")
	mustOK(t, 204, s, b)
	s, b = do("PUT", "/v1/policy/rd", `{"rules":[{"path":"secret/**","capabilities":["read"]}]}`, root, "team-a")
	mustOK(t, 204, s, b)
	for i := 0; i < 2; i++ {
		s, b = do("POST", "/v1/auth/token", `{"policies":["rd"],"ttl":"10m"}`, root, "team-a")
		mustOK(t, 201, s, b)
	}

	s, b = do("GET", "/v1/sys/namespaces/team-a/stats", "", root, "")
	mustOK(t, 200, s, b)
	var st statsResp
	if err := json.Unmarshal([]byte(b), &st); err != nil {
		t.Fatal(err)
	}
	if st.Name != "team-a" || st.num(t, "secrets_kv1") != 2 || st.num(t, "secrets_kv2") != 3 ||
		st.num(t, "secrets") != 5 || st.num(t, "tokens") != 2 || st.num(t, "policies") != 1 {
		t.Fatalf("stats = %s", b)
	}
	if strings.Contains(b, "x\"") {
		t.Fatalf("stats must not contain values: %s", b)
	}

	// Update replaces the metadata, keeps name and creation time.
	s, b = do("GET", "/v1/sys/namespaces/team-a", "", root, "")
	mustOK(t, 200, s, b)
	created := jsonField(t, []byte(b), "created_at")
	s, b = do("PUT", "/v1/sys/namespaces/team-a", `{"description":"new","owner":"","labels":{"tier":"1"}}`, root, "")
	mustOK(t, 200, s, b)
	if jsonField(t, []byte(b), "description") != "new" || jsonField(t, []byte(b), "created_at") != created {
		t.Fatalf("update = %s", b)
	}
	if strings.Contains(b, "payments") || strings.Contains(b, "prod") {
		t.Fatalf("update must replace, not merge: %s", b)
	}
	s, b = do("PUT", "/v1/sys/namespaces/team-a", `{"description":"`+strings.Repeat("a", 600)+`"}`, root, "")
	mustOK(t, 400, s, b)
	s, b = do("PUT", "/v1/sys/namespaces/missing", `{"description":"x"}`, root, "")
	mustOK(t, 404, s, b)
	s, b = do("GET", "/v1/sys/namespaces/missing/stats", "", root, "")
	mustOK(t, 404, s, b)

	// List: additive "namespaces" with metadata, counters only on request.
	s, b = do("GET", "/v1/sys/namespaces/?list=true", "", root, "")
	mustOK(t, 200, s, b)
	var list struct {
		Keys       []string `json:"keys"`
		Namespaces []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			Stats       map[string]any `json:"stats"`
		} `json:"namespaces"`
	}
	if err := json.Unmarshal([]byte(b), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Keys) != 1 || len(list.Namespaces) != 1 || list.Namespaces[0].Description != "new" || list.Namespaces[0].Stats != nil {
		t.Fatalf("list = %s", b)
	}
	s, b = do("GET", "/v1/sys/namespaces/?list=true&stats=true", "", root, "")
	mustOK(t, 200, s, b)
	if err := json.Unmarshal([]byte(b), &list); err != nil || list.Namespaces[0].Stats["secrets"].(float64) != 5 {
		t.Fatalf("list with stats = %s (%v)", b, err)
	}
}

// A caller only gets the counters it could list itself.
func TestNamespaceStatsPermissionMask(t *testing.T) {
	ts, _, root := newTestServer(t)
	do := usageDo(t, ts.URL)
	s0, b0 := do("POST", "/v1/sys/namespaces", `{"name":"team-a"}`, root, "")
	mustOK(t, 201, s0, b0)
	if s, b := do("PUT", "/v1/secret/a", `{"value":"x"}`, root, "team-a"); s != 204 {
		t.Fatal(s, b)
	}
	// May read namespace info and list policies, nothing else.
	if s, b := do("PUT", "/v1/policy/viewer", `{"rules":[
		{"path":"sys/namespaces/**","capabilities":["read"]},
		{"path":"auth/policy","capabilities":["list"]}]}`, root, ""); s != 204 {
		t.Fatal(s, b)
	}
	s, b := do("POST", "/v1/auth/token", `{"policies":["viewer"],"ttl":"10m"}`, root, "")
	mustOK(t, 201, s, b)
	viewer := jsonField(t, []byte(b), "id")

	s, b = do("GET", "/v1/sys/namespaces/team-a/stats", "", viewer, "")
	mustOK(t, 200, s, b)
	var st statsResp
	if err := json.Unmarshal([]byte(b), &st); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Stats["policies"]; !ok {
		t.Fatalf("policies must be visible: %s", b)
	}
	for _, k := range []string{"secrets", "secrets_kv1", "secrets_kv2", "tokens", "auth_roles"} {
		if _, ok := st.Stats[k]; ok {
			t.Fatalf("%q must be omitted without list permission: %s", k, b)
		}
	}

	// No permission on the namespace at all.
	if s, b = do("PUT", "/v1/policy/none", `{"rules":[{"path":"secret/**","capabilities":["read"]}]}`, root, ""); s != 204 {
		t.Fatal(s, b)
	}
	s, b = do("POST", "/v1/auth/token", `{"policies":["none"],"ttl":"10m"}`, root, "")
	mustOK(t, 201, s, b)
	none := jsonField(t, []byte(b), "id")
	if s, b = do("GET", "/v1/sys/namespaces/team-a/stats", "", none, ""); s != 403 {
		t.Fatalf("status = %d: %s", s, b)
	}
	if s, b = do("PUT", "/v1/sys/namespaces/team-a", `{"description":"x"}`, viewer, ""); s != 403 {
		t.Fatalf("update without write must be refused: %d %s", s, b)
	}
	if s, _ = do("GET", "/v1/sys/namespaces/team-a/stats", "", "", ""); s != 401 {
		t.Fatalf("no token: %d", s)
	}
}

// A token bound to a namespace sees only that namespace, even with a policy
// that would otherwise allow reading all of them.
func TestNamespaceStatsBoundTokenIsolation(t *testing.T) {
	ts, _, root := newTestServer(t)
	do := usageDo(t, ts.URL)
	for _, n := range []string{"team-a", "team-b"} {
		if s, b := do("POST", "/v1/sys/namespaces", `{"name":"`+n+`","description":"d-`+n+`"}`, root, ""); s != 201 {
			t.Fatal(s, b)
		}
	}
	if s, b := do("PUT", "/v1/secret/s", `{"value":"x"}`, root, "team-b"); s != 204 {
		t.Fatal(s, b)
	}
	if s, b := do("PUT", "/v1/policy/ns-admin", `{"rules":[
		{"path":"sys/namespaces/**","capabilities":["read","list","write"]},
		{"path":"sys/namespaces","capabilities":["list"]}]}`, root, "team-a"); s != 204 {
		t.Fatal(s, b)
	}
	s, b := do("POST", "/v1/auth/token", `{"policies":["ns-admin"],"ttl":"10m"}`, root, "team-a")
	mustOK(t, 201, s, b)
	ta := jsonField(t, []byte(b), "id")

	if s, b = do("GET", "/v1/sys/namespaces/team-a/stats", "", ta, ""); s != 200 {
		t.Fatalf("own stats: %d %s", s, b)
	}
	for _, path := range []string{"/v1/sys/namespaces/team-b/stats"} {
		if s, b = do("GET", path, "", ta, ""); s != 403 || strings.Contains(b, "secrets") {
			t.Fatalf("%s must be refused: %d %s", path, s, b)
		}
	}
	if s, b = do("PUT", "/v1/sys/namespaces/team-b", `{"description":"hijack"}`, ta, ""); s != 403 {
		t.Fatalf("update of another namespace must be refused: %d %s", s, b)
	}
	if s, b = do("GET", "/v1/sys/namespaces/team-b", "", root, ""); s != 200 || strings.Contains(b, "hijack") {
		t.Fatalf("team-b was modified: %d %s", s, b)
	}

	s, b = do("GET", "/v1/sys/namespaces/?list=true&stats=true", "", ta, "")
	mustOK(t, 200, s, b)
	var list struct {
		Namespaces []struct{ Name string } `json:"namespaces"`
	}
	if err := json.Unmarshal([]byte(b), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Namespaces) != 1 || list.Namespaces[0].Name != "team-a" || strings.Contains(b, "d-team-b") {
		t.Fatalf("bound token must only see its namespace: %s", b)
	}
}

func TestUsageSecrets(t *testing.T) {
	ts, _, root := newTestServer(t)
	do := usageDo(t, ts.URL)
	get := func(tok, ns string) (int, map[string]any) {
		s, b := do("GET", "/v1/sys/usage/secrets", "", tok, ns)
		var m map[string]any
		_ = json.Unmarshal([]byte(b), &m)
		return s, m
	}
	if s, m := get(root, ""); s != 200 || m["secrets"].(float64) != 0 {
		t.Fatalf("empty: %d %v", s, m)
	}
	for _, p := range []string{"a", "b"} {
		if s, b := do("PUT", "/v1/secret/"+p, `{"value":"x"}`, root, ""); s != 204 {
			t.Fatal(s, b)
		}
	}
	if s, b := do("PUT", "/v2/secret/c", `{"value":"x"}`, root, ""); s != 200 {
		t.Fatal(s, b)
	}
	// Cached for a few seconds: wait is not needed because the first call above
	// ran before the writes only for the root namespace key; use another ns.
	if s, b := do("POST", "/v1/sys/namespaces", `{"name":"team-a"}`, root, ""); s != 201 {
		t.Fatal(s, b)
	}
	if s, b := do("PUT", "/v1/secret/z", `{"value":"x"}`, root, "team-a"); s != 204 {
		t.Fatal(s, b)
	}
	s, m := get(root, "team-a")
	if s != 200 || m["secrets"].(float64) != 1 || m["secrets_kv1"].(float64) != 1 || m["secrets_kv2"].(float64) != 0 {
		t.Fatalf("team-a: %d %v", s, m)
	}

	// No list permission on secret/: refused, not zero.
	if s, b := do("PUT", "/v1/policy/noaccess", `{"rules":[{"path":"sys/health","capabilities":["read"]}]}`, root, ""); s != 204 {
		t.Fatal(s, b)
	}
	s2, b2 := do("POST", "/v1/auth/token", `{"policies":["noaccess"],"ttl":"10m"}`, root, "")
	mustOK(t, 201, s2, b2)
	if s, _ := get(jsonField(t, []byte(b2), "id"), ""); s != 403 {
		t.Fatalf("denied token got %d", s)
	}
}

func TestMountStats(t *testing.T) {
	ts, _, root := newTestServer(t)
	do := usageDo(t, ts.URL)
	for _, p := range []string{"a", "b"} {
		if s, b := do("PUT", "/v1/secret/"+p, `{"value":"x"}`, root, ""); s != 204 {
			t.Fatal(s, b)
		}
	}
	for _, v := range []string{"1", "2", "3"} {
		if s, b := do("PUT", "/v2/secret/k", `{"value":"`+v+`"}`, root, ""); s != 200 {
			t.Fatal(s, b)
		}
	}
	if s, b := do("POST", "/v2/secret/destroy/k", `{"versions":[1]}`, root, ""); s != 204 {
		t.Fatal(s, b)
	}
	if s, b := do("DELETE", "/v2/secret/k", "", root, ""); s != 204 {
		t.Fatal(s, b)
	}
	if s, b := do("POST", "/v1/sys/mounts/team-kv", `{"type":"kv","description":"custom"}`, root, ""); s != 201 {
		t.Fatal(s, b)
	}

	var tune struct {
		Stats struct {
			EngineBacked      bool   `json:"engine_backed"`
			Secrets           *int   `json:"secrets"`
			SecretsKV1        *int   `json:"secrets_kv1"`
			SecretsKV2        *int   `json:"secrets_kv2"`
			Versions          *int   `json:"versions"`
			DeletedVersions   *int   `json:"deleted_versions"`
			DestroyedVersions *int   `json:"destroyed_versions"`
			LastWriteAt       string `json:"last_write_at"`
			Leases            *int   `json:"leases"`
			Tokens            *int   `json:"tokens"`
		} `json:"stats"`
	}
	s, b := do("GET", "/v1/sys/mounts-tune/secret?stats=true", "", root, "")
	mustOK(t, 200, s, b)
	if err := json.Unmarshal([]byte(b), &tune); err != nil {
		t.Fatal(err)
	}
	st := tune.Stats
	if !st.EngineBacked || st.SecretsKV1 == nil || *st.SecretsKV1 != 2 || *st.SecretsKV2 != 1 || *st.Secrets != 3 ||
		*st.Versions != 3 || *st.DeletedVersions != 1 || *st.DestroyedVersions != 1 || st.LastWriteAt == "" {
		t.Fatalf("secret/ stats = %s", b)
	}

	// Without ?stats the tune response is unchanged.
	if s, b = do("GET", "/v1/sys/mounts-tune/secret", "", root, ""); s != 200 || strings.Contains(b, "stats") {
		t.Fatalf("plain tune: %d %s", s, b)
	}

	// A table-only mount stores nothing: the unmount impact is honestly zero.
	s, b = do("GET", "/v1/sys/mounts-tune/team-kv?stats=true", "", root, "")
	mustOK(t, 200, s, b)
	tune.Stats = st
	tune.Stats.Secrets = nil
	if err := json.Unmarshal([]byte(b), &tune); err != nil {
		t.Fatal(err)
	}
	if tune.Stats.EngineBacked || tune.Stats.Secrets == nil || *tune.Stats.Secrets != 0 || *tune.Stats.Leases != 0 || *tune.Stats.Tokens != 0 {
		t.Fatalf("custom mount stats = %s", b)
	}

	// List: stats keyed by mount path.
	s, b = do("GET", "/v1/sys/mounts?stats=true", "", root, "")
	mustOK(t, 200, s, b)
	var list struct {
		Stats map[string]map[string]any `json:"stats"`
	}
	if err := json.Unmarshal([]byte(b), &list); err != nil {
		t.Fatal(err)
	}
	if list.Stats["secret/"]["secrets"].(float64) != 3 || list.Stats["team-kv/"]["engine_backed"].(bool) {
		t.Fatalf("list stats = %s", b)
	}
	if s, b = do("GET", "/v1/sys/mounts", "", root, ""); s != 200 || strings.Contains(b, `"stats"`) {
		t.Fatalf("plain list: %d %s", s, b)
	}

	// A caller that cannot list secret/ gets the mount without counters.
	if s, b = do("PUT", "/v1/policy/mounts-ro", `{"rules":[{"path":"sys/mounts","capabilities":["read"]}]}`, root, ""); s != 204 {
		t.Fatal(s, b)
	}
	s, b = do("POST", "/v1/auth/token", `{"policies":["mounts-ro"],"ttl":"10m"}`, root, "")
	mustOK(t, 201, s, b)
	ro := jsonField(t, []byte(b), "id")
	s, b = do("GET", "/v1/sys/mounts-tune/secret?stats=true", "", ro, "")
	mustOK(t, 200, s, b)
	if strings.Contains(b, `"secrets"`) || strings.Contains(b, `"versions"`) {
		t.Fatalf("counters leaked to a caller without list permission: %s", b)
	}
}
