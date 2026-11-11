package server

import (
	"testing"
	"time"
)

func TestTickStatsSummary(t *testing.T) {
	var ts tickStats
	if n, _, _, _ := ts.summary(); n != 0 {
		t.Fatalf("empty n = %d", n)
	}
	for i := 1; i <= 100; i++ {
		ts.add(time.Duration(i) * time.Millisecond)
	}
	n, mean, p95, max := ts.summary()
	if n != 100 {
		t.Errorf("n = %d", n)
	}
	if mean != 50500*time.Microsecond {
		t.Errorf("mean = %v", mean)
	}
	if p95 != 95*time.Millisecond {
		t.Errorf("p95 = %v", p95)
	}
	if max != 100*time.Millisecond {
		t.Errorf("max = %v", max)
	}
	ts.reset()
	if n, _, _, _ := ts.summary(); n != 0 {
		t.Errorf("after reset n = %d", n)
	}
}

func TestSlowTickLimiter(t *testing.T) {
	var l slowLogLimiter
	t0 := time.Unix(100, 0)
	if !l.allow(t0) {
		t.Fatal("first should be allowed")
	}
	if l.allow(t0.Add(500 * time.Millisecond)) {
		t.Fatal("within a second should be denied")
	}
	if !l.allow(t0.Add(time.Second)) {
		t.Fatal("after a second should be allowed")
	}
}
