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

// The queue behind a paced sender, which is the part that made a tight limit
// behave like a broken link rather than a slow one.
func TestWorkerSendBufFollowsTheRate(t *testing.T) {
	if got := workerSendBufFor(0); got != workerSendBuf {
		t.Errorf("unpaced queue is %d, want the full %d", got, workerSendBuf)
	}
	// 2000 B/s is the case that failed: one packet, not 128, so a sender
	// learns it is over the limit in a packet's time instead of a minute's.
	if got := workerSendBufFor(2000); got != 1 {
		t.Errorf("at 2000 B/s the queue is %d packets, want 1", got)
	}
	// Never zero, or nothing could ever be queued at all.
	if got := workerSendBufFor(1); got < 1 {
		t.Errorf("queue of %d would take nothing", got)
	}
	// A fast limit is still capped at the unpaced depth.
	if got := workerSendBufFor(100 << 20); got != workerSendBuf {
		t.Errorf("a high limit gave %d, want it capped at %d", got, workerSendBuf)
	}
	// Roughly a sixth of a second at the configured rate.
	if got := workerSendBufFor(84000); got != 10 {
		t.Errorf("at 84000 B/s the queue is %d packets, want 10", got)
	}
}
