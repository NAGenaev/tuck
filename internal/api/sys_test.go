package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/NAGenaev/tuck/internal/core"
	"github.com/NAGenaev/tuck/internal/physical"
	"github.com/NAGenaev/tuck/internal/seal"
)


// newShamirServerWithShares sets up a realistic Shamir unseal scenario:
//
//  1. First boot: ShamirSeal.Init() initializes the barrier. StartResult.Shares
//     contains the operator shares.
//  2. Core is sealed (process-restart simulation).
//  3. Second boot: a fresh ShamirSeal on the same config/backend returns
//     ErrNeedsUnseal. The server waits for shards via POST /v1/sys/unseal.
//
// Returns the test server, the waiting core, the shares, and the root token ID.
func newShamirServerWithShares(t *testing.T, n, k int) (*httptest.Server, *core.Core, []string, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "shamir.json")
	backend := physical.NewInMem()

	// --- Step 1: first boot ---
	sealBoot, err := seal.NewShamir(cfgPath, n, k)
	if err != nil {
		t.Fatalf("NewShamir: %v", err)
	}
	bootCore := core.New(backend, sealBoot)
	bootResult, err := bootCore.Start(context.Background())
	if err != nil {
		t.Fatalf("first-boot Start: %v", err)
	}
	if bootResult == nil || bootResult.RootToken == nil {
		t.Fatal("first-boot: expected StartResult with RootToken")
	}
	if len(bootResult.Shares) != n {
		t.Fatalf("first-boot: expected %d shares, got %d", n, len(bootResult.Shares))
	}
	shares := bootResult.Shares
	rootTokID := bootResult.RootToken.ID

	// Simulate process restart: seal the core.
	bootCore.Seal()

	// --- Step 2: restart boot — expects ErrNeedsUnseal ---
	sealRestart, err := seal.NewShamirFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("NewShamirFromConfig: %v", err)
	}
	shamirCore := core.New(backend, sealRestart)
	_, startErr := shamirCore.Start(context.Background())
	if startErr != core.ErrNeedsUnseal {
		t.Fatalf("restart Start() = %v, want ErrNeedsUnseal", startErr)
	}

	ts := httptest.NewServer(New(shamirCore).Handler())
	t.Cleanup(ts.Close)
	return ts, shamirCore, shares, rootTokID
}

// TestSealStatus_DevSeal checks GET /v1/sys/seal-status for the dev seal.
func TestSealStatus_DevSeal(t *testing.T) {
	ts, _, _ := newTestServer(t)

	resp, err := http.Get(ts.URL + "/v1/sys/seal-status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("seal-status status = %d, want 200; body: %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if result["sealed"] != false {
		t.Errorf("sealed = %v, want false", result["sealed"])
	}
	if result["type"] != "dev" {
		t.Errorf("type = %v, want \"dev\"", result["type"])
	}
}

// TestSealStatus_NoAuth verifies GET /v1/sys/seal-status needs no auth.
func TestSealStatus_NoAuth(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/v1/sys/seal-status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("seal-status without token: status = %d, want 200", resp.StatusCode)
	}
}

// TestPostSeal_RequiresToken verifies POST /v1/sys/seal rejects missing tokens.
func TestPostSeal_RequiresToken(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, err := http.Post(ts.URL+"/v1/sys/seal", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /v1/sys/seal without token: status = %d, want 401", resp.StatusCode)
	}
}

// TestPostSeal_SealsThenStatusReflects verifies POST /v1/sys/seal seals the
// core and the status endpoint reflects the change.
func TestPostSeal_SealsThenStatusReflects(t *testing.T) {
	ts, _, rootTok := newTestServer(t)

	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/sys/seal", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST seal: status = %d, want 200; body: %s", resp.StatusCode, body)
	}

	// Now seal-status should report sealed=true.
	resp2, _ := http.Get(ts.URL + "/v1/sys/seal-status")
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	var status map[string]any
	json.Unmarshal(body2, &status) //nolint:errcheck
	if status["sealed"] != true {
		t.Errorf("after seal: sealed = %v, want true", status["sealed"])
	}
}

// TestPostUnseal_DevSealReturns400 verifies that POST /v1/sys/unseal returns
// 400 when the active seal is not SharableUnseal.
func TestPostUnseal_DevSealReturns400(t *testing.T) {
	ts, _, _ := newTestServer(t)

	body := `{"key":"dGVzdA"}`
	resp, err := http.Post(ts.URL+"/v1/sys/unseal", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST unseal on dev seal: status = %d, want 400", resp.StatusCode)
	}
}

// TestPostUnseal_MissingKey returns 400 for empty key field.
func TestPostUnseal_MissingKey(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, err := http.Post(ts.URL+"/v1/sys/unseal", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST unseal with no key: status = %d, want 400", resp.StatusCode)
	}
}

// TestReady_Unsealed verifies GET /v1/sys/ready returns 200 when the barrier is open.
func TestReady_Unsealed(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/v1/sys/ready")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ready (unsealed): status = %d, want 200; body: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.Unmarshal(body, &result) //nolint:errcheck
	if result["ready"] != true {
		t.Errorf("ready = %v, want true", result["ready"])
	}
}

// TestReady_Sealed verifies GET /v1/sys/ready returns 503 when sealed — so
// Kubernetes readinessProbe stops routing traffic during unseal.
func TestReady_Sealed(t *testing.T) {
	ts, c, _ := newTestServer(t)
	c.Seal()

	resp, err := http.Get(ts.URL + "/v1/sys/ready")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("ready (sealed): status = %d, want 503; body: %s", resp.StatusCode, body)
	}
	var result map[string]any
	json.Unmarshal(body, &result) //nolint:errcheck
	if result["sealed"] != true {
		t.Errorf("sealed = %v, want true", result["sealed"])
	}
}

// TestReady_NoAuth verifies GET /v1/sys/ready needs no token.
func TestReady_NoAuth(t *testing.T) {
	ts, _, _ := newTestServer(t)
	resp, _ := http.Get(ts.URL + "/v1/sys/ready")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("ready without token: status = %d, want 200", resp.StatusCode)
	}
}

// TestShamirUnsealFlow drives the complete Shamir unseal ceremony:
// server starts sealed, three shard POSTs open the barrier.
func TestShamirUnsealFlow(t *testing.T) {
	ts, shamirCore, shares, _ := newShamirServerWithShares(t, 5, 3)

	// Server should be sealed at start.
	if !shamirCore.Sealed() {
		t.Fatal("expected sealed after Start with Shamir seal")
	}

	// Supply two shards — should stay sealed.
	for i := 0; i < 2; i++ {
		body, _ := json.Marshal(map[string]string{"key": shares[i]})
		resp, err := http.Post(ts.URL+"/v1/sys/unseal", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("shard %d: %v", i, err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("shard %d: status = %d, body: %s", i, resp.StatusCode, respBody)
		}
		var result map[string]any
		json.Unmarshal(respBody, &result) //nolint:errcheck
		if result["sealed"] != true {
			t.Errorf("shard %d: sealed = %v, want true (threshold not yet met)", i, result["sealed"])
		}
	}

	// Third shard — should unseal.
	body, _ := json.Marshal(map[string]string{"key": shares[2]})
	resp, err := http.Post(ts.URL+"/v1/sys/unseal", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("threshold shard: status = %d, body: %s", resp.StatusCode, respBody)
	}

	var result map[string]any
	json.Unmarshal(respBody, &result) //nolint:errcheck
	if result["sealed"] != false {
		t.Errorf("after k shards: sealed = %v, want false", result["sealed"])
	}
	if result["message"] != "unseal complete" {
		t.Errorf("after k shards: message = %v, want \"unseal complete\"", result["message"])
	}

	// Verify the seal-status endpoint now reports unsealed.
	resp2, _ := http.Get(ts.URL + "/v1/sys/seal-status")
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	var status map[string]any
	json.Unmarshal(body2, &status) //nolint:errcheck
	if status["sealed"] != false {
		t.Errorf("seal-status after unseal: sealed = %v, want false", status["sealed"])
	}
}
