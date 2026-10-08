package core_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/NAGenaev/tuck/internal/core"
	physraft "github.com/NAGenaev/tuck/internal/physical/raft"
	"github.com/NAGenaev/tuck/internal/seal"
)

// fakeKMS is a minimal Vault-Transit-compatible encrypt/decrypt endpoint.
func fakeKMS(t *testing.T) string {
	t.Helper()
	var mu sync.Mutex
	n := 0
	store := map[string]string{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/transit/encrypt/", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Plaintext string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		n++
		ct := "vault:v1:" + base64.RawURLEncoding.EncodeToString([]byte{byte(n)})
		store[ct] = req.Plaintext
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"ciphertext": ct}})
	})
	mux.HandleFunc("/v1/transit/decrypt/", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Ciphertext string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		pt, ok := store[req.Ciphertext]
		mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"plaintext": pt}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts.URL
}

type haNode struct {
	id      string
	rb      *physraft.Backend
	c       *core.Core
	wrapped string
}

func (n *haNode) newCore(kms string) *core.Core {
	return core.New(n.rb, seal.NewTransit(kms, "k", "tok", n.wrapped))
}

func openHACluster(t *testing.T, kms string) []*haNode {
	t.Helper()
	ids := []string{"n0", "n1", "n2"}
	addrs := map[string]string{}
	for _, id := range ids {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addrs[id] = l.Addr().String()
		_ = l.Close()
	}
	var nodes []*haNode
	for _, id := range ids {
		dir := t.TempDir()
		cfg := physraft.Config{
			NodeID: id, BindAddr: addrs[id], DataDir: filepath.Join(dir, "raft"),
			Bootstrap:          true,
			HeartbeatTimeout:   300 * time.Millisecond,
			ElectionTimeout:    300 * time.Millisecond,
			LeaderLeaseTimeout: 150 * time.Millisecond,
		}
		for _, o := range ids {
			cfg.Peers = append(cfg.Peers, physraft.Peer{ID: o, Addr: addrs[o]})
		}
		rb, err := physraft.Open(cfg)
		if err != nil {
			t.Fatalf("open %s: %v", id, err)
		}
		t.Cleanup(func() { _ = rb.Close() })
		n := &haNode{id: id, rb: rb, wrapped: filepath.Join(dir, "wrapped")}
		n.c = n.newCore(kms)
		nodes = append(nodes, n)
	}
	return nodes
}

func TestHA_FollowersWaitForLeaderInitAndShareSealMaterial(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-node: skipped in -short mode")
	}
	kms := fakeKMS(t)
	nodes := openHACluster(t, kms)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	inits := make(chan string, len(nodes))
	errs := make(chan error, len(nodes))
	for _, n := range nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := n.c.StartWhenReady(ctx, 50*time.Millisecond)
			if err != nil {
				errs <- err
				return
			}
			if res != nil {
				inits <- n.id
			}
		}()
	}
	wg.Wait()
	close(inits)
	close(errs)
	for err := range errs {
		t.Fatalf("start: %v", err)
	}
	var initiators []string
	for id := range inits {
		initiators = append(initiators, id)
	}
	if len(initiators) != 1 {
		t.Fatalf("exactly one node must initialize the store, got %v", initiators)
	}

	var leader *haNode
	for _, n := range nodes {
		if n.c.Sealed() {
			t.Fatalf("%s is sealed after start", n.id)
		}
		if n.rb.IsLeader() {
			leader = n
		}
	}
	if leader == nil || leader.id != initiators[0] {
		t.Fatalf("initializer %v is not the leader", initiators)
	}

	want, err := os.ReadFile(leader.wrapped)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nodes {
		got, err := os.ReadFile(n.wrapped)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: wrapped root key not replicated (err=%v)", n.id, err)
		}
	}

	// Rotation by a follower is refused before it can touch local seal material.
	for _, n := range nodes {
		if n == leader {
			continue
		}
		if _, err := n.c.RotateKey(ctx); !errors.Is(err, physraft.ErrNotLeader) {
			t.Fatalf("%s: follower RotateKey = %v, want ErrNotLeader", n.id, err)
		}
		got, _ := os.ReadFile(n.wrapped)
		if !bytes.Equal(got, want) {
			t.Fatalf("%s: follower rotation clobbered local seal material", n.id)
		}
	}

	// The leader rotates the root key; a follower restart must pick up the new
	// wrapped key from the cluster, not unseal with its stale file.
	if err := leader.c.PutSecret(ctx, "", "app/pass", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := leader.c.RotateKey(ctx); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	var follower *haNode
	for _, n := range nodes {
		if n != leader {
			follower = n
			break
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	rotated, _ := os.ReadFile(leader.wrapped)
	if bytes.Equal(rotated, want) {
		t.Fatal("rotation did not change the wrapped key")
	}
	for {
		v, ok, err := follower.c.GetSecret(ctx, "", "app/pass")
		if err == nil && ok && string(v) == "v1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("follower never saw replicated secret (ok=%v err=%v)", ok, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	for {
		e, err := follower.rb.Get(ctx, "core/seal-meta")
		if err == nil && e != nil && bytes.Equal(e.Value, rotated) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("follower never received the rotated wrapped key")
		}
		time.Sleep(50 * time.Millisecond)
	}
	restarted := follower.newCore(kms)
	if _, err := restarted.StartWhenReady(ctx, 50*time.Millisecond); err != nil {
		t.Fatalf("follower restart after rotation: %v", err)
	}
	if restarted.Sealed() {
		t.Fatal("restarted follower is sealed")
	}
	got, _ := os.ReadFile(follower.wrapped)
	if !bytes.Equal(got, rotated) {
		t.Fatal("follower did not adopt the rotated wrapped key on restart")
	}
}
