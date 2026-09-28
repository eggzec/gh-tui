package github

import (
	"context"
	"errors"
	"io"
	"time"
)

// errRecalled is the cause with which the gate ends an attempt that it let
// through, but that still waits for a slot, once a limit that came since
// stops it: rather than go out only to be refused, it goes back to the
// gate, which holds or fails it as its class may wait.
var errRecalled = errors.New("recalled by its rate limit")

// sendKey is the context key of the hook that the limiter calls once an
// attempt has its slot, just before it is sent.
type sendKey struct{}

// withSend returns ctx carrying send, which the limiter calls as the
// attempt of ctx goes out (markSent).
func withSend(ctx context.Context, send func() error) context.Context {
	return context.WithValue(ctx, sendKey{}, send)
}

// markSent tells the gate that the attempt of ctx is about to be sent, and
// returns errRecalled if the gate recalled it first, and then it mustn't
// be sent. An attempt that the gate didn't let through, or can't recall,
// is always sent.
func markSent(ctx context.Context) error {
	if send, ok := ctx.Value(sendKey{}).(func() error); ok {
		return send()
	}
	return nil
}

// track has r, whose attempt cancel ends, be recalled if a limit stops it
// before it is sent, and reports whether it may still be sent: false if
// it was recalled already, as it waited for its turn in the gate.
func (b *budget) track(r *reservation, cancel context.CancelCauseFunc) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.recalled {
		return false
	}
	r.cancel = cancel
	return true
}

// send marks r sent, unless it was recalled, and then returns errRecalled.
// Both happen under b.mu, so a request is either recalled or sent, never
// both: recall passes over one that is sent.
func (b *budget) send(r *reservation) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.recalled {
		return errRecalled
	}
	r.sent, r.cancel = true, nil
	return nil
}

// recall recalls each request counted but not sent yet that a limit now
// stops, and ends its attempt, which then goes back to the gate. b.mu
// must be held.
func (b *budget) recall(now time.Time) {
	for _, r := range b.pending {
		if r.sent || r.recalled || !b.stops(r, now) {
			continue
		}
		r.recalled = true
		if r.cancel != nil {
			r.cancel(errRecalled)
			r.cancel = nil
		}
	}
}

// stops reports whether a limit on now would stop r, were it to come to
// the gate: a secondary limit, or the quota of its resource while spent.
// A release that passed is a limit that may have lifted, which the
// request let go to find out mustn't be recalled for. b.mu must be held.
func (b *budget) stops(r *reservation, now time.Time) bool {
	if now.Before(b.gate.secondary) {
		return true
	}
	q := b.quotas[r.resource]
	return q != nil && q.release.After(now)
}

// sentBody ends the context of its attempt once it is closed.
type sentBody struct {
	io.ReadCloser
	cancel context.CancelCauseFunc
}

func (b *sentBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel(nil)
	return err
}
