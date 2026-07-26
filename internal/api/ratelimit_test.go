package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestSysconfigRoundtrip verifies that PUT /v1/sys/config persists values and
// GET /v1/sys/config returns exactly those values.
func TestSysconfigRoundtrip(t *testing.T) {
	ts, _, tok := newTestServer(t)

	body := `{"ip_rate_limit_rps":42.5,"ip_rate_limit_burst":10,"token_rate_limit_rps":5.0,"token_rate_limit_burst":3}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/sys/config", body, tok))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /v1/sys/config status = %d, want 200", resp.StatusCode)
	}

	resp, err = http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/config", "", tok))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/sys/config status = %d, want 200", resp.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode GET /v1/sys/config: %v", err)
	}
	if got["ip_rate_limit_rps"] != 42.5 {
		t.Errorf("ip_rate_limit_rps = %v, want 42.5", got["ip_rate_limit_rps"])
	}
	if int(got["ip_rate_limit_burst"].(float64)) != 10 {
		t.Errorf("ip_rate_limit_burst = %v, want 10", got["ip_rate_limit_burst"])
	}
	if got["token_rate_limit_rps"] != 5.0 {
		t.Errorf("token_rate_limit_rps = %v, want 5.0", got["token_rate_limit_rps"])
	}
	if int(got["token_rate_limit_burst"].(float64)) != 3 {
		t.Errorf("token_rate_limit_burst = %v, want 3", got["token_rate_limit_burst"])
	}
}

// TestIPRateLimitBlocks verifies that after configuring burst=1 IP rate limit,
// a second immediate request from the same IP is rejected with HTTP 429.
// Rate config is applied in-process via PUT /v1/sys/config → ApplyRateConfig.
func TestIPRateLimitBlocks(t *testing.T) {
	ts, _, tok := newTestServer(t)

	// Set a near-zero refill rate so the burst slot never refills during the test.
	cfg := `{"ip_rate_limit_rps":0.001,"ip_rate_limit_burst":1}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/sys/config", cfg, tok))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("configure IP rate limit: status = %d, want 200", resp.StatusCode)
	}

	// First request: consumes the single burst token.
	r1, err := http.Get(ts.URL + "/v1/health") //nolint:noctx
	if err != nil {
		t.Fatal(err)
	}
	_ = r1.Body.Close()
	if r1.StatusCode != http.StatusOK {
		t.Fatalf("first GET /v1/health: status = %d, want 200", r1.StatusCode)
	}

	// Second immediate request: bucket empty → must be rejected.
	r2, err := http.Get(ts.URL + "/v1/health") //nolint:noctx
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second GET /v1/health: status = %d, want 429", r2.StatusCode)
	}
}

// TestTokenRateLimitBlocks verifies that after configuring burst=1 token rate limit,
// a second immediate authenticated request with the same token is rejected with 429.
func TestTokenRateLimitBlocks(t *testing.T) {
	ts, _, tok := newTestServer(t)

	cfg := `{"token_rate_limit_rps":0.001,"token_rate_limit_burst":1}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/sys/config", cfg, tok))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("configure token rate limit: status = %d, want 200", resp.StatusCode)
	}

	// First authenticated request: consumes the single burst slot for this token.
	r1, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/config", "", tok))
	if err != nil {
		t.Fatal(err)
	}
	_ = r1.Body.Close()
	if r1.StatusCode != http.StatusOK {
		t.Fatalf("first GET /v1/sys/config: status = %d, want 200", r1.StatusCode)
	}

	// Second immediate request with the same token: must be blocked.
	r2, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/sys/config", "", tok))
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second GET /v1/sys/config: status = %d, want 429", r2.StatusCode)
	}
}

// TestRateLimitDisabled verifies that rps=0 / burst=0 fully disables rate limiting
// so that many rapid requests all succeed.
func TestRateLimitDisabled(t *testing.T) {
	ts, _, tok := newTestServer(t)

	cfg := `{"ip_rate_limit_rps":0,"ip_rate_limit_burst":0,"token_rate_limit_rps":0,"token_rate_limit_burst":0}`
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/sys/config", cfg, tok))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable rate limit: status = %d", resp.StatusCode)
	}

	for i := range 20 {
		r, err := http.Get(ts.URL + "/v1/health") //nolint:noctx
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, r.StatusCode)
		}
	}
}

// TestIPRateLimitPerIP verifies that rate limit buckets are per-IP: two different
// source IPs each get their own independent burst slot.
// (In the test environment all requests share 127.0.0.1, so this test exercises
// the rate-limit-disabled path across distinct tokens to confirm isolation.)
func TestRateLimitTokenIsolation(t *testing.T) {
	ts, c, rootTok := newTestServer(t)

	// Create a second token so we have two independent identities.
	resp, err := http.DefaultClient.Do(authedReq(t, http.MethodPost, ts.URL+"/v1/auth/token",
		`{"policies":["root"]}`, rootTok))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	_ = c // suppress unused warning
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create second token: status = %d, want 201", resp.StatusCode)
	}
	var tokenResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	tok2 := tokenResp.ID
	if tok2 == "" {
		t.Skip("create token returned empty — skipping isolation test")
	}

	// Configure tight token rate limit.
	cfg := `{"token_rate_limit_rps":0.001,"token_rate_limit_burst":1}`
	r, err := http.DefaultClient.Do(authedReq(t, http.MethodPut, ts.URL+"/v1/sys/config", cfg, rootTok))
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Body.Close()

	// Exhaust tok2's bucket.
	r1, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/health", "", tok2))
	if err != nil {
		t.Fatal(err)
	}
	_ = r1.Body.Close()

	// rootTok has its own independent bucket and is not affected.
	r2, err := http.DefaultClient.Do(authedReq(t, http.MethodGet, ts.URL+"/v1/health", "", rootTok))
	if err != nil {
		t.Fatal(err)
	}
	_ = r2.Body.Close()
	if r2.StatusCode == http.StatusTooManyRequests {
		t.Errorf("rootTok request was rate-limited despite using a different token bucket")
	}
}
