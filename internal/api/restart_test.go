package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

// The process restarts on the same bbolt file and dev seal key: data written
// before the restart, the root token and the cipher suite must all survive,
// and the second start must not re-initialise the store.
func TestRestartKeepsDataAndTokens(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tuck.db")
	keyPath := filepath.Join(dir, "rootkey")

	boot := func() (*httptest.Server, *core.Core, *core.StartResult, *physical.Bolt) {
		be, err := physical.OpenBolt(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		c := core.New(be, seal.NewDev(keyPath))
		res, err := c.Start(ctx)
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		return httptest.NewServer(New(c).Handler()), c, res, be
	}

	ts1, _, first, be1 := boot()
	if first == nil || first.RootToken == nil {
		t.Fatal("first start must return a root token")
	}
	root := first.RootToken.ID
	if code, b := doJSON(t, ts1, http.MethodPut, "/v1/secret/prod/db", `{"value":"hunter2"}`, root); code != http.StatusNoContent && code != http.StatusOK {
		t.Fatalf("write: %d %s", code, b)
	}
	ts1.Close()
	if err := be1.Close(); err != nil {
		t.Fatal(err)
	}

	ts2, c2, second, be2 := boot()
	defer ts2.Close()
	defer func() { _ = be2.Close() }()
	if second != nil && second.RootToken != nil {
		t.Fatal("second start must not issue a new root token")
	}
	if c2.Sealed() {
		t.Fatal("dev seal must auto-unseal after restart")
	}
	code, b := doJSON(t, ts2, http.MethodGet, "/v1/secret/prod/db", "", root)
	if code != http.StatusOK {
		t.Fatalf("read after restart: %d %s", code, b)
	}
	if !strings.Contains(string(b), "hunter2") {
		t.Fatalf("value lost across restart: %s", b)
	}
	if code, _ := doJSON(t, ts2, http.MethodGet, "/v1/secret/prod/db", "", "not-a-token"); code != http.StatusForbidden && code != http.StatusUnauthorized {
		t.Fatalf("bad token after restart: %d", code)
	}
}
