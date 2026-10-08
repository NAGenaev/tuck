package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NAGenaev/tuck/internal/core"
	k8sauth "github.com/NAGenaev/tuck/internal/k8s"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

func TestK8sClustersReloadWithoutRestart(t *testing.T) {
	sa := &k8sauth.ReviewResult{Authenticated: true, Username: "system:serviceaccount:prod:api"}
	c := core.NewWithK8s(physical.NewInMem(), seal.NewDev(filepath.Join(t.TempDir(), "rootkey")), &mockReviewer{result: sa})
	res, err := c.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	root := res.RootToken.ID
	srv := New(c)

	// What the "config file" says right now; the reloader applies it.
	wanted := map[string]k8sauth.Reviewer{"dc1": &mockReviewer{result: sa}}
	srv.SetClusterReloader(func() ([]string, []string, error) { return c.ReplaceK8sClusters(wanted) })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(srv.Handler())
	ts.Listener = &rstListener{ln}
	ts.Start()
	t.Cleanup(ts.Close)

	reload := func(tok string) (int, []byte) {
		t.Helper()
		return doJSON(t, ts, http.MethodPost, "/v1/auth/kubernetes/clusters/reload", "", tok)
	}
	if s, b := reload(root); s != http.StatusOK || !strings.Contains(string(b), `"added":["dc1"]`) {
		t.Fatalf("first reload: %d %s", s, b)
	}
	if s, b := loginK8sCluster(t, ts.URL, "dc2", "sa"); s == http.StatusOK {
		t.Fatalf("dc2 must not exist yet: %d %s", s, b)
	}
	if s, b := doJSON(t, ts, http.MethodPut, "/v1/auth/kubernetes/clusters/dc1/role/prod/api", `{"policies":["default"]}`, root); s != http.StatusNoContent {
		t.Fatalf("role dc1: %d %s", s, b)
	}

	// The config gains dc2 and loses dc1.
	wanted = map[string]k8sauth.Reviewer{"dc2": &mockReviewer{result: sa}}
	s, b := reload(root)
	if s != http.StatusOK || !strings.Contains(string(b), `"added":["dc2"]`) || !strings.Contains(string(b), `"removed":["dc1"]`) {
		t.Fatalf("second reload: %d %s", s, b)
	}
	if s, b := doJSON(t, ts, http.MethodPut, "/v1/auth/kubernetes/clusters/dc2/role/prod/api", `{"policies":["default"]}`, root); s != http.StatusNoContent {
		t.Fatalf("role dc2 right after the reload: %d %s", s, b)
	}
	if s, b := loginK8sCluster(t, ts.URL, "dc2", "sa"); s != http.StatusOK {
		t.Errorf("dc2 login: %d %s", s, b)
	}
	if s, _ := loginK8sCluster(t, ts.URL, "dc1", "sa"); s == http.StatusOK {
		t.Error("removed cluster dc1 still accepts logins")
	}

	// A bad config keeps the current set.
	srv.SetClusterReloader(func() ([]string, []string, error) { return nil, nil, http.ErrAbortHandler })
	if s, _ := reload(root); s != http.StatusBadRequest {
		t.Errorf("failing reload: %d, want 400", s)
	}
	if s, _ := loginK8sCluster(t, ts.URL, "dc2", "sa"); s != http.StatusOK {
		t.Error("a failed reload dropped the working cluster set")
	}

	// Policy-less callers cannot trigger it.
	low := limitedToken(t, ts, root, `{"path":"secret/**","capabilities":["read"]}`)
	if s, _ := reload(low); s != http.StatusForbidden {
		t.Errorf("unprivileged reload: %d, want 403", s)
	}
}
