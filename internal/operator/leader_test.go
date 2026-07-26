package operator

import (
	"regexp"
	"testing"
	"time"
)

// microTimePattern matches k8s.io/apimachinery's metav1.MicroTime wire
// format exactly: fixed 6-digit microsecond precision. The API server
// rejects anything else (see the comment on leaseTimeFormat) — this test
// exists because that failure mode was previously invisible: it only
// reproduces with a live API server and depends on the specific trailing
// digits of time.Now()'s nanosecond component, so it silently worked in
// some manual runs and silently failed in others.
var microTimePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$`)

func TestLeaseTimeFormat_AlwaysSixFractionalDigits(t *testing.T) {
	// A range of nanosecond values whose trailing zeros, once trimmed by
	// time.RFC3339Nano, would previously have produced anywhere from 0 to 9
	// fractional digits — every one of them must still format to exactly 6.
	nanos := []int{
		0,         // .000000000 -> RFC3339Nano trims to no fraction at all
		1,         // .000000001 -> 9 digits
		100,       // .000000100 -> 7 digits after trim
		123456789, // .123456789 -> 9 digits, no trailing zeros to trim
		123000000, // .123000000 -> 3 digits after trim
		555263210, // arbitrary value that reproduced the live 400 error
	}
	for _, ns := range nanos {
		tm := time.Date(2026, 7, 26, 14, 21, 49, ns, time.UTC)
		got := tm.Format(leaseTimeFormat)
		if !microTimePattern.MatchString(got) {
			t.Errorf("Format(nanos=%d) = %q, want exactly 6 fractional digits (K8s MicroTime format)", ns, got)
		}
	}
}

func TestLeaseTimeFormat_RoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 26, 14, 21, 49, 555263210, time.UTC)
	formatted := now.Format(leaseTimeFormat)
	parsed := leaseParseTime(formatted, time.Time{})
	if parsed.IsZero() {
		t.Fatalf("leaseParseTime(%q) returned zero time (parse failed)", formatted)
	}
	// Microsecond precision is expected to survive the round trip; only
	// sub-microsecond precision is lost (by design — matches MicroTime).
	if now.Truncate(time.Microsecond) != parsed {
		t.Errorf("round trip mismatch: got %v, want %v", parsed, now.Truncate(time.Microsecond))
	}
}

func TestLeaseTimeFormat_ParseFallbackOnGarbage(t *testing.T) {
	fallback := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := leaseParseTime("not-a-time", fallback); got != fallback {
		t.Errorf("leaseParseTime with garbage input = %v, want fallback %v", got, fallback)
	}
	if got := leaseParseTime("", fallback); got != fallback {
		t.Errorf("leaseParseTime with empty input = %v, want fallback %v", got, fallback)
	}
}
