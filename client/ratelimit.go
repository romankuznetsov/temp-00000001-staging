package main

import (
	"golang.org/x/time/rate"
)

// How much one session may send to its TURN relay, in bytes per second. Zero
// leaves it alone. Set once from -rate-up before any session starts.
//
// Per session because that is what VK allocates and meters: several sessions
// share one relay address, so a per-address limit would describe nothing VK
// sees.
var sessionSendLimit int

// About a sixth of a second of queue at the configured rate, never less than
// one packet. Sized in time because a fixed 128 packets is most of a minute
// at a call's rate, which the flow inside answers with RTO backoff rather
// than with a smaller window.
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
// The burst holds at least one whole packet: a bucket that cannot would never
// release one, and the session would stall rather than slow down.
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
