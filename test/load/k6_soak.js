/**
 * Tuck — k6 load & soak test
 *
 * Requires k6 (https://k6.io).
 *
 * Quick load test (5 000 RPS for 1 min):
 *   k6 run --env TUCK_ADDR=http://localhost:8200 \
 *          --env TUCK_TOKEN=tuck_... \
 *          --duration 1m --vus 200 k6_soak.js
 *
 * 24-hour soak (steady 200 RPS, watch for memory leak):
 *   k6 run --env TUCK_ADDR=http://localhost:8200 \
 *          --env TUCK_TOKEN=tuck_... \
 *          --duration 24h --vus 20 k6_soak.js
 *
 * Environment variables:
 *   TUCK_ADDR   - server base URL (default http://localhost:8200)
 *   TUCK_TOKEN  - root or service token with read/write on secret/load-test/*
 *   TUCK_INSECURE - set to "1" to skip TLS verification (for self-signed dev certs)
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const addr  = __ENV.TUCK_ADDR  || 'http://localhost:8200';
const token = __ENV.TUCK_TOKEN || '';

const params = {
  headers: {
    'X-Tuck-Token': token,
    'Content-Type': 'application/json',
  },
  timeout: '10s',
};

// Custom metrics
const writeLatency  = new Trend('tuck_write_latency_ms');
const readLatency   = new Trend('tuck_read_latency_ms');
const listLatency   = new Trend('tuck_list_latency_ms');
const errorRate     = new Rate('tuck_error_rate');
const sealedErrors  = new Counter('tuck_sealed_errors');

export const options = {
  thresholds: {
    // 99th percentile write < 200ms, read < 100ms
    'tuck_write_latency_ms': ['p(99)<200'],
    'tuck_read_latency_ms':  ['p(99)<100'],
    // Error rate < 0.1%
    'tuck_error_rate': ['rate<0.001'],
    // Standard k6 http metrics
    'http_req_failed': ['rate<0.001'],
  },
};

// Seed a few secrets at startup so reads don't all 404.
export function setup() {
  if (!token) { console.warn('TUCK_TOKEN not set — requests will be unauthorised'); return; }
  for (let i = 0; i < 10; i++) {
    const path = `load-test/key-${i}`;
    http.put(`${addr}/v1/secret/${path}`, `value-${i}-seed`, params);
  }
}

export default function () {
  const id = Math.floor(Math.random() * 10);
  const path = `load-test/key-${id}`;

  // 60% reads, 30% writes, 10% lists
  const roll = Math.random();

  if (roll < 0.60) {
    // Read
    const res = http.get(`${addr}/v1/secret/${path}`, params);
    readLatency.add(res.timings.duration);
    const ok = check(res, { 'read 200': r => r.status === 200 });
    if (!ok) {
      errorRate.add(1);
      if (res.status === 503) sealedErrors.add(1);
    } else {
      errorRate.add(0);
    }

  } else if (roll < 0.90) {
    // Write
    const res = http.put(
      `${addr}/v1/secret/${path}`,
      `value-${id}-${Date.now()}`,
      params,
    );
    writeLatency.add(res.timings.duration);
    const ok = check(res, { 'write 204': r => r.status === 204 });
    errorRate.add(ok ? 0 : 1);
    if (!ok && res.status === 503) sealedErrors.add(1);

  } else {
    // List
    const res = http.request('LIST', `${addr}/v1/secret/load-test/`, null, params);
    listLatency.add(res.timings.duration);
    const ok = check(res, { 'list 200': r => r.status === 200 });
    errorRate.add(ok ? 0 : 1);
  }

  sleep(0.01); // 10ms think time → ~100 RPS per VU
}

export function teardown() {
  // Clean up seeded secrets
  for (let i = 0; i < 10; i++) {
    http.del(`${addr}/v1/secret/load-test/key-${i}`, null, params);
  }
}
