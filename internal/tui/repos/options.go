package repos

import (
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// Option configures a Section in [New].
type Option func(*Section)

// WithCurrent sets the repository the app starts with, so its row is marked
// as current.
func WithCurrent(ref core.RepoRef) Option {
	return func(s *Section) { s.current = ref }
}

// WithNow sets the clock that ages are measured against. The default is
// time.Now.
func WithNow(now func() time.Time) Option {
	return func(s *Section) {
		if now != nil {
			s.now = now
		}
	}
}
