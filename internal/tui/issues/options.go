package issues

import "time"

// Option configures a Section.
type Option func(*Section)

// WithNow sets the clock that ages are measured against. Tests pin it.
func WithNow(now func() time.Time) Option {
	return func(s *Section) {
		if now != nil {
			s.now = now
		}
	}
}

// WithPrefetch reads the issue and the first comments of the first rows of
// each list once it loads, and of the row under the cursor once the cursor
// has rested on it for delay, so that they open at once. Each costs two
// requests; issues already cached are skipped. The default reads nothing
// ahead.
func WithPrefetch(rows int, delay time.Duration) Option {
	return func(s *Section) { s.prefetch = &prefetch{rows: rows, delay: delay} }
}

// prefetch is how the issues are read ahead.
type prefetch struct {
	rows  int
	delay time.Duration
}
