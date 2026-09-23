package watch

import "time"

// Defaults used when no option overrides them.
const (
	DefaultInterval       = time.Minute
	DefaultMinInterval    = 10 * time.Second
	DefaultMaxBackoff     = 10 * time.Minute
	DefaultIdleMultiplier = 4
)

type config struct {
	interval    time.Duration
	minInterval time.Duration
	maxBackoff  time.Duration
	idle        int
}

// Option configures an Engine.
type Option func(*config)

// WithInterval sets the interval used when a poll returns no server hint.
// Values <= 0 are ignored.
func WithInterval(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.interval = d
		}
	}
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
// the engine is inactive. Values < 1 are ignored.
func WithIdleMultiplier(n int) Option {
	return func(c *config) {
		if n >= 1 {
			c.idle = n
		}
	}
}
