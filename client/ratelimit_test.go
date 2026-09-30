package main

import (
	"context"
	"testing"
	"time"
)

// The limiter has to pace, and it has to be able to pass a whole packet even
// when the limit is smaller than one packet per second - a bucket that can
// never hold a packet would stall the session instead of slowing it.
func TestSessionLimiter(t *testing.T) {
	old := sessionSendLimit
	defer func() { sessionSendLimit = old }()

	sessionSendLimit = 0
	if newSessionLimiter() != nil {
		t.Error("no limit configured, but a limiter was built")
	}

	// Smaller than one packet a second, which is the case that used to be a
	// deadlock rather than a slow link.
	sessionSendLimit = 100
	if lim := newSessionLimiter(); lim == nil {
		t.Fatal("a limit was configured but no limiter was built")
	} else if lim.Burst() < pktBufSize {
		t.Errorf("burst %d cannot hold one packet (%d): a session would stall, not slow down",
			lim.Burst(), pktBufSize)
	}

	// 16 KB/s: eight 2 KB packets past the initial burst should take about a
	// second, and certainly not arrive at once.
	sessionSendLimit = 16 * 1024
	lim := newSessionLimiter()
	ctx := context.Background()
	if err := lim.WaitN(ctx, lim.Burst()); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for i := 0; i < 8; i++ {
		if err := lim.WaitN(ctx, 2048); err != nil {
			t.Fatal(err)
		}
	}
	if el := time.Since(start); el < 700*time.Millisecond {
		t.Errorf("16 KB at 16 KB/s took %v: not paced", el)
	}
}
