// Package ratelimit provides a per-IP token-bucket rate limiter.
package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Limiter is a per-IP token-bucket rate limiter.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   int     // max tokens
}

type bucket struct {
	tokens    float64
	lastRefil time.Time
}

// New creates a Limiter allowing rate tokens/second with a burst of burst.
func New(rate float64, burst int) *Limiter {
	return &Limiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
	}
}

// Allow reports whether a request from the given IP should be allowed.
func (l *Limiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: float64(l.burst), lastRefil: time.Now()}
		l.buckets[ip] = b
	}
	now := time.Now()
	elapsed := now.Sub(b.lastRefil).Seconds()
	b.tokens = min(float64(l.burst), b.tokens+elapsed*l.rate)
	b.lastRefil = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Cleanup removes stale buckets older than ttl. Call periodically.
func (l *Limiter) Cleanup(ttl time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-ttl)
	for ip, b := range l.buckets {
		if b.lastRefil.Before(cutoff) {
			delete(l.buckets, ip)
		}
	}
}

// Middleware returns an http.Handler that rate-limits by client IP.
// Returns 429 Too Many Requests when the limit is exceeded.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		if !l.Allow(ip) {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
