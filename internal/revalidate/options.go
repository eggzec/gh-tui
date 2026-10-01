package revalidate

import "time"

// Defaults used when no option overrides them.
const (
	DefaultConcurrency = 2
	// DefaultStartDelay leaves the first reads of a session to the views,
	// which revalidate what they show, so the first pass finds those
	// entries current and doesn't ask GitHub about them twice.
	DefaultStartDelay = 2 * time.Second
	DefaultMaxBackoff = 10 * time.Minute
)

// Settings are what a Revalidator is told by the code that makes it, from
// the user's config. It has no defaults for them.
type Settings struct {
	// Interval is how long after a pass ends the next one starts. One
	// below a second counts as a second, so that passes never spin.
	Interval time.Duration
	// PerMinute is how many requests the revalidator sends in any minute
	// at most. A pass checks at most as many entries as that allows in an
	// interval, and leaves the rest to the passes after. Values below 1
	// count as 1.
	PerMinute int
	// Recent is how recently an entry must have been used for ScopeRecent
	// to check it. Zero leaves only the entries of the selected repository.
	Recent time.Duration
}

// Scope selects the entries a pass checks.
type Scope int

// Scopes.
const (
	// ScopeRecent checks the selected repository's entries, and the
	// others, including those of no repository, used within the recent
	// age.
	ScopeRecent Scope = iota
	// ScopeAll checks every entry.
	ScopeAll
)

type config struct {
	Settings
	concurrency int
	scope       Scope
	idle        int
	startDelay  time.Duration
	maxBackoff  time.Duration
	publish     func(key string)
	report      func(Pass)
}

// Option configures a Revalidator.
type Option func(*config)

// WithConcurrency sets how many requests are in flight at most. The
// default is DefaultConcurrency. Values < 1 are ignored.
func WithConcurrency(n int) Option {
	return func(c *config) {
		if n >= 1 {
			c.concurrency = n
		}
	}
}

// WithScope sets which entries a pass checks. The default is ScopeRecent.
func WithScope(s Scope) Option {
	return func(c *config) { c.scope = s }
}

// WithIdleMultiplier sets the factor that intervals are multiplied, and the
// budget divided, by while the revalidator is inactive. Without it they
// aren't. Values < 1 are ignored.
func WithIdleMultiplier(n int) Option {
	return func(c *config) {
		if n >= 1 {
			c.idle = n
		}
	}
}

// WithStartDelay sets how long after Run starts the first pass starts. The
// default is DefaultStartDelay. Values < 0 are ignored.
func WithStartDelay(d time.Duration) Option {
	return func(c *config) {
		if d >= 0 {
			c.startDelay = d
		}
	}
}

// WithMaxBackoff caps how far the interval grows while GitHub can't be
// reached. It never shortens the interval. The default is
// DefaultMaxBackoff. Values <= 0 are ignored.
func WithMaxBackoff(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.maxBackoff = d
		}
	}
}

// WithPublish sets the function that publishes the sync key of a change,
// such as to the sync engine, so that the views showing it read it again.
// Changes found close together are published together, each key once. By
// default changes are stored but not published.
func WithPublish(publish func(key string)) Option {
	return func(c *config) {
		if publish != nil {
			c.publish = publish
		}
	}
}

// WithReport sets a function that is told what each pass did, such as to
// log it. It is called in Run's goroutine.
func WithReport(report func(Pass)) Option {
	return func(c *config) {
		if report != nil {
			c.report = report
		}
	}
}
