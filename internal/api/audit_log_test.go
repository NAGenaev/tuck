package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/audit"
	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

type alClient struct {
	t    *testing.T
	base string
}

func (c alClient) do(method, path, body, tok, ns string) (int, string) {
	c.t.Helper()
	resp, err := http.DefaultClient.Do(authedNsReq(c.t, method, c.base+path, body, tok, ns))
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

type alEvents struct {
	Events []audit.Entry `json:"events"`
	Match  int           `json:"matched"`
	Trunc  bool          `json:"truncated"`
	Window audit.Window  `json:"window"`
	Head   string        `json:"head_hash"`
	Seq    uint64        `json:"head_seq"`
}

// events polls /events until at least min records match (the audit entry of a
// request is written after its response is sent).
func (c alClient) events(query, tok string, min int) alEvents {
	c.t.Helper()
	var out alEvents
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, body := c.do("GET", "/v1/sys/audit/events?"+query, "", tok, "")
		if st != 200 {
			c.t.Fatalf("events?%s = %d %s", query, st, body)
		}
		out = alEvents{}
		if err := json.Unmarshal([]byte(body), &out); err != nil {
			c.t.Fatal(err)
		}
		if len(out.Events) >= min || time.Now().After(deadline) {
			return out
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAuditEventsEndpointAndFilters(t *testing.T) {
	ts, _, root := newTestServer(t)
	c := alClient{t, ts.URL}

	c.do("PUT", "/v1/secret/alog/one", `{"value":"S3CRET-VALUE"}`, root, "")
	c.do("GET", "/v1/secret/alog/one", "", root, "")
	c.do("GET", "/v1/secret/alog/missing", "", root, "")

	all := c.events("path=secret/alog", root, 3)
	if len(all.Events) != 3 {
		t.Fatalf("got %d events: %+v", len(all.Events), all.Events)
	}
	// Newest first; one chain with consecutive sequence numbers.
	for i := 1; i < len(all.Events); i++ {
		if all.Events[i-1].Seq <= all.Events[i].Seq {
			t.Fatalf("not newest-first: %+v", all.Events)
		}
	}
	fp := audit.Fingerprint(root)
	for _, e := range all.Events {
		if e.Fingerprint != fp || e.Hash == "" || e.PrevHash == "" || e.Operation == "" {
			t.Fatalf("incomplete record: %+v", e)
		}
	}
	if all.Head == "" || all.Seq == 0 || all.Window.Size == 0 {
		t.Fatalf("missing window/head: %+v", all)
	}

	// Nothing secret in the payload: neither the value nor the token ID.
	_, raw := c.do("GET", "/v1/sys/audit/events?limit=1000", "", root, "")
	if strings.Contains(raw, "S3CRET-VALUE") || strings.Contains(raw, root) {
		t.Fatal("audit events leak a secret value or a token ID")
	}

	if e := c.events("path=secret/alog&status=404", root, 1); len(e.Events) != 1 || e.Events[0].Status != 404 {
		t.Fatalf("status filter: %+v", e.Events)
	}
	if e := c.events("path=secret/alog&status=4xx", root, 1); len(e.Events) != 1 {
		t.Fatalf("status class filter: %+v", e.Events)
	}
	if e := c.events("path=secret/alog&operation=write", root, 1); len(e.Events) != 1 || e.Events[0].Method != "PUT" {
		t.Fatalf("operation filter: %+v", e.Events)
	}
	if e := c.events("path=secret/alog&limit=1", root, 1); len(e.Events) != 1 || !e.Trunc || e.Match != 3 {
		t.Fatalf("limit: %+v", e)
	}
	if e := c.events("path=secret/alog&since=1h", root, 3); len(e.Events) != 3 {
		t.Fatalf("since duration: %+v", e.Events)
	}
	if e := c.events("path=secret/alog&since="+time.Now().Add(time.Hour).UTC().Format(time.RFC3339), root, 0); len(e.Events) != 0 {
		t.Fatalf("future since must match nothing: %+v", e.Events)
	}
	if e := c.events("q="+fp, root, 3); len(e.Events) < 3 {
		t.Fatalf("free text on fingerprint: %+v", e.Events)
	}
	after := all.Events[0].Seq
	if e := c.events("path=secret/alog&after_seq="+itoa(after), root, 0); len(e.Events) != 0 {
		t.Fatalf("after_seq: %+v", e.Events)
	}

	for _, q := range []string{"status=abc", "operation=nuke", "limit=0", "limit=x", "since=yesterday", "after_seq=-1"} {
		if st, body := c.do("GET", "/v1/sys/audit/events?"+q, "", root, ""); st != 400 {
			t.Errorf("events?%s = %d %s, want 400", q, st, body)
		}
	}
}

func itoa(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestAuditEventsCarryNamespace(t *testing.T) {
	ts, _, root := newTestServer(t)
	c := alClient{t, ts.URL}
	if st, b := c.do("POST", "/v1/sys/namespaces", `{"name":"team-a"}`, root, ""); st != 201 {
		t.Fatal(st, b)
	}
	c.do("PUT", "/v1/secret/ns/one", `{"value":"x"}`, root, "team-a")
	e := c.events("namespace=team-a&path=secret/ns", root, 1)
	if len(e.Events) != 1 || e.Events[0].Namespace != "team-a" {
		t.Fatalf("namespace not recorded: %+v", e.Events)
	}
	// The header alone must not be trusted: an unknown namespace falls back to
	// root rather than being recorded as "ghost" (and 404s there, since the
	// secret lives in team-a).
	c.do("GET", "/v1/secret/ns/one", "", root, "ghost")
	if e := c.events("namespace=ghost", root, 0); len(e.Events) != 0 {
		t.Fatalf("unvalidated namespace recorded verbatim: %+v", e.Events)
	}
	e2 := c.events("namespace=root&path=secret/ns", root, 1)
	if len(e2.Events) != 1 || e2.Events[0].Namespace != "" || e2.Events[0].Status != 404 {
		t.Fatalf("unvalidated namespace must fall back to root: %+v", e2.Events)
	}
}

func TestAuditEndpointsAccessControl(t *testing.T) {
	ts, _, root := newTestServer(t)
	c := alClient{t, ts.URL}

	endpoints := []struct{ method, path string }{
		{"GET", "/v1/sys/audit/events"},
		{"GET", "/v1/sys/audit/events/stream"},
		{"GET", "/v1/sys/audit/stats"},
		{"POST", "/v1/sys/audit/verify"},
	}
	for _, ep := range endpoints {
		if st, _ := c.do(ep.method, ep.path, "", "", ""); st != 401 {
			t.Errorf("%s %s without token = %d, want 401", ep.method, ep.path, st)
		}
	}

	// A token without any sys/audit rule.
	if st, b := c.do("PUT", "/v1/policy/reader", `{"rules":[{"path":"secret/**","capabilities":["read"]}]}`, root, ""); st != 204 {
		t.Fatal(st, b)
	}
	_, b := c.do("POST", "/v1/auth/token", `{"policies":["reader"],"ttl":"10m"}`, root, "")
	reader := jsonField(t, []byte(b), "id")
	for _, ep := range endpoints {
		if st, _ := c.do(ep.method, ep.path, "", reader, ""); st != 403 {
			t.Errorf("%s %s with a plain token = %d, want 403", ep.method, ep.path, st)
		}
	}

	// A root-namespace token with an explicit audit policy is allowed.
	if st, b := c.do("PUT", "/v1/policy/auditor", `{"rules":[{"path":"sys/audit/**","capabilities":["read","list"]}]}`, root, ""); st != 204 {
		t.Fatal(st, b)
	}
	_, b = c.do("POST", "/v1/auth/token", `{"policies":["auditor"],"ttl":"10m"}`, root, "")
	auditor := jsonField(t, []byte(b), "id")
	for _, p := range []string{"/v1/sys/audit/events", "/v1/sys/audit/stats"} {
		if st, body := c.do("GET", p, "", auditor, ""); st != 200 {
			t.Errorf("GET %s with an auditor token = %d %s", p, st, body)
		}
	}

	// A namespace-bound token must never read the cross-namespace log, even
	// with a matching policy inside its own namespace.
	if st, b := c.do("POST", "/v1/sys/namespaces", `{"name":"team-a"}`, root, ""); st != 201 {
		t.Fatal(st, b)
	}
	if st, b := c.do("PUT", "/v1/policy/auditor", `{"rules":[{"path":"sys/audit/**","capabilities":["read","list"]}]}`, root, "team-a"); st != 204 {
		t.Fatal(st, b)
	}
	st, b2 := c.do("POST", "/v1/auth/token", `{"policies":["auditor"],"ttl":"10m"}`, root, "team-a")
	if st != 201 {
		t.Fatal(st, b2)
	}
	nsTok := jsonField(t, []byte(b2), "id")
	for _, ep := range endpoints {
		if st, body := c.do(ep.method, ep.path, "", nsTok, "team-a"); st != 403 {
			t.Errorf("%s %s with a namespaced token = %d %s, want 403", ep.method, ep.path, st, body)
		}
	}
}

func TestAuditStatsEndpoint(t *testing.T) {
	ts, _, root := newTestServer(t)
	c := alClient{t, ts.URL}
	for i := 0; i < 4; i++ {
		c.do("GET", "/v1/secret/stats/x", "", root, "")
	}
	c.do("PUT", "/v1/secret/stats/x", `{"value":"v"}`, root, "")

	type statsResp struct {
		Stats  audit.StatsSnapshot `json:"stats"`
		Window audit.Window        `json:"window"`
		Seq    uint64              `json:"head_seq"`
	}
	var sr statsResp
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, body := c.do("GET", "/v1/sys/audit/stats?minutes=5", "", root, "")
		if st != 200 {
			t.Fatalf("stats = %d %s", st, body)
		}
		sr = statsResp{}
		if err := json.Unmarshal([]byte(body), &sr); err != nil {
			t.Fatal(err)
		}
		if sr.Stats.Today >= 5 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	s := sr.Stats
	if s.Today < 5 || s.LastMinute < 5 || len(s.PerMinute) != 5 || s.WindowMinute != 5 {
		t.Fatalf("stats = %+v", s)
	}
	if s.ByOperation["read"] < 4 || s.ByOperation["write"] < 1 {
		t.Fatalf("by operation = %v", s.ByOperation)
	}
	var sum int64
	for _, m := range s.PerMinute {
		sum += m.Count
	}
	if sum < 5 {
		t.Fatalf("histogram sums to %d", sum)
	}

	if st, _ := c.do("GET", "/v1/sys/audit/stats?minutes=0", "", root, ""); st != 400 {
		t.Fatalf("minutes=0 = %d", st)
	}
	_, body := c.do("GET", "/v1/sys/audit/stats?minutes=999999", "", root, "")
	var big statsResp
	_ = json.Unmarshal([]byte(body), &big)
	if len(big.Stats.PerMinute) != 1440 {
		t.Fatalf("minutes must clamp to 1440, got %d", len(big.Stats.PerMinute))
	}
}

func TestAuditVerifyEndpoint(t *testing.T) {
	ts, c0, root := newTestServer(t)
	c := alClient{t, ts.URL}
	for i := 0; i < 3; i++ {
		c.do("GET", "/v1/secret/v/x", "", root, "")
	}

	type vres struct {
		OK     bool               `json:"ok"`
		Memory *audit.VerifyResult `json:"memory"`
		File   *audit.VerifyResult `json:"file"`
		FErr   string             `json:"file_error"`
		FSrc   string             `json:"file_source"`
	}
	verify := func(body string) (int, vres) {
		st, b := c.do("POST", "/v1/sys/audit/verify", body, root, "")
		var v vres
		_ = json.Unmarshal([]byte(b), &v)
		return st, v
	}

	st, v := verify("")
	if st != 200 || !v.OK || v.Memory == nil || !v.Memory.OK || v.Memory.Checked < 3 {
		t.Fatalf("memory verify: %d %+v", st, v)
	}
	if v.FErr == "" {
		t.Fatal("no primary file is configured: file_error must say so")
	}
	if st, _ := verify(`{"source":"file"}`); st != 400 {
		t.Fatalf("file-only without primary = %d, want 400", st)
	}
	if st, _ := verify(`{"source":"bogus"}`); st != 400 {
		t.Fatalf("bogus source = %d, want 400", st)
	}
	if st, _ := verify(`{"source":"file","sink":"ghost"}`); st != 404 {
		t.Fatalf("unknown sink = %d, want 404", st)
	}
	if st, _ := verify(`{not json`); st != 400 {
		t.Fatalf("bad JSON = %d, want 400", st)
	}

	// A file sink keeps the same chain on disk; tampering with it is detected.
	path := filepath.Join(t.TempDir(), "sink.log")
	pj, _ := json.Marshal(map[string]any{"path": path})
	if st, b := c.do("PUT", "/v1/sys/audit/file/f1", string(pj), root, ""); st != 204 {
		t.Fatal(st, b)
	}
	for i := 0; i < 3; i++ {
		c.do("GET", "/v1/secret/v/y", "", root, "")
	}
	alWaitFor(t, "file sink writes", func() bool { return c0.AuditSinkStats()["f1"].Sent >= 3 })

	st, v = verify(`{"source":"file","sink":"f1"}`)
	if st != 200 || !v.OK || v.File == nil || v.File.Checked < 3 {
		t.Fatalf("file verify: %d %+v", st, v)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = []byte(strings.Replace(string(b), `"status":404`, `"status":200`, 1))
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	st, v = verify(`{"source":"file","sink":"f1"}`)
	if st != 200 || v.OK || v.File == nil || v.File.OK || v.File.FirstBroken < 0 {
		t.Fatalf("tampered file must fail verification: %d %+v %+v", st, v, v.File)
	}
	// ...while the in-memory window is still intact.
	if st, v := verify(`{"source":"memory"}`); st != 200 || !v.OK {
		t.Fatalf("memory verify: %d %+v", st, v)
	}

	// Release the file handle before TempDir cleanup removes it (Windows locks
	// open files).
	c0.Dispatcher().Close()
}

func alWaitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestAuditSinkListStatsStateAndRedaction(t *testing.T) {
	ts, c0, root := newTestServer(t)
	c := alClient{t, ts.URL}

	path := filepath.Join(t.TempDir(), "sink.log")
	// Release file handles before TempDir's own cleanup removes them (Windows
	// locks open files); registered after TempDir so it runs first (LIFO).
	t.Cleanup(c0.Dispatcher().Close)
	pj, _ := json.Marshal(map[string]any{"path": path})
	if st, b := c.do("PUT", "/v1/sys/audit/file/f1", string(pj), root, ""); st != 204 {
		t.Fatal(st, b)
	}
	if st, b := c.do("PUT", "/v1/sys/audit/webhook/w1", `{"url":"http://user:pw-secret@127.0.0.1:1/hook?token=zzz-secret","timeout_sec":1}`, root, ""); st != 204 {
		t.Fatal(st, b)
	}
	// The same path cannot back two sinks.
	if st, b := c.do("PUT", "/v1/sys/audit/file/f2", string(pj), root, ""); st != 409 {
		t.Fatalf("duplicate path = %d %s, want 409", st, b)
	}

	for i := 0; i < 3; i++ {
		c.do("GET", "/v1/secret/s/x", "", root, "")
	}
	alWaitFor(t, "file sink delivery", func() bool { return c0.AuditSinkStats()["f1"].Sent >= 3 })
	alWaitFor(t, "webhook failure", func() bool { return c0.AuditSinkStats()["w1"].Errors >= 1 })

	type sink struct {
		Name    string             `json:"name"`
		Type    string             `json:"type"`
		Options map[string]string  `json:"options"`
		Enabled bool               `json:"enabled"`
		Active  bool               `json:"active"`
		Health  string             `json:"health"`
		Stats   *audit.SinkStats   `json:"stats"`
		Errors  int                `json:"errors"`
	}
	type listResp struct {
		Sinks []sink          `json:"sinks"`
		Caps  map[string]bool `json:"capabilities"`
	}
	list := func() (listResp, string) {
		st, body := c.do("LIST", "/v1/sys/audit/", "", root, "")
		if st != 200 {
			t.Fatalf("list = %d %s", st, body)
		}
		var l listResp
		if err := json.Unmarshal([]byte(body), &l); err != nil {
			t.Fatal(err)
		}
		return l, body
	}
	find := func(l listResp, name string) sink {
		for _, s := range l.Sinks {
			if s.Name == name {
				return s
			}
		}
		t.Fatalf("sink %q not listed: %+v", name, l)
		return sink{}
	}

	l, raw := list()
	if strings.Contains(raw, "pw-secret") || strings.Contains(raw, "zzz-secret") {
		t.Fatalf("webhook credentials leaked in list: %s", raw)
	}
	if l.Caps["syslog"] != audit.SyslogSupported || !l.Caps["pause"] {
		t.Fatalf("capabilities = %v", l.Caps)
	}
	f1 := find(l, "f1")
	if !f1.Enabled || !f1.Active || f1.Health != "ok" || f1.Stats == nil || f1.Stats.Sent < 3 || f1.Stats.Bytes <= 0 || f1.Stats.LastSuccessAt == "" {
		t.Fatalf("file sink stats: %+v %+v", f1, f1.Stats)
	}
	w1 := find(l, "w1")
	if w1.Health != "failing" || w1.Stats == nil || w1.Stats.Errors < 1 || w1.Stats.LastError == "" || w1.Stats.LastErrorAt == "" || w1.Errors < 1 {
		t.Fatalf("webhook sink stats: %+v %+v", w1, w1.Stats)
	}
	if strings.Contains(w1.Stats.LastError, "pw-secret") || strings.Contains(w1.Stats.LastError, "zzz-secret") {
		t.Fatalf("last_error leaks credentials: %s", w1.Stats.LastError)
	}
	if !strings.Contains(w1.Options["url"], "127.0.0.1:1/hook") || strings.Contains(w1.Options["url"], "pw-secret") {
		t.Fatalf("redacted url = %q", w1.Options["url"])
	}

	// Pause the file sink: persisted, honoured by the dispatcher, nothing delivered.
	if st, b := c.do("PUT", "/v1/sys/audit/state/f1", `{"enabled":false}`, root, ""); st != 204 {
		t.Fatal(st, b)
	}
	sentBefore := c0.AuditSinkStats()["f1"].Sent
	for i := 0; i < 3; i++ {
		c.do("GET", "/v1/secret/s/x", "", root, "")
	}
	alWaitFor(t, "skipped records", func() bool { return c0.AuditSinkStats()["f1"].Skipped >= 3 })
	f1 = find(func() listResp { l, _ := list(); return l }(), "f1")
	if f1.Enabled || f1.Health != "paused" || f1.Stats == nil || !f1.Stats.Paused || f1.Stats.Sent != sentBefore {
		t.Fatalf("paused sink: %+v %+v (sent before %d)", f1, f1.Stats, sentBefore)
	}

	// Survives a reload from the barrier, and a re-PUT keeps the pause.
	if err := c0.LoadAuditSinks(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := c0.AuditSinkStats()["f1"]; !st.Paused {
		t.Fatal("pause was not persisted")
	}
	pj2, _ := json.Marshal(map[string]any{"path": path, "max_size_mb": 5})
	if st, b := c.do("PUT", "/v1/sys/audit/file/f1", string(pj2), root, ""); st != 204 {
		t.Fatal(st, b)
	}
	if !c0.AuditSinkStats()["f1"].Paused {
		t.Fatal("re-registering must keep the sink paused")
	}

	// Resume.
	if st, b := c.do("PUT", "/v1/sys/audit/state/f1", `{"enabled":true}`, root, ""); st != 204 {
		t.Fatal(st, b)
	}
	c.do("GET", "/v1/secret/s/x", "", root, "")
	alWaitFor(t, "delivery after resume", func() bool { return c0.AuditSinkStats()["f1"].Sent >= 1 })
	if find(func() listResp { l, _ := list(); return l }(), "f1").Health != "ok" {
		t.Fatal("resumed sink must be ok")
	}

	// Errors.
	if st, _ := c.do("PUT", "/v1/sys/audit/state/ghost", `{"enabled":false}`, root, ""); st != 404 {
		t.Fatalf("unknown sink = %d", st)
	}
	if st, _ := c.do("PUT", "/v1/sys/audit/state/f1", `{}`, root, ""); st != 400 {
		t.Fatalf("missing enabled = %d", st)
	}
	if st, _ := c.do("PUT", "/v1/sys/audit/state/f1", `{"enabled":false}`, "", ""); st != 401 {
		t.Fatalf("no token = %d", st)
	}
}

func TestAuditSyslogSinkCapability(t *testing.T) {
	ts, _, root := newTestServer(t)
	c := alClient{t, ts.URL}

	if !audit.SyslogSupported {
		if st, _ := c.do("PUT", "/v1/sys/audit/syslog/sl", `{"network":"udp","addr":"127.0.0.1:514"}`, root, ""); st != 501 {
			t.Fatalf("unsupported platform = %d, want 501", st)
		}
		return
	}
	for _, body := range []string{
		`{"network":"icmp","addr":"127.0.0.1:514"}`,
		`{"network":"udp"}`,
		`{"network":"udp","addr":"127.0.0.1:514","facility":"nope"}`,
		`{bad`,
	} {
		if st, b := c.do("PUT", "/v1/sys/audit/syslog/sl", body, root, ""); st != 400 {
			t.Errorf("%s = %d %s, want 400", body, st, b)
		}
	}
	if st, b := c.do("PUT", "/v1/sys/audit/syslog/sl", `{"network":"udp","addr":"127.0.0.1:514","facility":"local3"}`, root, ""); st != 204 {
		t.Fatalf("valid syslog sink = %d %s", st, b)
	}
	_, body := c.do("LIST", "/v1/sys/audit/", "", root, "")
	if !strings.Contains(body, `"syslog"`) {
		t.Fatalf("syslog sink not listed: %s", body)
	}
}

// ── live tail ───────────────────────────────────────────────────────────────

type alSSE struct {
	resp *http.Response
	ch   chan [2]string // event, data
}

func alOpenStream(t *testing.T, base, query, tok string) *alSSE {
	t.Helper()
	resp, err := http.DefaultClient.Do(authedReq(t, "GET", base+"/v1/sys/audit/events/stream?"+query, "", tok))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("stream = %d %s", resp.StatusCode, b)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}
	s := &alSSE{resp: resp, ch: make(chan [2]string, 256)}
	t.Cleanup(func() { _ = resp.Body.Close() })
	go func() {
		defer close(s.ch)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		var ev string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				s.ch <- [2]string{ev, strings.TrimPrefix(line, "data: ")}
			}
		}
	}()
	return s
}

func (s *alSSE) next(t *testing.T, event string) string {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m, ok := <-s.ch:
			if !ok {
				t.Fatalf("stream closed while waiting for %q", event)
			}
			if m[0] == event {
				return m[1]
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %q", event)
		}
	}
}

func TestAuditLiveTail(t *testing.T) {
	ts, _, root := newTestServer(t)
	c := alClient{t, ts.URL}

	// Records written before connecting are not replayed without since_seq.
	c.do("GET", "/v1/secret/tail/old", "", root, "")
	c.events("path=secret/tail/old", root, 1)

	s := alOpenStream(t, ts.URL, "path=secret/tail", root)
	hello := s.next(t, "hello")
	if !strings.Contains(hello, "head_seq") {
		t.Fatalf("hello = %s", hello)
	}

	c.do("GET", "/v1/secret/other/ignored", "", root, "") // filtered out
	c.do("PUT", "/v1/secret/tail/new", `{"value":"v"}`, root, "")

	var e audit.Entry
	if err := json.Unmarshal([]byte(s.next(t, "audit")), &e); err != nil {
		t.Fatal(err)
	}
	if e.Path != "/v1/secret/tail/new" || e.Method != "PUT" || e.Hash == "" {
		t.Fatalf("streamed record = %+v", e)
	}

	// Replay from a sequence number.
	s2 := alOpenStream(t, ts.URL, "path=secret/tail&since_seq=0", root)
	s2.next(t, "hello")
	seen := map[string]bool{}
	for len(seen) < 2 {
		var e audit.Entry
		if err := json.Unmarshal([]byte(s2.next(t, "audit")), &e); err != nil {
			t.Fatal(err)
		}
		seen[e.Path] = true
	}
	if !seen["/v1/secret/tail/old"] || !seen["/v1/secret/tail/new"] {
		t.Fatalf("replay = %v", seen)
	}

	if st, _ := c.do("GET", "/v1/sys/audit/events/stream?status=bogus", "", root, ""); st != 400 {
		t.Fatalf("bad filter on stream = %d", st)
	}
	if st, _ := c.do("GET", "/v1/sys/audit/events/stream?since_seq=abc", "", root, ""); st != 400 {
		t.Fatalf("bad since_seq = %d", st)
	}
}

func TestAuditVerifyPrimaryFile(t *testing.T) {
	// The primary-file path of the dispatcher is exercised end to end: a file
	// logger as primary log exposes the file to /verify without any path input.
	path := filepath.Join(t.TempDir(), "primary.log")
	rl, err := audit.NewRotatingFileLogger(path, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rl.Close() }()

	c0 := core.New(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	// The server captures the dispatcher when it is created, so swap it first.
	c0.SetDispatcher(audit.NewDispatcher(rl))
	res, err := c0.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	root := res.RootToken.ID
	hs := httptest.NewServer(New(c0).Handler())
	defer hs.Close()
	c := alClient{t, hs.URL}

	for i := 0; i < 3; i++ {
		c.do("GET", "/v1/secret/p/x", "", root, "")
	}
	c.events("path=secret/p", root, 3)
	st, b := c.do("POST", "/v1/sys/audit/verify", `{"source":"file"}`, root, "")
	if st != 200 || !strings.Contains(b, `"ok":true`) || !strings.Contains(b, `"file_source":"primary"`) {
		t.Fatalf("primary verify = %d %s", st, b)
	}
}
