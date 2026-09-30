package main

import (
	"context"
	"testing"
	"time"
)

func TestTunnelLimiterIsOnlyBuiltWhenAsked(t *testing.T) {
	defer restoreLimit(t)

	sessionSendLimit = 0
	initTunnelLimiter(18)
	if tunnelLimiter != nil {
		t.Error("no limit configured, but a limiter was built")
	}

	// No sessions is not a rate of nothing: it is a tunnel that carries
	// nothing yet, and pacing it to zero would wedge every Writer.
	sessionSendLimit = 32000
	initTunnelLimiter(0)
	if tunnelLimiter != nil {
		t.Error("zero sessions built a limiter that could never release a packet")
	}
}

// The budget is written per session, so the tunnel's is that times the number
// of sessions - the figure the interface page promises and the one a user
// sets the limit against.
func TestTunnelLimiterSpendsEverySessionsShare(t *testing.T) {
	defer restoreLimit(t)

	sessionSendLimit = 32000 // 256 Kbit/s a session
	initTunnelLimiter(18)
	if tunnelLimiter == nil {
		t.Fatal("a limit was configured but no limiter was built")
	}
	if got, want := int(tunnelLimiter.Limit()), 18*32000; got != want {
		t.Errorf("tunnel paced to %d B/s, want %d: 18 sessions at 32000 B/s each", got, want)
	}
}

// Sessions come and go, and the tunnel's share has to follow them. The limit
// is written per session, so a tunnel that lost half of them must send half
// as much rather than let the survivors take up the slack.
func TestTunnelLimiterFollowsTheLiveSessionCount(t *testing.T) {
	defer restoreLimit(t)

	sessionSendLimit = 32000
	initTunnelLimiter(18)

	setTunnelSessions(9)
	if got, want := int(tunnelLimiter.Limit()), 9*32000; got != want {
		t.Errorf("9 sessions left, tunnel paced to %d B/s, want %d", got, want)
	}
	// The last one going must not leave a bucket that releases nothing: a
	// Writer waiting on a rate of zero never comes back.
	setTunnelSessions(0)
	if int(tunnelLimiter.Limit()) == 0 {
		t.Error("tunnel paced to zero, which a Writer would wait on for ever")
	}

	// Unpaced stays unpaced, whatever the sessions do.
	restoreLimit(t)
	setTunnelSessions(18)
	if tunnelLimiter != nil {
		t.Error("no limit configured, but registering a session built one")
	}
}

// A bucket is full when the tunnel is idle, so its burst is what leaves at
// line speed the moment there is something to send. A burst of one whole
// second meant twenty-odd packets went out back to back and then nothing
// until it refilled. The TCP inside reads that as an on/off link rather than
// a slow one: it lost a window to tail drop at every pause and spent whole
// seconds in RTO backoff, so the transfer reported zero bytes for one second
// in seven.
func TestTunnelLimiterDoesNotStoreUpASecond(t *testing.T) {
	defer restoreLimit(t)

	sessionSendLimit = 32000
	initTunnelLimiter(18)

	// One instant, so this measures the standing burst and not the rate.
	var atOnce int
	for now := time.Now(); tunnelLimiter.AllowN(now, 1400); {
		atOnce += 1400
	}
	if atOnce > 2*1400 {
		t.Errorf("%d B left at once, %.2fs of traffic at %d B/s: a paced tunnel should start at its rate, not empty a stored-up bucket",
			atOnce, float64(atOnce)/float64(18*32000), 18*32000)
	}
}

// A limit under one packet a second used to be a deadlock rather than a slow
// link: a bucket that cannot hold a packet never releases one.
func TestTunnelLimiterPacesAndNeverStalls(t *testing.T) {
	defer restoreLimit(t)

	sessionSendLimit = 100
	initTunnelLimiter(1)
	if tunnelLimiter.Burst() < pktBufSize {
		t.Errorf("burst %d cannot hold one packet (%d): the tunnel would stall, not slow down",
			tunnelLimiter.Burst(), pktBufSize)
	}

	// 16 KB/s: eight 2 KB packets past the initial burst should take about a
	// second, and certainly not arrive at once.
	sessionSendLimit = 16 * 1024
	initTunnelLimiter(1)
	ctx := context.Background()
	if err := tunnelLimiter.WaitN(ctx, tunnelLimiter.Burst()); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for i := 0; i < 8; i++ {
		if err := tunnelLimiter.WaitN(ctx, 2048); err != nil {
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

func restoreLimit(t *testing.T) {
	t.Helper()
	sessionSendLimit = 0
	tunnelLimiter = nil
}
