package api

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSoak runs sustained concurrent KV load against an in-process server.
// Skipped in short mode. SOAK_DURATION and SOAK_WORKERS env vars tune it.
func TestSoak(t *testing.T) {
	if testing.Short() {
		t.Skip("soak: skipped in -short mode")
	}

	duration := 10 * time.Second
	if s := os.Getenv("SOAK_DURATION"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			duration = d
		}
	}
	workers := 4
	if w := os.Getenv("SOAK_WORKERS"); w != "" {
		if n, err := strconv.Atoi(w); err == nil && n > 0 {
			workers = n
		}
	}

	ts, _, rootTok := newTestServer(t)

	goroutinesBefore := runtime.NumGoroutine()
	var ops, errs atomic.Int64

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(wid + 1)))
			for ctx.Err() == nil {
				key := fmt.Sprintf("soak/w%d/k%d", wid, r.Intn(20))
				switch r.Intn(4) {
				case 0: // put
					resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/secret/"+key, "v", rootTok))
					if err != nil {
						continue
					}
					resp.Body.Close()
					if resp.StatusCode == http.StatusNoContent {
						ops.Add(1)
					} else {
						errs.Add(1)
					}
				case 1: // get
					resp, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/secret/"+key, "", rootTok))
					if err != nil {
						continue
					}
					resp.Body.Close()
					if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
						ops.Add(1)
					} else {
						errs.Add(1)
					}
				case 2: // delete
					resp, err := http.DefaultClient.Do(authedReq(t, http.MethodDelete, ts.URL+"/v1/secret/"+key, "", rootTok))
					if err != nil {
						continue
					}
					resp.Body.Close()
					if resp.StatusCode == http.StatusNoContent {
						ops.Add(1)
					} else {
						errs.Add(1)
					}
				case 3: // list
					resp, err := http.DefaultClient.Do(authedReq(t, "LIST", ts.URL+"/v1/secret/soak/", "", rootTok))
					if err != nil {
						continue
					}
					resp.Body.Close()
					if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
						ops.Add(1)
					} else {
						errs.Add(1)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	totalOps := ops.Load()
	totalErrs := errs.Load()
	t.Logf("soak: duration=%s workers=%d ops=%d unexpected_errs=%d", duration, workers, totalOps, totalErrs)

	if totalOps == 0 {
		t.Error("soak: no operations completed")
	}
	if totalErrs > 0 {
		t.Errorf("soak: %d unexpected errors out of %d operations", totalErrs, totalOps+totalErrs)
	}

	// Goroutine leak check: allow a small margin for test cleanup.
	goroutinesAfter := runtime.NumGoroutine()
	if growth := goroutinesAfter - goroutinesBefore; growth > 20 {
		t.Errorf("soak: goroutine leak — before=%d after=%d growth=%d", goroutinesBefore, goroutinesAfter, growth)
	}
}
