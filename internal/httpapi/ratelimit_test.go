package httpapi

import "testing"

func TestIPLimiterDisabled(t *testing.T) {
	if l := newIPLimiter(0); l != nil {
		t.Error("expected nil limiter when disabled")
	}
}

func TestIPLimiterEnforcesPerIP(t *testing.T) {
	l := newIPLimiter(5)
	for i := 0; i < 5; i++ {
		if !l.allow("1.1.1.1") {
			t.Fatalf("request %d should be allowed (burst)", i+1)
		}
	}
	if l.allow("1.1.1.1") {
		t.Error("6th request should be limited")
	}
	// A different IP has its own bucket.
	if !l.allow("2.2.2.2") {
		t.Error("other IP should not be limited")
	}
}
