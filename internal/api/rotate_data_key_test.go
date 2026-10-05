package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRotateDataKeyEndpoint(t *testing.T) {
	ts, _, root := newTestServer(t)
	do := func(method, path, body string) (int, string) {
		t.Helper()
		resp, err := http.DefaultClient.Do(authedNsReq(t, method, ts.URL+path, body, root, ""))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if s, b := do("PUT", "/v1/secret/app/pw", `{"value":"hunter2"}`); s != 204 {
		t.Fatal(s, b)
	}
	s, b := do("POST", "/v1/sys/rotate-data-key?reencrypt=true", "")
	if s != 200 || !strings.Contains(b, `"key_version":2`) {
		t.Fatalf("rotate: %d %s", s, b)
	}
	if s, b := do("GET", "/v1/secret/app/pw", ""); s != 200 || !strings.Contains(b, "hunter2") {
		t.Fatalf("read after rotate: %d %s", s, b)
	}
}
