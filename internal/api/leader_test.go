package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)

type fakeHA struct {
	*physical.InMem
	leader bool
}

func (f *fakeHA) IsLeader() bool             { return f.leader }
func (f *fakeHA) LeaderAddr() string         { return "10.0.0.1:8201" }
func (f *fakeHA) AddVoter(_, _ string) error { return nil }
func (f *fakeHA) RemoveServer(string) error  { return nil }
func (f *fakeHA) ClusterStatus() any         { return nil }

func leaderStatus(t *testing.T, base string) int {
	t.Helper()
	resp, err := http.Get(base + "/v1/sys/leader")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func TestLeaderEndpoint_NonHAIsReady(t *testing.T) {
	ts, _, _ := newTestServer(t)
	if got := leaderStatus(t, ts.URL); got != http.StatusOK {
		t.Fatalf("non-HA: want 200, got %d", got)
	}
}

func TestLeaderEndpoint_LeaderVsStandby(t *testing.T) {
	for _, tc := range []struct {
		leader bool
		want   int
	}{{true, http.StatusOK}, {false, http.StatusTooManyRequests}} {
		be := &fakeHA{InMem: physical.NewInMem(), leader: true} // only a leader initializes
		c := core.New(be, seal.NewDev(filepath.Join(t.TempDir(), "rootkey")))
		if _, err := c.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		be.leader = tc.leader
		srv := httptest.NewServer(New(c).Handler())
		got := leaderStatus(t, srv.URL)
		srv.Close()
		if got != tc.want {
			t.Fatalf("leader=%v: want %d, got %d", tc.leader, tc.want, got)
		}
	}
}

func TestLeaderEndpoint_SealedIs503(t *testing.T) {
	ts, c, tok := newTestServer(t)
	_ = tok
	c.Seal()
	if got := leaderStatus(t, ts.URL); got != http.StatusServiceUnavailable {
		t.Fatalf("sealed: want 503, got %d", got)
	}
}
