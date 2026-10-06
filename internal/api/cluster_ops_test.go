package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	physraft "github.com/NAGenaev/tuck/internal/physical/raft"
	"github.com/NAGenaev/tuck/internal/seal"
)

// newRaftTestServer runs the API on a bootstrapped single-node Raft backend.
func newRaftTestServer(t *testing.T) (*httptest.Server, *core.Core, string) {
	t.Helper()
	dir := t.TempDir()
	pl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := pl.Addr().String()
	_ = pl.Close()
	rb, err := physraft.Open(physraft.Config{
		NodeID:             "node-a",
		BindAddr:           addr,
		DataDir:            filepath.Join(dir, "raft"),
		Bootstrap:          true,
		HeartbeatTimeout:   300 * time.Millisecond,
		ElectionTimeout:    300 * time.Millisecond,
		LeaderLeaseTimeout: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open raft: %v", err)
	}
	t.Cleanup(func() { _ = rb.Close() })
	deadline := time.Now().Add(10 * time.Second)
	for !rb.IsLeader() {
		if time.Now().After(deadline) {
			t.Fatal("no leader")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return serveCore(t, core.New(rb, seal.NewDev(filepath.Join(dir, "rootkey"))))
}

func serveCore(t *testing.T, c *core.Core) (*httptest.Server, *core.Core, string) {
	t.Helper()
	res, err := c.Start(context.Background())
	if err != nil {
		t.Fatalf("core start: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(New(c).Handler())
	ts.Listener = &rstListener{ln}
	ts.Start()
	t.Cleanup(ts.Close)
	return ts, c, res.RootToken.ID
}

func clusterJSON(t *testing.T, method, url, body, tok string) (int, map[string]any, []byte) {
	t.Helper()
	resp, err := http.DefaultClient.Do(authedReq(t, method, url, body, tok))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return resp.StatusCode, m, raw
}

func limitedToken(t *testing.T, ts *httptest.Server, rootTok, rules string) string {
	t.Helper()
	if code, _, raw := clusterJSON(t, http.MethodPut, ts.URL+"/v1/policy/p", `{"rules":[`+rules+`]}`, rootTok); code >= 300 {
		t.Fatalf("policy: %d %s", code, raw)
	}
	code, m, raw := clusterJSON(t, http.MethodPost, ts.URL+"/v1/auth/token", `{"display_name":"x","policies":["p"]}`, rootTok)
	if code >= 300 {
		t.Fatalf("token: %d %s", code, raw)
	}
	return m["id"].(string)
}

func TestClusterStatusReplicationFields(t *testing.T) {
	ts, _, tok := newRaftTestServer(t)
	code, m, raw := clusterJSON(t, http.MethodGet, ts.URL+"/v1/sys/cluster", "", tok)
	if code != http.StatusOK {
		t.Fatalf("status %d %s", code, raw)
	}
	for _, k := range []string{"node_id", "advertise_addr", "term", "commit_index", "applied_index", "last_log_index", "last_snapshot_index", "heartbeat_timeout_ms", "last_contact_ms"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %q in %s", k, raw)
		}
	}
	if m["node_id"] != "node-a" || m["is_leader"] != true {
		t.Fatalf("unexpected status %s", raw)
	}
	if _, has := m["tls"]; has {
		t.Fatal("plain TCP transport must not report a tls block")
	}
	srv := m["servers"].([]any)[0].(map[string]any)
	met, _ := srv["metrics"].(map[string]any)
	if srv["self"] != true || met == nil || met["source"] != "local" {
		t.Fatalf("self server entry: %v", srv)
	}
}

func TestClusterSnapshotsAPI(t *testing.T) {
	ts, _, tok := newRaftTestServer(t)

	code, m, raw := clusterJSON(t, http.MethodGet, ts.URL+"/v1/sys/cluster/snapshot", "", tok)
	if code != http.StatusOK || len(m["snapshots"].([]any)) != 0 {
		t.Fatalf("empty list: %d %s", code, raw)
	}
	// Write something so there is data to snapshot.
	if code, _, raw := clusterJSON(t, http.MethodPut, ts.URL+"/v1/secret/snap/x", "hunter2", tok); code != http.StatusNoContent {
		t.Fatalf("put: %d %s", code, raw)
	}

	code, m, raw = clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/snapshot", "", tok)
	if code != http.StatusCreated || m["created"] != true {
		t.Fatalf("create: %d %s", code, raw)
	}
	snap := m["snapshot"].(map[string]any)
	id := snap["id"].(string)
	if snap["index"].(float64) == 0 || snap["size"].(float64) == 0 || snap["created_at"] == "" {
		t.Fatalf("metadata: %v", snap)
	}

	// Nothing changed: reported, not duplicated.
	code, m, raw = clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/snapshot", "", tok)
	if code != http.StatusOK || m["created"] != false || m["snapshot"].(map[string]any)["id"] != id {
		t.Fatalf("second create: %d %s", code, raw)
	}

	code, m, _ = clusterJSON(t, http.MethodGet, ts.URL+"/v1/sys/cluster/snapshot", "", tok)
	if code != http.StatusOK || len(m["snapshots"].([]any)) != 1 {
		t.Fatalf("list: %d %v", code, m)
	}

	// Download: the encrypted FSM dump - the secret value must not appear in clear.
	code, _, body := clusterJSON(t, http.MethodGet, ts.URL+"/v1/sys/cluster/snapshot/"+id, "", tok)
	if code != http.StatusOK || len(body) == 0 {
		t.Fatalf("download: %d len=%d", code, len(body))
	}
	if strings.Contains(string(body), "hunter2") {
		t.Fatal("snapshot contains a plaintext secret")
	}
	var dump map[string][]byte
	if err := json.Unmarshal(body, &dump); err != nil || len(dump) == 0 {
		t.Fatalf("snapshot is not an FSM dump: %v", err)
	}

	for _, bad := range []string{"nope", "..%2Fraft.db"} {
		if code, _, _ := clusterJSON(t, http.MethodGet, ts.URL+"/v1/sys/cluster/snapshot/"+bad, "", tok); code != http.StatusNotFound {
			t.Fatalf("download %q: %d, want 404", bad, code)
		}
	}
}

func TestClusterOpsPermissions(t *testing.T) {
	ts, _, root := newRaftTestServer(t)
	// A token allowed to read the cluster status only.
	ro := limitedToken(t, ts, root, `{"path":"sys/cluster","capabilities":["read"]}`)

	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/v1/sys/cluster/step-down", `{}`},
		{http.MethodGet, "/v1/sys/cluster/snapshot", ""},
		{http.MethodPost, "/v1/sys/cluster/snapshot", ""},
		{http.MethodGet, "/v1/sys/cluster/snapshot/1-2-3", ""},
	} {
		if code, _, raw := clusterJSON(t, c.method, ts.URL+c.path, c.body, ro); code != http.StatusForbidden {
			t.Errorf("%s %s: %d %s, want 403", c.method, c.path, code, raw)
		}
		if code, _, _ := clusterJSON(t, c.method, ts.URL+c.path, c.body, ""); code != http.StatusUnauthorized {
			t.Errorf("%s %s without token: %d, want 401", c.method, c.path, code)
		}
	}
	if code, _, _ := clusterJSON(t, http.MethodGet, ts.URL+"/v1/sys/cluster", "", ro); code != http.StatusOK {
		t.Fatalf("read-only status: %d", code)
	}
}

func TestClusterOpsSealed(t *testing.T) {
	ts, c, tok := newRaftTestServer(t)
	c.Seal()
	for _, p := range []struct{ method, path string }{
		{http.MethodPost, "/v1/sys/cluster/step-down"},
		{http.MethodGet, "/v1/sys/cluster/snapshot"},
		{http.MethodPost, "/v1/sys/cluster/snapshot"},
	} {
		if code, _, _ := clusterJSON(t, p.method, ts.URL+p.path, "", tok); code != http.StatusServiceUnavailable {
			t.Errorf("%s %s sealed: %d, want 503", p.method, p.path, code)
		}
	}
}

func TestClusterStepDownSingleNode(t *testing.T) {
	ts, _, tok := newRaftTestServer(t)
	code, m, raw := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/step-down", ``, tok)
	if code != http.StatusConflict || !strings.Contains(m["error"].(string), "no other voter") {
		t.Fatalf("single node step-down: %d %s", code, raw)
	}
}

func TestClusterOpsNotInClusterMode(t *testing.T) {
	ts, _, tok := newTestServer(t)
	for _, p := range []struct{ method, path string }{
		{http.MethodPost, "/v1/sys/cluster/step-down"},
		{http.MethodGet, "/v1/sys/cluster/snapshot"},
		{http.MethodPost, "/v1/sys/cluster/snapshot"},
	} {
		if code, _, _ := clusterJSON(t, p.method, ts.URL+p.path, "", tok); code != http.StatusNotImplemented {
			t.Errorf("%s %s: %d, want 501", p.method, p.path, code)
		}
	}
}

// fakeRaft is a cluster backend whose StepDown result is scripted, to check how
// each failure is mapped to HTTP.
type fakeRaft struct {
	physical.Backend
	leader  bool
	err     error
	gotID   string
	stepped int
}

func (f *fakeRaft) IsLeader() bool                  { return f.leader }
func (f *fakeRaft) LeaderAddr() string              { return "10.0.0.9:8201" }
func (f *fakeRaft) AddVoter(string, string) error   { return nil }
func (f *fakeRaft) RemoveServer(string) error       { return nil }
func (f *fakeRaft) ClusterStatus() any              { return map[string]any{} }
func (f *fakeRaft) StepDown(id string) error        { f.stepped++; f.gotID = id; return f.err }
func (f *fakeRaft) ListSnapshots() ([]physraft.SnapshotInfo, error) { return nil, nil }
func (f *fakeRaft) CreateSnapshot() (physraft.SnapshotInfo, error) {
	return physraft.SnapshotInfo{}, errors.New("boom")
}
func (f *fakeRaft) OpenSnapshot(string) (physraft.SnapshotInfo, io.ReadCloser, error) {
	return physraft.SnapshotInfo{}, nil, physraft.ErrSnapshotNotFound
}

func TestClusterStepDownErrorMapping(t *testing.T) {
	fb := &fakeRaft{Backend: physical.NewInMem()}
	ts, _, tok := serveCore(t, core.New(fb, seal.NewDev(filepath.Join(t.TempDir(), "rootkey"))))

	// Not the leader: refused before touching the backend, with the leader address.
	code, m, raw := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/step-down", `{}`, tok)
	if code != http.StatusServiceUnavailable || m["leader_addr"] != "10.0.0.9:8201" || fb.stepped != 0 {
		t.Fatalf("follower: %d %s stepped=%d", code, raw, fb.stepped)
	}

	fb.leader = true
	cases := []struct {
		err  error
		body string
		want int
	}{
		{nil, `{"target_id":"node-b"}`, http.StatusOK},
		{physraft.ErrInvalidTransferTarget, `{"target_id":"x"}`, http.StatusBadRequest},
		{physraft.ErrNoOtherVoter, ``, http.StatusConflict},
		{physraft.ErrNotLeader, `{}`, http.StatusServiceUnavailable},
		{errors.New("leadership transfer timed out"), `{}`, http.StatusConflict},
	}
	for _, c := range cases {
		fb.err = c.err
		code, m, raw := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/step-down", c.body, tok)
		if code != c.want {
			t.Errorf("err=%v: %d %s, want %d", c.err, code, raw, c.want)
		}
		if c.err == nil && (m["status"] != "transferred" || fb.gotID != "node-b") {
			t.Errorf("success body %s target=%q", raw, fb.gotID)
		}
	}
	if code, _, _ := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/step-down", `{not json`, tok); code != http.StatusBadRequest {
		t.Errorf("bad json: %d", code)
	}
	if code, _, _ := clusterJSON(t, http.MethodPost, ts.URL+"/v1/sys/cluster/snapshot", ``, tok); code != http.StatusInternalServerError {
		t.Errorf("snapshot failure: %d", code)
	}
	if code, _, _ := clusterJSON(t, http.MethodGet, ts.URL+"/v1/sys/cluster/snapshot/zzz", ``, tok); code != http.StatusNotFound {
		t.Errorf("unknown snapshot: %d", code)
	}
}
