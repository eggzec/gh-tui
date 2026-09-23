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
