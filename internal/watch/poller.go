package watch

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

type poller struct {
	key    string
	fn     PollFunc
	wake   chan struct{}
	refs   int
	cancel context.CancelFunc
	// unreached is whether the last poll got no answer from GitHub, so
	// that it backs off until GitHub answers again.
	unreached atomic.Bool
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
		pctx := obs.ForBackground(obs.WithTrace(ctx, "sync.poll"))
		start := time.Now()
		res, err := p.fn(pctx)
		if ctx.Err() != nil {
			return
		}
		p.unreached.Store(unreached(err))
		switch {
		case err == nil:
			failures = 0
			hint = res.Interval
		case !errors.Is(err, core.ErrRateLimited):
			// A rate limit backs nothing off: the client holds the next
			// poll until the limit lifts, and a backoff would only poll
			// later than that; the poll waits for its Reset below.
			failures++
		}
		if err != nil || res.Changed {
			e.publish(p, Event{Key: p.key, Err: err})
		}
		next := e.delay(hint, failures)
		if rl, ok := errors.AsType[*core.RateLimitError](err); ok {
			// One the client didn't see coming, such as another's use of
			// the quota, lifts at its Reset, and polling sooner is wasted.
			next = max(next, time.Until(rl.Reset))
		}
		logPoll(pctx, p.key, start, res, err, failures, next)
		timer.Reset(next)
	}
}

// unreached reports whether err is of a poll that got no answer from
// GitHub, or only a server error.
func unreached(err error) bool {
	return errors.Is(err, core.ErrOffline) || errors.Is(err, core.ErrUnavailable)
}

// delay returns how long to wait before the next poll, given the last server
// hint and the number of consecutive failures.
func (e *Engine) delay(hint time.Duration, failures int) time.Duration {
	e.mu.Lock()
	d, active := e.cfg.interval, e.active
	e.mu.Unlock()
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
	if !active {
		d *= time.Duration(e.cfg.idle)
	}
	return d
}

// logPoll logs a poll of key: at debug level if nothing changed, at info
// level if something did, and at warn level if it failed, with the backoff
// that failures grew the next delay to.
func logPoll(ctx context.Context, key string, start time.Time, res Result, err error, failures int, next time.Duration) {
	level := slog.LevelDebug
	switch {
	case err != nil:
		level = slog.LevelWarn
	case res.Changed:
		level = slog.LevelInfo
	}
	if !obs.Enabled(ctx, level) {
		return
	}
	attrs := []slog.Attr{
		slog.String("span", "sync.poll"),
		slog.String("key", key),
		slog.Float64("duration_ms", obs.Millis(time.Since(start))),
		slog.Bool("changed", res.Changed),
		slog.Float64("next_in_s", next.Round(time.Second).Seconds()),
	}
	if res.Interval > 0 {
		attrs = append(attrs, slog.Float64("poll_interval_s", res.Interval.Seconds()))
	}
	if err != nil {
		attrs = append(attrs, slog.String("err", err.Error()), slog.Int("failures", failures))
	}
	slog.LogAttrs(ctx, level, "sync poll", attrs...)
}
