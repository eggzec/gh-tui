package repos

import (
	"slices"
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

// WithPinned sets the repositories listed first, in order and marked as
// pinned, such as the ones in the config. The viewer's repositories follow
// without them.
func WithPinned(refs []core.RepoRef) Option {
	return func(s *Section) {
		var pinned []core.RepoRef
		for _, ref := range refs {
			if !slices.ContainsFunc(pinned, func(p core.RepoRef) bool { return sameRef(p, ref) }) {
				pinned = append(pinned, ref)
			}
		}
		s.pinned = pinned
	}
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
