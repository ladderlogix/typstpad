package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterWindow(t *testing.T) {
	l := New(3, time.Minute)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if l.Allow("k") {
		t.Fatal("4th request should be blocked")
	}
	// A different key is independent.
	if !l.Allow("other") {
		t.Fatal("different key should be allowed")
	}
	// After the window elapses, the key resets.
	now = now.Add(time.Minute + time.Second)
	if !l.Allow("k") {
		t.Fatal("request after window reset should be allowed")
	}
}
