package audit

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NAGenaev/tuck/internal/clientip"
)

func TestMiddlewareRecordsIdentityAndTrustedClientIP(t *testing.T) {
	t.Cleanup(func() { _ = clientip.SetTrusted(nil) })
	if err := clientip.SetTrusted([]string{"172.30.0.10"}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	h := Middleware(NewLogger(&buf), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetIdentity(r.Context(), "ldap:bob.dev", "entity-1")
		w.WriteHeader(http.StatusNoContent)
	}))

	r := httptest.NewRequest(http.MethodPut, "/v1/secret/db/pass", nil)
	r.RemoteAddr = "172.30.0.10:5000"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.7")
	r.Header.Set("X-Tuck-Token", "tuck_x")
	h.ServeHTTP(httptest.NewRecorder(), r)

	var e Entry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &e); err != nil {
		t.Fatal(err)
	}
	if e.Identity != "ldap:bob.dev" || e.EntityID != "entity-1" {
		t.Errorf("identity not recorded: %+v", e)
	}
	if e.ClientIP != "203.0.113.7" {
		t.Errorf("client_ip = %q, want the first untrusted hop", e.ClientIP)
	}
	if ComputeHash(e) != e.Hash {
		t.Error("hash must cover the new fields")
	}

	// A direct client cannot forge the address.
	buf.Reset()
	r2 := httptest.NewRequest(http.MethodGet, "/v1/sys/health", nil)
	r2.RemoteAddr = "198.51.100.9:1"
	r2.Header.Set("X-Forwarded-For", "203.0.113.7")
	h.ServeHTTP(httptest.NewRecorder(), r2)
	_ = json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &e)
	if e.ClientIP != "198.51.100.9" {
		t.Errorf("forged X-Forwarded-For accepted: %q", e.ClientIP)
	}
}
