package watch

import (
	"context"
	"log/slog"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
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
		pctx := obs.ForBackground(obs.WithTrace(ctx, "sync.poll"))
		start := time.Now()
		res, err := p.fn(pctx)
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
		next := e.delay(hint, failures)
		logPoll(pctx, p.key, start, res, err, failures, next)
		timer.Reset(next)
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
