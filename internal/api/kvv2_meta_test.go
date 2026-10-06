package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// kvCall performs a request and decodes a JSON object response (if any).
func kvCall(t *testing.T, ts *httptest.Server, method, path, body, tok string) (int, map[string]any, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(authedReq(t, method, ts.URL+path, body, tok))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return resp.StatusCode, m, string(raw)
}

func kvMeta(t *testing.T, ts *httptest.Server, path, tok string) map[string]any {
	t.Helper()
	code, m, raw := kvCall(t, ts, http.MethodGet, "/v2/secret/metadata/"+path, "", tok)
	if code != http.StatusOK {
		t.Fatalf("GET metadata %s: %d %s", path, code, raw)
	}
	return m
}

func kvToken(t *testing.T, ts *httptest.Server, root, name, policyName, rules string) string {
	t.Helper()
	if code, _, raw := kvCall(t, ts, http.MethodPut, "/v1/policy/"+policyName, `{"rules":[`+rules+`]}`, root); code >= 300 {
		t.Fatalf("policy: %d %s", code, raw)
	}
	code, m, raw := kvCall(t, ts, http.MethodPost, "/v1/auth/token", `{"display_name":"`+name+`","policies":["`+policyName+`"]}`, root)
	if code >= 300 {
		t.Fatalf("token: %d %s", code, raw)
	}
	return m["id"].(string)
}

func TestKVv2MetaAuthorship(t *testing.T) {
	ts, _, root := newTestServer(t)
	tok := kvToken(t, ts, root, "alice-ci", "kvw", `{"path":"secret/**","capabilities":["write","read","list"]}`)

	if code, _, raw := kvCall(t, ts, http.MethodPut, "/v2/secret/app/db", "one", tok); code != 200 {
		t.Fatalf("write: %d %s", code, raw)
	}
	if code, _, raw := kvCall(t, ts, http.MethodPut, "/v2/secret/app/db", "two", root); code != 200 {
		t.Fatalf("write: %d %s", code, raw)
	}

	m := kvMeta(t, ts, "app/db", root)
	meta := m["metadata"].(map[string]any)
	vers := meta["versions"].(map[string]any)
	v1 := vers["1"].(map[string]any)
	by, ok := v1["created_by"].(map[string]any)
	if !ok {
		t.Fatalf("v1 has no created_by: %v", v1)
	}
	if by["display_name"] != "alice-ci" || by["accessor"] == "" || by["fingerprint"] == "" {
		t.Fatalf("created_by = %v", by)
	}
	if v1["created_at"] == "" || v1["created_at"] == nil {
		t.Fatal("missing created_at")
	}
	// The raw token must not leak anywhere in the metadata.
	if raw, _ := json.Marshal(m); strings.Contains(string(raw), tok) || strings.Contains(string(raw), root) {
		t.Fatal("token id leaked in metadata")
	}
	// Root's own write is attributed to root's display name, a different accessor.
	by2 := vers["2"].(map[string]any)["created_by"].(map[string]any)
	if by2["accessor"] == by["accessor"] {
		t.Fatal("authors of v1 and v2 must differ")
	}
	if meta["oldest_version"].(float64) != 1 || meta["current_version"].(float64) != 2 {
		t.Fatalf("meta = %v", meta)
	}
}

func TestKVv2MetaCustomPutPatch(t *testing.T) {
	ts, _, root := newTestServer(t)
	kvCall(t, ts, http.MethodPut, "/v2/secret/k", "v", root)

	code, _, raw := kvCall(t, ts, http.MethodPut, "/v2/secret/metadata/k",
		`{"custom_metadata":{"owner":"team-a","ticket":"SEC-1"},"cas_required":true,"max_versions":5}`, root)
	if code != http.StatusNoContent {
		t.Fatalf("PUT: %d %s", code, raw)
	}
	meta := kvMeta(t, ts, "k", root)
	md := meta["metadata"].(map[string]any)
	cm := md["custom_metadata"].(map[string]any)
	if cm["owner"] != "team-a" || cm["ticket"] != "SEC-1" || md["cas_required"] != true || md["max_versions"].(float64) != 5 {
		t.Fatalf("metadata = %v", md)
	}
	if meta["cas_required_effective"] != true {
		t.Fatalf("effective = %v", meta["cas_required_effective"])
	}

	// PATCH merges: update one, delete one, add one.
	code, _, raw = kvCall(t, ts, http.MethodPatch, "/v2/secret/metadata/k",
		`{"custom_metadata":{"owner":"team-b","ticket":null,"env":"prod"}}`, root)
	if code != http.StatusNoContent {
		t.Fatalf("PATCH: %d %s", code, raw)
	}
	md = kvMeta(t, ts, "k", root)["metadata"].(map[string]any)
	cm = md["custom_metadata"].(map[string]any)
	if len(cm) != 2 || cm["owner"] != "team-b" || cm["env"] != "prod" {
		t.Fatalf("after PATCH: %v", cm)
	}
	if md["cas_required"] != true || md["max_versions"].(float64) != 5 {
		t.Fatalf("PATCH clobbered other fields: %v", md)
	}

	// PUT replaces the map.
	kvCall(t, ts, http.MethodPut, "/v2/secret/metadata/k", `{"custom_metadata":{"only":"one"}}`, root)
	cm = kvMeta(t, ts, "k", root)["metadata"].(map[string]any)["custom_metadata"].(map[string]any)
	if len(cm) != 1 || cm["only"] != "one" {
		t.Fatalf("after PUT: %v", cm)
	}

	// Validation.
	for name, body := range map[string]string{
		"max_versions too big": `{"max_versions":1001}`,
		"max_versions neg":     `{"max_versions":-1}`,
		"empty key":            `{"custom_metadata":{"":"x"}}`,
		"long value":           `{"custom_metadata":{"k":"` + strings.Repeat("v", 513) + `"}}`,
		"not json":             `nope`,
	} {
		if code, _, raw := kvCall(t, ts, http.MethodPut, "/v2/secret/metadata/k", body, root); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, code, raw)
		}
	}
}

func TestKVv2CASRequiredStatusCodes(t *testing.T) {
	ts, _, root := newTestServer(t)
	kvCall(t, ts, http.MethodPut, "/v2/secret/c", "v", root)
	kvCall(t, ts, http.MethodPut, "/v2/secret/metadata/c", `{"cas_required":true}`, root)

	code, m, raw := kvCall(t, ts, http.MethodPut, "/v2/secret/c", "x", root)
	if code != http.StatusBadRequest || m["cas_required"] != true {
		t.Fatalf("no cas: %d %s, want 400 with cas_required", code, raw)
	}
	if code, _, raw := kvCall(t, ts, http.MethodPut, "/v2/secret/c?cas=7", "x", root); code != http.StatusConflict {
		t.Fatalf("wrong cas: %d %s, want 409", code, raw)
	}
	if code, _, raw := kvCall(t, ts, http.MethodPut, "/v2/secret/c?cas=1", "x", root); code != http.StatusOK {
		t.Fatalf("right cas: %d %s", code, raw)
	}
}

func TestKVv2MountOptions(t *testing.T) {
	ts, _, root := newTestServer(t)

	if code, _, raw := kvCall(t, ts, http.MethodPost, "/v1/sys/mounts-tune/secret", `{"max_versions":2,"cas_required":true}`, root); code != http.StatusNoContent {
		t.Fatalf("tune: %d %s", code, raw)
	}
	if code, m, _ := kvCall(t, ts, http.MethodGet, "/v1/sys/mounts-tune/secret", "", root); code != 200 || m["cas_required"] != true || m["max_versions"].(float64) != 2 {
		t.Fatalf("tune GET: %d %v", code, m)
	}

	// cas_required at mount level.
	if code, m, _ := kvCall(t, ts, http.MethodPut, "/v2/secret/m/a", "x", root); code != http.StatusBadRequest || m["cas_required"] != true {
		t.Fatalf("mount cas_required not enforced: %d %v", code, m)
	}
	if code, _, raw := kvCall(t, ts, http.MethodPut, "/v2/secret/m/a?cas=0", "1", root); code != 200 {
		t.Fatalf("create with cas=0: %d %s", code, raw)
	}
	meta := kvMeta(t, ts, "m/a", root)
	if meta["cas_required_effective"] != true {
		t.Fatalf("effective flag: %v", meta)
	}
	if mo := meta["mount_options"].(map[string]any); mo["max_versions"].(float64) != 2 || mo["cas_required"] != true {
		t.Fatalf("mount_options = %v", mo)
	}

	// max_versions from the mount seeds new secrets; eviction is reported.
	for i := 1; i <= 4; i++ {
		kvCall(t, ts, http.MethodPut, "/v2/secret/m/a?cas="+string(rune('0'+i)), "v", root)
	}
	md := kvMeta(t, ts, "m/a", root)["metadata"].(map[string]any)
	if md["max_versions"].(float64) != 2 || md["current_version"].(float64) != 5 || md["oldest_version"].(float64) != 4 {
		t.Fatalf("eviction: %v", md)
	}
	if code, _, _ := kvCall(t, ts, http.MethodGet, "/v2/secret/m/a?version=1", "", root); code != http.StatusNotFound {
		t.Fatalf("evicted version readable: %d", code)
	}

	// Lifting the option takes effect immediately.
	kvCall(t, ts, http.MethodPost, "/v1/sys/mounts-tune/secret", `{"cas_required":false}`, root)
	if code, _, raw := kvCall(t, ts, http.MethodPut, "/v2/secret/m/a", "plain", root); code != 200 {
		t.Fatalf("after lifting: %d %s", code, raw)
	}

	// Validation and scoping.
	if code, _, _ := kvCall(t, ts, http.MethodPost, "/v1/sys/mounts-tune/secret", `{"max_versions":5000}`, root); code != http.StatusBadRequest {
		t.Fatalf("max_versions 5000: %d", code)
	}
	if code, _, _ := kvCall(t, ts, http.MethodPost, "/v1/sys/mounts", `{"type":"kv","path":"other"}`, root); code < 300 {
		if code, _, raw := kvCall(t, ts, http.MethodPost, "/v1/sys/mounts-tune/other", `{"cas_required":true}`, root); code != http.StatusBadRequest {
			t.Fatalf("kv option on other mount: %d %s, want 400", code, raw)
		}
	}
}

func TestKVv2ListDetailedAndTotals(t *testing.T) {
	ts, _, root := newTestServer(t)
	kvCall(t, ts, http.MethodPut, "/v2/secret/app/db", "TOPSECRET-1", root)
	kvCall(t, ts, http.MethodPut, "/v2/secret/app/db", "TOPSECRET-2", root)
	kvCall(t, ts, http.MethodPut, "/v2/secret/app/cache", "c", root)
	kvCall(t, ts, http.MethodPut, "/v2/secret/app/sub/leaf", "l", root)
	kvCall(t, ts, http.MethodDelete, "/v2/secret/app/cache", "", root)

	// Plain list is unchanged.
	code, m, raw := kvCall(t, ts, http.MethodGet, "/v2/secret/metadata/app?list=true", "", root)
	if code != 200 || m["entries"] != nil || m["totals"] != nil {
		t.Fatalf("plain list changed: %d %s", code, raw)
	}
	if keys := m["keys"].([]any); len(keys) != 3 {
		t.Fatalf("keys = %v", keys)
	}

	code, m, raw = kvCall(t, ts, http.MethodGet, "/v2/secret/metadata/app?list=true&detailed=true&totals=true", "", root)
	if code != 200 {
		t.Fatalf("detailed: %d %s", code, raw)
	}
	if strings.Contains(raw, "TOPSECRET") {
		t.Fatal("secret value in detailed listing")
	}
	by := map[string]map[string]any{}
	for _, e := range m["entries"].([]any) {
		em := e.(map[string]any)
		by[em["name"].(string)] = em
	}
	if db := by["db"]; db["current_version"].(float64) != 2 || db["versions"].(float64) != 2 || db["updated_at"] == nil || db["updated_by"] == "" {
		t.Fatalf("db entry: %v", db)
	}
	if by["cache"]["deleted"] != true {
		t.Fatalf("cache entry: %v", by["cache"])
	}
	if f := by["sub/"]; f["folder"] != true || f["count"].(float64) != 1 {
		t.Fatalf("folder entry: %v", f)
	}
	tot := m["totals"].(map[string]any)
	if tot["secrets"].(float64) != 3 || tot["versions"].(float64) != 4 || tot["deleted"].(float64) != 1 {
		t.Fatalf("totals: %v", tot)
	}

	// Root totals only.
	code, m, raw = kvCall(t, ts, http.MethodGet, "/v2/secret/metadata/?list=true&totals=true", "", root)
	if code != 200 || m["totals"].(map[string]any)["secrets"].(float64) != 3 || m["entries"] != nil {
		t.Fatalf("root totals: %d %s", code, raw)
	}
}

func TestKVv2ListDetailedRestrictedToken(t *testing.T) {
	ts, _, root := newTestServer(t)
	kvCall(t, ts, http.MethodPut, "/v2/secret/area/open", "1", root)
	kvCall(t, ts, http.MethodPut, "/v2/secret/area/closed", "1", root)

	// list on everything, read only on area/open.
	tok := kvToken(t, ts, root, "lister", "lst",
		`{"path":"secret/area","capabilities":["list"]},{"path":"secret/area/**","capabilities":["list"]},{"path":"secret/area/open","capabilities":["read"]}`)

	code, m, raw := kvCall(t, ts, http.MethodGet, "/v2/secret/metadata/area?list=true&detailed=true&totals=true", "", tok)
	if code != 200 {
		t.Fatalf("restricted detailed: %d %s", code, raw)
	}
	for _, e := range m["entries"].([]any) {
		em := e.(map[string]any)
		switch em["name"] {
		case "open":
			if em["current_version"].(float64) != 1 || em["restricted"] == true {
				t.Errorf("open: %v", em)
			}
		case "closed":
			if em["restricted"] != true || em["current_version"] != nil || em["updated_at"] != nil || em["updated_by"] != nil {
				t.Errorf("closed leaks metadata: %v", em)
			}
		}
	}
	if tot := m["totals"].(map[string]any); tot["secrets"].(float64) != 1 || tot["hidden"].(float64) != 1 {
		t.Fatalf("totals: %v", tot)
	}

	// A token without list gets 403 for the detailed variant too.
	nolist := kvToken(t, ts, root, "nolist", "nl", `{"path":"secret/other/**","capabilities":["read"]}`)
	if code, _, _ := kvCall(t, ts, http.MethodGet, "/v2/secret/metadata/area?list=true&detailed=true", "", nolist); code != http.StatusForbidden {
		t.Fatalf("no list capability: %d, want 403", code)
	}
	// And a write on metadata needs write capability.
	if code, _, _ := kvCall(t, ts, http.MethodPatch, "/v2/secret/metadata/area/open", `{"custom_metadata":{"a":"b"}}`, tok); code != http.StatusForbidden {
		t.Fatalf("PATCH without write: %d, want 403", code)
	}
}

func TestKVv2LegacyEntryStillReadable(t *testing.T) {
	// A secret written through the API has all new fields; one stored without
	// them (simulated through the store test) is covered in kvv2; here we check
	// that a secret without custom metadata serialises without the key at all.
	ts, _, root := newTestServer(t)
	kvCall(t, ts, http.MethodPut, "/v2/secret/plain", "v", root)
	md := kvMeta(t, ts, "plain", root)["metadata"].(map[string]any)
	if _, has := md["custom_metadata"]; has {
		t.Fatalf("empty custom_metadata should be omitted: %v", md)
	}
	if md["cas_required"] != false || md["oldest_version"].(float64) != 1 {
		t.Fatalf("defaults: %v", md)
	}
}
