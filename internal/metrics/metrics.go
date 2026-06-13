// Package metrics exposes a minimal Prometheus-compatible /metrics endpoint.
// It uses no external dependencies — the text format is written directly.
package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// startTime is set once at process start; used to compute uptime_seconds.
var startTime = time.Now()

// Counters exposed globally.
var (
	RequestsTotal   [6]atomic.Int64 // indexed by HTTP status class: 2xx,3xx,4xx,5xx,0xx(err),total
	AuthFailures    atomic.Int64
	SealedRequests  atomic.Int64 // requests rejected because sealed
	UnsealOps       atomic.Int64 // POST /v1/sys/unseal calls
	GCRemovedTokens atomic.Int64
	WrappingRevokes atomic.Int64 // wrapping tokens revoked/expired
	AuditSinkErrors atomic.Int64 // cumulative errors across all sinks
)

// Gauges — set by core on state changes.
var (
	SealedGauge       atomic.Int32 // 0 = unsealed, 1 = sealed
	ActiveTokensGauge atomic.Int64 // live (non-expired, non-revoked) tokens
	LeaseCountGauge   atomic.Int64 // active dynamic-secret leases
)

// Inc2xx increments 2xx counter and total.
func Inc2xx() { RequestsTotal[0].Add(1); RequestsTotal[5].Add(1) }

// Inc4xx increments 4xx counter and total.
func Inc4xx() { RequestsTotal[2].Add(1); RequestsTotal[5].Add(1) }

// Inc5xx increments 5xx counter and total.
func Inc5xx() { RequestsTotal[3].Add(1); RequestsTotal[5].Add(1) }

// IncAuthFailure increments auth failure counter.
func IncAuthFailure() { AuthFailures.Add(1) }

// IncSealed increments sealed-request counter.
func IncSealed() { SealedRequests.Add(1) }

// IncUnsealOp increments unseal operation counter.
func IncUnsealOp() { UnsealOps.Add(1) }

// IncGCRemoved increments GC removed tokens counter.
func IncGCRemoved(n int64) { GCRemovedTokens.Add(n) }

// IncAuditSinkError increments the audit sink error counter.
func IncAuditSinkError() { AuditSinkErrors.Add(1) }

// SetActiveTokens sets the active token gauge.
func SetActiveTokens(n int64) { ActiveTokensGauge.Store(n) }

// SetLeaseCount sets the active lease gauge.
func SetLeaseCount(n int64) { LeaseCountGauge.Store(n) }

// UptimeSeconds returns seconds elapsed since process start.
func UptimeSeconds() float64 { return time.Since(startTime).Seconds() }

// Handler returns an http.HandlerFunc serving Prometheus text metrics.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP tuck_requests_total Total HTTP requests by status class\n")
		fmt.Fprintf(w, "# TYPE tuck_requests_total counter\n")
		fmt.Fprintf(w, `tuck_requests_total{class="2xx"} %d`+"\n", RequestsTotal[0].Load())
		fmt.Fprintf(w, `tuck_requests_total{class="4xx"} %d`+"\n", RequestsTotal[2].Load())
		fmt.Fprintf(w, `tuck_requests_total{class="5xx"} %d`+"\n", RequestsTotal[3].Load())
		fmt.Fprintf(w, `tuck_requests_total{class="total"} %d`+"\n", RequestsTotal[5].Load())

		fmt.Fprintf(w, "# HELP tuck_auth_failures_total Authentication failures\n")
		fmt.Fprintf(w, "# TYPE tuck_auth_failures_total counter\n")
		fmt.Fprintf(w, "tuck_auth_failures_total %d\n", AuthFailures.Load())

		fmt.Fprintf(w, "# HELP tuck_sealed Whether the barrier is sealed (1) or not (0)\n")
		fmt.Fprintf(w, "# TYPE tuck_sealed gauge\n")
		fmt.Fprintf(w, "tuck_sealed %d\n", SealedGauge.Load())

		fmt.Fprintf(w, "# HELP tuck_unseal_ops_total Unseal shard operations\n")
		fmt.Fprintf(w, "# TYPE tuck_unseal_ops_total counter\n")
		fmt.Fprintf(w, "tuck_unseal_ops_total %d\n", UnsealOps.Load())

		fmt.Fprintf(w, "# HELP tuck_gc_removed_tokens_total Expired tokens removed by GC\n")
		fmt.Fprintf(w, "# TYPE tuck_gc_removed_tokens_total counter\n")
		fmt.Fprintf(w, "tuck_gc_removed_tokens_total %d\n", GCRemovedTokens.Load())

		fmt.Fprintf(w, "# HELP tuck_uptime_seconds Seconds since the tuck process started\n")
		fmt.Fprintf(w, "# TYPE tuck_uptime_seconds gauge\n")
		fmt.Fprintf(w, "tuck_uptime_seconds %.3f\n", UptimeSeconds())

		fmt.Fprintf(w, "# HELP tuck_active_tokens Active (non-expired) token count\n")
		fmt.Fprintf(w, "# TYPE tuck_active_tokens gauge\n")
		fmt.Fprintf(w, "tuck_active_tokens %d\n", ActiveTokensGauge.Load())

		fmt.Fprintf(w, "# HELP tuck_lease_count Active dynamic-secret lease count\n")
		fmt.Fprintf(w, "# TYPE tuck_lease_count gauge\n")
		fmt.Fprintf(w, "tuck_lease_count %d\n", LeaseCountGauge.Load())

		fmt.Fprintf(w, "# HELP tuck_audit_sink_errors_total Cumulative errors across all audit sinks\n")
		fmt.Fprintf(w, "# TYPE tuck_audit_sink_errors_total counter\n")
		fmt.Fprintf(w, "tuck_audit_sink_errors_total %d\n", AuditSinkErrors.Load())
	}
}
