package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/operator"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

type fakeTuckSecretSource struct {
	list *operator.TuckSecretList
	ns   map[string]map[string]string
	err  error
}

func (f *fakeTuckSecretSource) List(_ context.Context, _ string) (*operator.TuckSecretList, error) {
	return f.list, f.err
}

func (f *fakeTuckSecretSource) ListNamespaceAnnotations(_ context.Context) (map[string]map[string]string, error) {
	return f.ns, f.err
}

func newTuckSecretServer(t *testing.T, src TuckSecretSource) (*httptest.Server, string) {
	t.Helper()
	c := core.New(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	res, err := c.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	srv := New(c)
	if src != nil {
		srv.SetTuckSecretSource(src)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, res.RootToken.ID
}

func tsItem(ns, name, secret string, synced bool, syncErr string) operator.TuckSecret {
	t := operator.TuckSecret{
		Metadata: operator.ObjectMeta{Name: name, Namespace: ns},
		Spec: operator.TuckSecretSpec{
			SecretName: secret, TuckPath: "db/password", SecretKey: "password",
			Data: map[string]string{"user": "db/user"}, RefreshInterval: "1m",
		},
	}
	if synced {
		t.Status.LastSyncTime = time.Now()
	}
	t.Status.LastSyncError = syncErr
	return t
}

func getTuckSecrets(t *testing.T, ts *httptest.Server, token, ns string) (int, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(authedNsReq(t, "GET", ts.URL+"/v1/sys/k8s/tucksecrets", "", token, ns))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestListTuckSecretsNamespaceScoping(t *testing.T) {
	src := &fakeTuckSecretSource{
		list: &operator.TuckSecretList{Items: []operator.TuckSecret{
			tsItem("app-a", "db", "db-creds", true, ""),
			tsItem("app-b", "db", "db-creds", true, ""),
			tsItem("app-plain", "x", "x", false, ""),
			tsItem("app-a", "broken", "b", true, "permission denied"),
		}},
		ns: map[string]map[string]string{
			"app-a":     {"tuck.io/tuck-namespace": "team-a"},
			"app-b":     {"tuck.io/tuck-namespace": "team-b"},
			"app-plain": {},
		},
	}
	ts, root := newTuckSecretServer(t, src)
	do := func(method, path, body, tok, ns string) (int, string) {
		resp, err := http.DefaultClient.Do(authedNsReq(t, method, ts.URL+path, body, tok, ns))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	for _, n := range []string{"team-a", "team-b"} {
		if s, b := do("POST", "/v1/sys/namespaces", `{"name":"`+n+`"}`, root, ""); s != 201 {
			t.Fatal(s, b)
		}
	}

	type resp struct {
		Items []tuckSecretView `json:"items"`
		Count int              `json:"count"`
	}
	parse := func(body string) resp {
		var r resp
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			t.Fatalf("decode %q: %v", body, err)
		}
		return r
	}

	// Root without a namespace sees everything, sorted, with resolved bindings.
	s, b := getTuckSecrets(t, ts, root, "")
	if s != 200 {
		t.Fatal(s, b)
	}
	all := parse(b)
	if all.Count != 4 || all.Items[0].K8sNamespace != "app-a" || all.Items[0].Name != "broken" {
		t.Fatalf("unexpected root listing: %s", b)
	}
	states := map[string]string{}
	for _, it := range all.Items {
		states[it.K8sNamespace+"/"+it.Name] = it.State + "|" + it.TuckNamespace
	}
	for k, want := range map[string]string{
		"app-a/db": "synced|team-a", "app-a/broken": "error|team-a",
		"app-b/db": "synced|team-b", "app-plain/x": "pending|",
	} {
		if states[k] != want {
			t.Errorf("%s = %q, want %q", k, states[k], want)
		}
	}
	if e := all.Items[1].Entries; len(e) != 2 || e[0].Key != "password" || e[0].Path != "db/password" || e[1].Key != "user" {
		t.Errorf("entries: %+v", e)
	}

	// Root narrowed to a namespace via the header sees only that namespace.
	s, b = getTuckSecrets(t, ts, root, "team-a")
	if s != 200 || parse(b).Count != 2 {
		t.Fatalf("root+team-a: %d %s", s, b)
	}

	// A token bound to team-a sees only team-a, and cannot reach team-b by header.
	if s, b := do("PUT", "/v1/policy/viewer", `{"rules":[{"path":"sys/k8s/tucksecrets","capabilities":["list"]}]}`, root, "team-a"); s != 204 {
		t.Fatal(s, b)
	}
	s, b = do("POST", "/v1/auth/token", `{"policies":["viewer"],"ttl":"10m"}`, root, "team-a")
	if s != 201 {
		t.Fatal(s, b)
	}
	ta := jsonField(t, []byte(b), "id")
	s, b = getTuckSecrets(t, ts, ta, "")
	got := parse(b)
	if s != 200 || got.Count != 2 {
		t.Fatalf("team-a token: %d %s", s, b)
	}
	for _, it := range got.Items {
		if it.TuckNamespace != "team-a" {
			t.Errorf("leaked %s/%s (tuck ns %q)", it.K8sNamespace, it.Name, it.TuckNamespace)
		}
	}
	if s, _ := getTuckSecrets(t, ts, ta, "team-b"); s != 403 {
		t.Errorf("foreign namespace header: %d, want 403", s)
	}

	// A token without the capability is refused.
	if s, b := do("PUT", "/v1/policy/nolist", `{"rules":[{"path":"secret/**","capabilities":["read"]}]}`, root, "team-a"); s != 204 {
		t.Fatal(s, b)
	}
	s, b = do("POST", "/v1/auth/token", `{"policies":["nolist"],"ttl":"10m"}`, root, "team-a")
	if s != 201 {
		t.Fatal(s, b)
	}
	if s, _ := getTuckSecrets(t, ts, jsonField(t, []byte(b), "id"), ""); s != 403 {
		t.Errorf("token without capability: %d, want 403", s)
	}
}

func TestListTuckSecretsUnavailable(t *testing.T) {
	ts, root := newTuckSecretServer(t, nil)
	s, b := getTuckSecrets(t, ts, root, "")
	if s != 503 || !strings.Contains(b, "k8s_unavailable") {
		t.Fatalf("no source: %d %s", s, b)
	}

	src := &fakeTuckSecretSource{err: fmt.Errorf("list TuckSecrets: %w", &operator.StatusError{Code: 403})}
	ts, root = newTuckSecretServer(t, src)
	s, b = getTuckSecrets(t, ts, root, "")
	if s != 503 || !strings.Contains(b, "k8s_forbidden") {
		t.Fatalf("forbidden: %d %s", s, b)
	}

	src = &fakeTuckSecretSource{err: errors.New("connection refused")}
	ts, root = newTuckSecretServer(t, src)
	s, b = getTuckSecrets(t, ts, root, "")
	if s != 503 || !strings.Contains(b, "k8s_error") || strings.Contains(b, "refused") {
		t.Fatalf("generic error must not leak details: %d %s", s, b)
	}
}

// Tuck usually runs outside the clusters it serves: the page must read every
// trusted cluster, label the rows with their cluster, and keep working when
// one cluster is down.
func TestListTuckSecretsAcrossClusters(t *testing.T) {
	healthy := func(ns, name string) *fakeTuckSecretSource {
		return &fakeTuckSecretSource{
			list: &operator.TuckSecretList{Items: []operator.TuckSecret{tsItem(ns, name, name, true, "")}},
			ns:   map[string]map[string]string{ns: {"tuck.io/tuck-namespace": "payments"}},
		}
	}
	c := core.New(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
	res, err := c.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	srv := New(c)
	srv.SetTuckSecretSources(map[string]TuckSecretSource{
		"dc1": healthy("payments", "core-db"),
		"dc2": healthy("payments", "core-db"),
		"dc3": &fakeTuckSecretSource{err: &operator.StatusError{Code: http.StatusForbidden}},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	get := func(q string) (int, string) {
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/k8s/tucksecrets"+q, "", res.RootToken.ID))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	s, b := get("")
	if s != http.StatusOK {
		t.Fatalf("one failing cluster must not fail the page: %d %s", s, b)
	}
	var out struct {
		Items    []tuckSecretView `json:"items"`
		Clusters []clusterReport  `json:"clusters"`
	}
	if err := json.Unmarshal([]byte(b), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 || out.Items[0].Cluster != "dc1" || out.Items[1].Cluster != "dc2" {
		t.Errorf("rows are not labelled per cluster: %+v", out.Items)
	}
	reports := map[string]clusterReport{}
	for _, r := range out.Clusters {
		reports[r.Name] = r
	}
	if !reports["dc1"].OK || reports["dc1"].Count != 1 || reports["dc3"].OK || reports["dc3"].Code != "k8s_forbidden" {
		t.Errorf("cluster reports: %+v", out.Clusters)
	}

	// One cluster on request; an unknown one is a 404.
	if s, b := get("?cluster=dc2"); s != http.StatusOK || strings.Count(b, `"cluster":"dc2"`) != 1 || strings.Contains(b, "dc1") {
		t.Errorf("?cluster=dc2: %d %s", s, b)
	}
	if s, _ := get("?cluster=nope"); s != http.StatusNotFound {
		t.Errorf("unknown cluster: %d", s)
	}
	// A caller inside a Tuck namespace sees its own rows but not the cluster topology.
	if r, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/sys/namespaces", `{"name":"payments"}`, res.RootToken.ID)); err != nil || r.StatusCode != http.StatusCreated {
		t.Fatalf("create namespace: %v", err)
	}
	resp, err := http.DefaultClient.Do(authedNsReq(t, "GET", ts.URL+"/v1/sys/k8s/tucksecrets", "", res.RootToken.ID, "payments"))
	if err != nil {
		t.Fatal(err)
	}
	nsBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || strings.Contains(string(nsBody), `"clusters"`) || strings.Contains(string(nsBody), "dc3") || !strings.Contains(string(nsBody), "core-db") {
		t.Errorf("namespace caller: %d %s", resp.StatusCode, nsBody)
	}
	// Only failures: the old "unavailable" semantics (and the reason) survive.
	srv.SetTuckSecretSources(map[string]TuckSecretSource{"dc3": &fakeTuckSecretSource{err: &operator.StatusError{Code: http.StatusForbidden}}})
	if s, b := get(""); s != http.StatusServiceUnavailable || !strings.Contains(b, "k8s_forbidden") {
		t.Errorf("all clusters failing: %d %s", s, b)
	}
}
