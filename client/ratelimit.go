package main

import (
	"golang.org/x/time/rate"
)

// How much one session may send to its TURN relay, in bytes per second. Zero
// leaves it alone. Set once from -rate-up before any session starts.
//
// Per session rather than per relay address or in total, because the session
// is what VK allocates, meters and sees as one call stream: a dozen sessions
// share one relay address here, so shaping by address would describe nothing
// VK measures. Measured without it, a session sends about 90 KB/s where a
// real call's uplink is a fraction of that.
var sessionSendLimit int

// How deep a paced session's queue may be. A queue has to be measured in
// time, not in packets: 128 packets is right behind a sender running at full
// speed and absurd behind one running at a couple of KB/s, where it is most
// of a minute of buffering. A TCP flow inside the tunnel reads that as a dead
// link and answers with RTO backoff rather than with a smaller window, which
// is worse than the loss it was avoiding - measured at 126 Kbit/s of 288
// available, 178 retransmits in 25s, and large-packet latency up to a second.
//
// About a sixth of a second of queue, and never less than one packet, so the
// excess is refused early enough for the sender to notice at once.
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

// nil when no limit is set, so the send path costs nothing in the usual case.
//
// The burst is one whole packet: a token bucket that cannot hold a packet's
// worth would never release one, and the session would stall rather than
// slow down.
func newSessionLimiter() *rate.Limiter {
	if sessionSendLimit <= 0 {
		return nil
	}
	burst := sessionSendLimit
	if burst < pktBufSize {
		burst = pktBufSize
	}
	return rate.NewLimiter(rate.Limit(sessionSendLimit), burst)
}
