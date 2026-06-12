/**
 * Tuck load test — k6 script
 *
 * Scenarios (select with -e SCENARIO=<name>):
 *   smoke   — 1 VU × 1 min  — sanity check, no traffic spike
 *   load    — 50 VU × 5 min — baseline production throughput
 *   stress  — ramp 0→200 VU × 10 min — find the breaking point
 *   soak    — 50 VU × 24 h  — memory leaks / stability
 *
 * Usage:
 *   export TUCK_ADDR=https://127.0.0.1:8200
 *   export TUCK_TOKEN=tuck_...
 *   k6 run -e SCENARIO=load tests/load/k6.js
 *   k6 run -e SCENARIO=smoke --insecure-skip-tls-verify tests/load/k6.js
 */

import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Rate, Trend } from "k6/metrics";

// ── Custom metrics ──────────────────────────────────────────────────────────
const kvPutErrors   = new Counter("kv_put_errors");
const kvGetErrors   = new Counter("kv_get_errors");
const tokenErrors   = new Counter("token_errors");
const errorRate     = new Rate("error_rate");
const kvPutLatency  = new Trend("kv_put_latency", true);
const kvGetLatency  = new Trend("kv_get_latency", true);
const tokenLatency  = new Trend("token_latency", true);

// ── Config ──────────────────────────────────────────────────────────────────
const ADDR  = __ENV.TUCK_ADDR  || "https://127.0.0.1:8200";
const TOKEN = __ENV.TUCK_TOKEN || "";

const params = {
  headers: { "X-Tuck-Token": TOKEN, "Content-Type": "application/json" },
  // Accept self-signed cert; remove in prod with proper CA.
  insecureSkipTLSVerify: true,
};

// ── Scenario definitions ─────────────────────────────────────────────────────
const SCENARIOS = {
  smoke: {
    executor: "constant-vus",
    vus: 1,
    duration: "1m",
  },
  load: {
    executor: "constant-vus",
    vus: 50,
    duration: "5m",
  },
  stress: {
    executor: "ramping-vus",
    startVUs: 0,
    stages: [
      { duration: "2m", target: 50  },
      { duration: "3m", target: 100 },
      { duration: "3m", target: 200 },
      { duration: "2m", target: 0   },
    ],
  },
  soak: {
    executor: "constant-vus",
    vus: 50,
    duration: "24h",
  },
};

const scenario = __ENV.SCENARIO || "load";

export const options = {
  scenarios: { [scenario]: SCENARIOS[scenario] },
  thresholds: {
    // 99th-percentile latency targets
    "http_req_duration{endpoint:kv_get}":        ["p(99)<50"],
    "http_req_duration{endpoint:kv_put}":        ["p(99)<50"],
    "http_req_duration{endpoint:token_create}":  ["p(99)<100"],
    "http_req_duration{endpoint:seal_status}":   ["p(99)<20"],
    // Error rate must stay below 0.1 %
    error_rate: ["rate<0.001"],
    // Custom latency trends
    kv_get_latency:  ["p(99)<50"],
    kv_put_latency:  ["p(99)<50"],
    token_latency:   ["p(99)<100"],
  },
};

// ── VU workload ──────────────────────────────────────────────────────────────
export default function () {
  const vu  = __VU;
  const iter = __ITER;

  // 1. Write a secret (40 % of traffic)
  {
    const path  = `/v1/secret/load/vu${vu}/key${iter % 100}`;
    const start = Date.now();
    const res   = http.put(ADDR + path, JSON.stringify({ value: `v${iter}` }), params);
    kvPutLatency.add(Date.now() - start);

    const ok = check(res, {
      "kv put 204": (r) => r.status === 204,
    });
    if (!ok) { kvPutErrors.add(1); errorRate.add(1); } else { errorRate.add(0); }
  }

  sleep(0.01);

  // 2. Read the same secret (40 % of traffic)
  {
    const path  = `/v1/secret/load/vu${vu}/key${iter % 100}`;
    const start = Date.now();
    const res   = http.get(ADDR + path, params);
    kvGetLatency.add(Date.now() - start);

    const ok = check(res, {
      "kv get 200": (r) => r.status === 200,
    });
    if (!ok) { kvGetErrors.add(1); errorRate.add(1); } else { errorRate.add(0); }
  }

  sleep(0.01);

  // 3. Seal-status health check (10 % of traffic — simulates LB probes)
  if (iter % 10 === 0) {
    const res = http.get(ADDR + "/v1/sys/seal-status",
      { tags: { endpoint: "seal_status" }, insecureSkipTLSVerify: true });
    check(res, { "seal-status 200": (r) => r.status === 200 });
  }

  // 4. Create a short-lived token (10 % of traffic — simulates service init)
  if (iter % 10 === 1) {
    const start = Date.now();
    const res   = http.post(
      ADDR + "/v1/auth/token",
      JSON.stringify({ display_name: `vu${vu}`, policies: ["default"], ttl: "1m" }),
      { ...params, tags: { endpoint: "token_create" } },
    );
    tokenLatency.add(Date.now() - start);

    const ok = check(res, { "token create 201": (r) => r.status === 201 });
    if (!ok) { tokenErrors.add(1); errorRate.add(1); } else { errorRate.add(0); }
  }

  sleep(0.05);
}

// ── Summary hook ─────────────────────────────────────────────────────────────
export function handleSummary(data) {
  const dur = data.metrics.http_req_duration;
  console.log("─── Load Test Summary ───────────────────────────────");
  console.log(`Scenario : ${scenario}`);
  console.log(`Requests : ${data.metrics.http_reqs.values.count}`);
  console.log(`RPS      : ${data.metrics.http_reqs.values.rate.toFixed(1)}`);
  console.log(`p50      : ${dur.values["p(50)"].toFixed(2)} ms`);
  console.log(`p95      : ${dur.values["p(95)"].toFixed(2)} ms`);
  console.log(`p99      : ${dur.values["p(99)"].toFixed(2)} ms`);
  console.log(`Errors   : ${data.metrics.error_rate.values.rate * 100} %`);
  console.log("────────────────────────────────────────────────────");
  return { "tests/load/results.json": JSON.stringify(data, null, 2) };
}
