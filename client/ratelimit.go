package main

import (
	"golang.org/x/time/rate"
)

// What one session may send to its TURN relay, in bytes per second. Zero
// leaves it alone. Set once from -rate-up before any session starts.
//
// Per session because that is what VK allocates and meters, and because
// several sessions share one relay address here, so a per-address limit would
// describe nothing VK sees. Measured without it, a session sends about
// 90 KB/s where a real call's uplink is a fraction of that.
var sessionSendLimit int

// nil when no limit is set, so the send path costs nothing in the usual case.
// Every session's Writer waits on this one bucket.
var tunnelLimiter *rate.Limiter

// One bucket for the whole tunnel, at the per-session rate times the number
// of sessions. That it is not one bucket per session is the part that looks
// wrong and is not: a session paced on its own takes ~44ms to clear a packet
// at a call's rate, far longer than the dispatcher will dwell on one relay,
// so the run of consecutive packets that keeps a flow on a single relay no
// longer fits and every packet leaves by a different one. Relays differ in
// latency by tens of milliseconds, so the far side receives a shuffle and
// reads it as loss. Measured at 256 Kbit/s over 18 sessions, per session
// against shared: 502 fast retransmits in 25s against 97, six timeouts
// against none, and a quarter to a third of every second carrying nothing at
// all against none. Shared, those runs survive, the tunnel holds 93% of its
// budget instead of 85%, and each session still averages the rate it was
// given.
//
// The burst is one packet, which is both the least and the most that works.
// A bucket too small to hold a packet would never release one and the tunnel
// would stall rather than slow down; a bucket holding a second of traffic is
// full whenever the tunnel has been quiet, so it empties at line speed and
// then waits to refill, which the TCP inside reads as a link going on and off
// rather than as a slow one.
func initTunnelLimiter(workers int) {
	if sessionSendLimit <= 0 || workers <= 0 {
		tunnelLimiter = nil
		return
	}
	tunnelLimiter = rate.NewLimiter(rate.Limit(sessionSendLimit*workers), pktBufSize)
}

// Follows the sessions that are actually carrying traffic, not the number
// asked for. A total fixed at the latter would let each survivor send more
// than a call's worth whenever some of them failed to come up, which is the
// one thing the limit exists to prevent.
func setTunnelSessions(n int) {
	if tunnelLimiter == nil || n <= 0 {
		return
	}
	tunnelLimiter.SetLimit(rate.Limit(sessionSendLimit * n))
}

// How deep one paced session's queue may be. A queue has to be measured in
// time, not in packets: 128 packets is right behind a sender running at full
// speed and absurd behind one running at a couple of KB/s, where it is most
// of a minute of buffering. A TCP flow inside the tunnel reads that as a dead
// link and answers with RTO backoff rather than with a smaller window, which
// is worse than the loss it was avoiding - measured at 126 Kbit/s of 288
// available, 178 retransmits in 25s, and large-packet latency up to a second.
//
// Sized from the per-session share, so the queues together hold about a sixth
// of a second at the tunnel's rate however many sessions there are. Never
// less than one packet, so the excess is refused early enough for the sender
// to notice at once.
func workerSendBufFor(bytesPerSec int) int {
	if bytesPerSec <= 0 {
		return workerSendBuf
	}
	n := bytesPerSec / 6 / 1400
	if n < 1 {
		n = 1
	}
	if n > workerSendBuf {
		n = workerSendBuf
	}
	return n
}
