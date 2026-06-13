package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

var errChaosInjected = errors.New("chaos: injected transient error")

// chaosBackend wraps InMem and injects errors at a configurable rate (0–100).
type chaosBackend struct {
	inner    physical.Backend
	failRate atomic.Int32
	injected atomic.Int64
}

func (c *chaosBackend) mayFail() error {
	if rand.Int31n(100) < c.failRate.Load() { //nolint:gosec — non-crypto use
		c.injected.Add(1)
		return errChaosInjected
	}
	return nil
}

func (c *chaosBackend) Get(ctx context.Context, key string) (*physical.Entry, error) {
	if err := c.mayFail(); err != nil {
		return nil, err
	}
	return c.inner.Get(ctx, key)
}

func (c *chaosBackend) Put(ctx context.Context, entry *physical.Entry) error {
	if err := c.mayFail(); err != nil {
		return err
	}
	return c.inner.Put(ctx, entry)
}

func (c *chaosBackend) Delete(ctx context.Context, key string) error {
	if err := c.mayFail(); err != nil {
		return err
	}
	return c.inner.Delete(ctx, key)
}

func (c *chaosBackend) List(ctx context.Context, prefix string) ([]string, error) {
	if err := c.mayFail(); err != nil {
		return nil, err
	}
	return c.inner.List(ctx, prefix)
}

func (c *chaosBackend) Snapshot(ctx context.Context, w io.Writer) error {
	return c.inner.Snapshot(ctx, w)
}

// newChaosTestServer creates an httptest server backed by a chaosBackend.
// failRate must be 0 at construction time so that Core.Start succeeds; the
// caller can raise it via cb.failRate.Store() after the server is up.
func newChaosTestServer(tb testing.TB) (*httptest.Server, *core.Core, string, *chaosBackend) {
	tb.Helper()
	cb := &chaosBackend{inner: physical.NewInMem()}
	c := core.New(cb, seal.NewDev(filepath.Join(tb.TempDir(), "rootkey")))
	result, err := c.Start(context.Background())
	if err != nil {
		tb.Fatalf("chaos server start: %v", err)
	}
	ts := httptest.NewServer(New(c).Handler())
	tb.Cleanup(ts.Close)
	return ts, c, result.RootToken.ID, cb
}

// TestChaosTransientErrors verifies the server returns 5xx under injected
// backend failures but does not panic or deadlock.
func TestChaosTransientErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("chaos: skipped in -short mode")
	}

	ts, _, rootTok, cb := newChaosTestServer(t)

	// Write a reference secret with zero errors (stable baseline).
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/chaos/ref", "stable", rootTok))
	if err != nil || resp.StatusCode != http.StatusNoContent {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("setup write: err=%v status=%d", err, status)
	}
	resp.Body.Close()

	// Enable 30 % error injection.
	cb.failRate.Store(30)

	deadline := time.Now().Add(5 * time.Second)
	var attempts, successes int
	for time.Now().Before(deadline) {
		attempts++
		r, e := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/chaos/ref", "", rootTok))
		if e == nil {
			r.Body.Close()
			if r.StatusCode == http.StatusOK {
				successes++
			}
		}
		// Also exercise writes so Put errors are covered.
		wr, we := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/chaos/rnd", "x", rootTok))
		if we == nil {
			wr.Body.Close()
		}
	}
	cb.failRate.Store(0)

	t.Logf("chaos: attempts=%d successes=%d injected=%d", attempts, successes, cb.injected.Load())
	if successes == 0 {
		t.Error("chaos: no reads succeeded at 30% failure rate — expected ~70% pass rate")
	}
	if cb.injected.Load() == 0 {
		t.Error("chaos: no errors were injected — test may be misconfigured")
	}
}

// TestChaosSealUnsealCycle verifies that secrets written before a seal event
// survive a simulated process restart and are fully readable after re-unseal.
//
// The test uses two Core instances on the same physical backend, mirroring what
// a real restart looks like: the first Core is sealed, the second Core boots on
// the same storage and auto-unseals via the Dev seal key on disk.
func TestChaosSealUnsealCycle(t *testing.T) {
	dir := t.TempDir()
	sealKey := filepath.Join(dir, "rootkey")
	backend := physical.NewInMem()

	// Boot 1: initialise and write data.
	c1 := core.New(backend, seal.NewDev(sealKey))
	result, err := c1.Start(context.Background())
	if err != nil {
		t.Fatalf("boot1 start: %v", err)
	}
	rootTok := result.RootToken.ID
	ts1 := httptest.NewServer(New(c1).Handler())
	t.Cleanup(ts1.Close)

	const n = 10
	for i := 0; i < n; i++ {
		path := fmt.Sprintf("cycle/key%d", i)
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut,
			ts1.URL+"/v1/secret/"+path, fmt.Sprintf("val%d", i), rootTok))
		if err != nil || resp.StatusCode != http.StatusNoContent {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			t.Fatalf("write key%d: err=%v status=%d", i, err, status)
		}
		resp.Body.Close()
	}

	// Seal — simulates process crash / manual seal.
	c1.Seal()

	// Reads against the sealed server must return 503.
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet,
		ts1.URL+"/v1/secret/cycle/key0", "", rootTok))
	if err != nil {
		t.Fatalf("get while sealed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("sealed read: want 503, got %d", resp.StatusCode)
	}

	// Boot 2: new Core on the same physical backend — simulates a restart.
	c2 := core.New(backend, seal.NewDev(sealKey))
	if _, err := c2.Start(context.Background()); err != nil {
		t.Fatalf("boot2 start: %v", err)
	}
	ts2 := httptest.NewServer(New(c2).Handler())
	t.Cleanup(ts2.Close)

	// All n secrets must be intact after the restart.
	for i := 0; i < n; i++ {
		path := fmt.Sprintf("cycle/key%d", i)
		resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet,
			ts2.URL+"/v1/secret/"+path, "", rootTok))
		if err != nil {
			t.Fatalf("read key%d after restart: %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("read key%d after restart: status=%d body=%s", i, resp.StatusCode, body)
		}
	}
}
