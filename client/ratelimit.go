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
