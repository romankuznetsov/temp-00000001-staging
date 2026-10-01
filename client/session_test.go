package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/pion/stun/v3"
)

// The relay's answer is read out of the typed error rather than searched for
// in its text, because the text also carries the local socket address, and
// the ephemeral range is full of ports whose digits contain 401 and 486. A
// timeout on one of those used to be an authentication failure.
func TestStunErrorCodeIsReadNotSearchedFor(t *testing.T) {
	timeout := errors.New("read udp 0.0.0.0:54011->1.2.3.4:3478: i/o timeout")
	if isAuthError(timeout) {
		t.Error("a timeout on port 54011 was taken for an authentication failure")
	}
	if code := stunErrorCode(timeout); code != 0 {
		t.Errorf("a timeout was given STUN code %d", code)
	}

	quota := &stun.TurnError{ErrorCodeAttr: stun.ErrorCodeAttribute{Code: stun.CodeAllocQuotaReached}}
	if code := stunErrorCode(quota); code != stun.CodeAllocQuotaReached {
		t.Errorf("a 486 read as %d", code)
	}
	if isAuthError(quota) {
		t.Error("a quota refusal was taken for an authentication failure")
	}

	unauthorized := &stun.TurnError{ErrorCodeAttr: stun.ErrorCodeAttribute{Code: stun.CodeUnauthorized}}
	if !isAuthError(unauthorized) {
		t.Error("a 401 was not taken for an authentication failure")
	}
	// The session wraps what it returns, and the code has to survive that.
	if !isAuthError(fmt.Errorf("TURN Allocate: %w", unauthorized)) {
		t.Error("a wrapped 401 was lost")
	}
	if code := stunErrorCode(fmt.Errorf("TURN quota: %w", quota)); code != stun.CodeAllocQuotaReached {
		t.Errorf("a wrapped 486 read as %d", code)
	}
}
