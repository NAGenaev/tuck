package haregister

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type patchRec struct {
	mu     sync.Mutex
	bodies []string
	auth   string
	path   string
	ctype  string
}

func (r *patchRec) handler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, string(b))
		r.auth, r.path, r.ctype = req.Header.Get("Authorization"), req.Method+" "+req.URL.Path, req.Header.Get("Content-Type")
		r.mu.Unlock()
		w.WriteHeader(status)
	})
}

func (r *patchRec) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bodies) == 0 {
		return ""
	}
	return r.bodies[len(r.bodies)-1]
}

func tokenFile(t *testing.T, v string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(p, []byte(v+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSetLeader_PatchesPodLabel(t *testing.T) {
	rec := &patchRec{}
	srv := httptest.NewServer(rec.handler(200))
	defer srv.Close()
	l, err := NewPodLabeler(srv.URL, tokenFile(t, "sa-jwt"), "", "tuck", "tuck-server-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.SetLeader(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if rec.path != "PATCH /api/v1/namespaces/tuck/pods/tuck-server-1" {
		t.Fatalf("path = %q", rec.path)
	}
	if rec.auth != "Bearer sa-jwt" || rec.ctype != "application/merge-patch+json" {
		t.Fatalf("auth=%q ctype=%q", rec.auth, rec.ctype)
	}
	if !strings.Contains(rec.last(), `"tuck.io/leader":"true"`) {
		t.Fatalf("body = %s", rec.last())
	}
}

func TestSetLeader_ErrorStatusAndMissingToken(t *testing.T) {
	srv := httptest.NewServer((&patchRec{}).handler(403))
	defer srv.Close()
	l, _ := NewPodLabeler(srv.URL, tokenFile(t, "x"), "", "ns", "p")
	if err := l.SetLeader(context.Background(), true); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want 403 error, got %v", err)
	}
	l2, _ := NewPodLabeler(srv.URL, filepath.Join(t.TempDir(), "nope"), "", "ns", "p")
	if err := l2.SetLeader(context.Background(), true); err == nil {
		t.Fatal("missing token file must be an error")
	}
}

func TestNewPodLabeler_BadCAFailsClosed(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "ca.pem")
	_ = os.WriteFile(bad, []byte("not pem"), 0o600)
	if _, err := NewPodLabeler("https://k", tokenFile(t, "x"), bad, "ns", "p"); err == nil {
		t.Fatal("want error for unusable CA")
	}
}

func TestRun_FollowsLeadershipAndClearsOnExit(t *testing.T) {
	rec := &patchRec{}
	srv := httptest.NewServer(rec.handler(200))
	defer srv.Close()
	l, _ := NewPodLabeler(srv.URL, tokenFile(t, "x"), "", "ns", "p")

	var leader atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		l.Run(ctx, leader.Load, 10*time.Millisecond, time.Hour, func(string, ...any) {})
		close(done)
	}()
	wait := func(want string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !strings.Contains(rec.last(), want) {
			if time.Now().After(deadline) {
				t.Fatalf("last patch %q, want %s", rec.last(), want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	wait(`"false"`)
	leader.Store(true)
	wait(`"true"`)
	leader.Store(false)
	wait(`"false"`)
	leader.Store(true)
	wait(`"true"`)
	cancel()
	<-done
	if !strings.Contains(rec.last(), `"false"`) {
		t.Fatalf("label not cleared on shutdown: %s", rec.last())
	}
}

func TestRun_RetriesAfterFailureAndResyncs(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	l, _ := NewPodLabeler(srv.URL, tokenFile(t, "x"), "", "ns", "p")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		l.Run(ctx, func() bool { return true }, 10*time.Millisecond, 30*time.Millisecond, func(string, ...any) {})
		close(done)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for calls.Load() < 4 { // failed attempt, retry, then at least two resyncs
		if time.Now().After(deadline) {
			t.Fatalf("only %d calls", calls.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
}

func TestInCluster_RequiresPodEnv(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	if _, err := InCluster("ns", "p"); err == nil {
		t.Fatal("want error outside a pod")
	}
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	if _, err := InCluster("", "p"); err == nil {
		t.Fatal("want error without namespace")
	}
}
