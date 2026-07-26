package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestPutDBConfig_RedactsConnectionURL verifies that a PUT response masks
// connection_url the same way GET already does — the caller just sent the
// value, so this isn't a new information leak, but an unredacted PUT
// response is easy to leak via CI logs/shell history/audit tooling that
// captures request/response pairs indiscriminately.
func TestPutDBConfig_RedactsConnectionURL(t *testing.T) {
	ts, _, root := newTestServer(t)

	body := `{"plugin_name":"postgresql","connection_url":"postgres://user:hunter2@db:5432/app","database":"app"}`
	status, resp := doJSON(t, ts, http.MethodPut, "/v1/database/config/qa-postgres", body, root)
	if status != http.StatusOK {
		t.Fatalf("put db config: %d %s", status, resp)
	}
	if strings.Contains(string(resp), "hunter2") {
		t.Errorf("PUT response leaked connection_url password: %s", resp)
	}
	if got := jsonField(t, resp, "connection_url"); got != "[redacted]" {
		t.Errorf("connection_url = %q, want [redacted]", got)
	}

	// GET must still return the real config, unaffected by the mutation
	// the handler applies to its own in-memory copy after storing.
	status, resp = doJSON(t, ts, http.MethodGet, "/v1/database/config/qa-postgres", "", root)
	if status != http.StatusOK {
		t.Fatalf("get db config: %d %s", status, resp)
	}
	if got := jsonField(t, resp, "connection_url"); got != "[redacted]" {
		t.Errorf("GET connection_url = %q, want [redacted]", got)
	}
}
