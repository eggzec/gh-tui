package watch

import "time"

// Defaults used when no option overrides them.
const (
	DefaultMinInterval = 10 * time.Second
	DefaultMaxBackoff  = 10 * time.Minute
)

// Kind names a kind of key that polls at an interval of its own, such as
// the notifications or a run in progress.
type Kind string

type config struct {
	interval    time.Duration
	intervals   map[Kind]time.Duration
	minInterval time.Duration
	maxBackoff  time.Duration
	idle        int
}

// every returns the interval of the keys of kind when a poll returns no
// server hint.
func (c config) every(kind Kind) time.Duration {
	if d, ok := c.intervals[kind]; ok {
		return d
	}
	return c.interval
}

// Option configures an Engine.
type Option func(*config)

// WithInterval sets the interval used when a poll returns no server hint,
// for the keys of a kind that WithIntervals leaves out. Without it, they
// poll as often as the minimum interval allows. Values <= 0 are ignored.
func WithInterval(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.interval = d
		}
	}
}

// WithIntervals sets the interval of the keys of each kind, used when a
// poll returns no server hint. Values <= 0 are ignored.
func WithIntervals(intervals map[Kind]time.Duration) Option {
	return func(c *config) { c.intervals = positive(intervals) }
}

// positive returns the entries of intervals that are positive, in a map
// of their own.
func positive(intervals map[Kind]time.Duration) map[Kind]time.Duration {
	out := make(map[Kind]time.Duration, len(intervals))
	for k, d := range intervals {
		if d > 0 {
			out[k] = d
		}
	}
	return out
}

// WithMinInterval sets the shortest interval between polls of one key, no
// matter what the default or the server hint says. Values <= 0 are ignored.
func WithMinInterval(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.minInterval = d
		}
	}
}

// WithMaxBackoff caps the interval that repeated errors can grow to. It never
// shortens the regular interval. Values <= 0 are ignored.
func WithMaxBackoff(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.maxBackoff = d
		}
	}
}

// WithIdleMultiplier sets the factor that intervals are multiplied by while
// the engine is inactive. Without it they aren't. Values < 1 are ignored.
func WithIdleMultiplier(n int) Option {
	return func(c *config) {
		if n >= 1 {
			c.idle = n
		}
	}
}
