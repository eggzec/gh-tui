package issues

import (
	"time"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

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

// WithFilterPrefetch reads the first page of each filter not shown once the
// list of a repository loads, so that switching filters shows it at once.
// Each costs a request; pages cached fresh are skipped. The default reads
// nothing ahead.
func WithFilterPrefetch() Option {
	return func(s *Section) { s.prefetchFilters = true }
}

// WithIcons sets the glyphs of the states of issues. The default is the
// Nerd Font set.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// prefetch is how the issues are read ahead.
type prefetch struct {
	rows  int
	delay time.Duration
}

// WithOffline shares off with other sections, so that the user is told once
// for all of them that GitHub can't be reached. By default the section has
// its own.
func WithOffline(off *ui.Offline) Option {
	return func(s *Section) {
		if off != nil {
			s.offline = off
		}
	}
}
