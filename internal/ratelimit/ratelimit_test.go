package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLimiter_BurstThenDeny(t *testing.T) {
	// burst=2, rate=0.1: first two Allow() → true, third → false
	l := New(0.1, 2)

	if !l.Allow("1.2.3.4") {
		t.Fatal("first Allow() should return true")
	}
	if !l.Allow("1.2.3.4") {
		t.Fatal("second Allow() should return true")
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("third Allow() should return false (burst exhausted)")
	}
}

func TestLimiter_DifferentIPs(t *testing.T) {
	// Each IP gets its own bucket; "A" exhausting its burst does not affect "B".
	l := New(0.1, 1)

	if !l.Allow("10.0.0.1") {
		t.Fatal("first Allow() for IP A should return true")
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("second Allow() for IP A should return false")
	}
	if !l.Allow("10.0.0.2") {
		t.Fatal("first Allow() for IP B should return true")
	}
}

func TestTokenMiddleware_Exhausted(t *testing.T) {
	l := New(0.001, 1) // 1 burst, near-zero refill
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := l.TokenMiddleware(ok)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tuck-Token", "tok-abc")

	// First request allowed.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr.Code)
	}

	// Second request denied.
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rr2.Code)
	}
}

func TestTokenMiddleware_NoToken_Passes(t *testing.T) {
	l := New(0.001, 1)
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := l.TokenMiddleware(ok)

	// Requests without a token are not rate-limited by this middleware.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 (no token), got %d", i+1, rr.Code)
		}
	}
}

func TestMaxBodyMiddleware(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := MaxBodyMiddleware(100, ok)

	// Request within limit.
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.ContentLength = 50
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Request over limit.
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.ContentLength = 101
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rr2.Code)
	}
}
