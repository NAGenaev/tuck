package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

// newBenchHandler creates an in-memory Tuck instance and returns its HTTP
// handler + root token.  Using the handler directly (no TCP) eliminates
// port exhaustion on Windows and makes measurements OS-independent.
func newBenchHandler(b *testing.B) (http.Handler, string) {
	b.Helper()
	c := core.New(physical.NewInMem(), seal.NewDev(filepath.Join(b.TempDir(), "rootkey")))
	result, err := c.Start(context.Background())
	if err != nil {
		b.Fatalf("core start: %v", err)
	}
	return New(c).Handler(), result.RootToken.ID
}

func benchReq(method, path, body, token string) *http.Request {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("X-Tuck-Token", token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// BenchmarkKVPut measures write throughput for the KV secrets engine.
func BenchmarkKVPut(b *testing.B) {
	h, tok := newBenchHandler(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := serve(h, benchReq(http.MethodPut,
			fmt.Sprintf("/v1/secret/bench/key%d", i), "value", tok))
		if w.Code != http.StatusNoContent {
			b.Fatalf("unexpected status %d", w.Code)
		}
	}
}

// BenchmarkKVGet measures read throughput after a warm-up write.
func BenchmarkKVGet(b *testing.B) {
	h, tok := newBenchHandler(b)
	serve(h, benchReq(http.MethodPut, "/v1/secret/bench/key", "value", tok))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := serve(h, benchReq(http.MethodGet, "/v1/secret/bench/key", "", tok))
		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status %d", w.Code)
		}
	}
}

// BenchmarkKVGetParallel measures concurrent read throughput.
func BenchmarkKVGetParallel(b *testing.B) {
	h, tok := newBenchHandler(b)
	serve(h, benchReq(http.MethodPut, "/v1/secret/bench/key", "value", tok))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			serve(h, benchReq(http.MethodGet, "/v1/secret/bench/key", "", tok))
		}
	})
}

// BenchmarkKVPutParallel measures concurrent write throughput.
func BenchmarkKVPutParallel(b *testing.B) {
	h, tok := newBenchHandler(b)
	var i int
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i++
			serve(h, benchReq(http.MethodPut,
				fmt.Sprintf("/v1/secret/bench/k%d", i), "v", tok))
		}
	})
}

// BenchmarkTokenCreate measures token creation latency (involves barrier crypto).
func BenchmarkTokenCreate(b *testing.B) {
	h, tok := newBenchHandler(b)
	body := `{"display_name":"bench","policies":["default"]}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := serve(h, benchReq(http.MethodPost, "/v1/auth/token", body, tok))
		if w.Code != http.StatusCreated {
			b.Fatalf("unexpected status %d", w.Code)
		}
	}
}

// BenchmarkTokenValidate measures the hot path: authenticating every API request.
func BenchmarkTokenValidate(b *testing.B) {
	h, tok := newBenchHandler(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		serve(h, benchReq(http.MethodGet, "/v1/auth/token/lookup-self", "", tok))
	}
}

// BenchmarkTokenValidateParallel measures concurrent token validation.
func BenchmarkTokenValidateParallel(b *testing.B) {
	h, tok := newBenchHandler(b)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			serve(h, benchReq(http.MethodGet, "/v1/auth/token/lookup-self", "", tok))
		}
	})
}

// BenchmarkSealStatus measures the unauthenticated health-check endpoint
// (polled frequently by load balancers and Kubernetes liveness probes).
func BenchmarkSealStatus(b *testing.B) {
	h, _ := newBenchHandler(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		serve(h, benchReq(http.MethodGet, "/v1/sys/seal-status", "", ""))
	}
}
