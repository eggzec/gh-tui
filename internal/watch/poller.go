package watch

import (
	"context"
	"time"
)

type poller struct {
	key    string
	fn     PollFunc
	wake   chan struct{}
	refs   int
	cancel context.CancelFunc
}

// kick asks for an immediate poll. A poll that is already requested absorbs
// the new request.
func (p *poller) kick() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) poll(ctx context.Context, p *poller) {
	var (
		hint     time.Duration
		failures int
	)
	timer := time.NewTimer(e.delay(hint, failures))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-p.wake:
		}
		res, err := p.fn(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			failures++
		} else {
			failures = 0
			hint = res.Interval
		}
		if err != nil || res.Changed {
			e.publish(p, Event{Key: p.key, Err: err})
		}
		timer.Reset(e.delay(hint, failures))
	}
}

// delay returns how long to wait before the next poll, given the last server
// hint and the number of consecutive failures.
func (e *Engine) delay(hint time.Duration, failures int) time.Duration {
	d := e.cfg.interval
	if hint > 0 {
		d = hint
	}
	d = max(d, e.cfg.minInterval)

	// Backoff may exceed max backoff when the regular interval already does;
	// errors should never make polling faster.
	limit := max(e.cfg.maxBackoff, d)
	for range failures {
		if d >= limit {
			break
		}
		d *= 2
	}
	d = min(d, limit)

	e.mu.Lock()
	active := e.active
	e.mu.Unlock()
	if !active {
		d *= time.Duration(e.cfg.idle)
	}
	return d
}
