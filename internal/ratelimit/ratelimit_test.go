package ratelimit

import (
	"testing"
)

func TestLimiter_BurstThenDeny(t *testing.T) {
	// burst=2, rate=0.1: first two Allow() → true, third → false
	l := New(0.1, 2)

	if !l.Allow("1.2.3.4") {
		t.Fatal("first Allow() should return true")
	}
	if !l.Allow("1.2.3.4") {
		t.Fatal("second Allow() should return true")
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("third Allow() should return false (burst exhausted)")
	}
}

func TestLimiter_DifferentIPs(t *testing.T) {
	// Each IP gets its own bucket; "A" exhausting its burst does not affect "B".
	l := New(0.1, 1)

	if !l.Allow("10.0.0.1") {
		t.Fatal("first Allow() for IP A should return true")
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("second Allow() for IP A should return false")
	}
	if !l.Allow("10.0.0.2") {
		t.Fatal("first Allow() for IP B should return true")
	}
}
