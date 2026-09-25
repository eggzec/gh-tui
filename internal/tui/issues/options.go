package issues

import (
	"context"
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

// WithFilterPrefetch reads the first page of each state not shown once the
// list of a repository loads, so that switching tabs shows it at once. Each
// costs a request; pages cached fresh are skipped, and so is a list the
// user filtered. The default reads nothing ahead.
func WithFilterPrefetch() Option {
	return func(s *Section) { s.prefetchFilters = true }
}

// WithIcons sets the glyphs of the states of issues. The default is the
// Nerd Font set.
func WithIcons(icons ui.Icons) Option {
	return func(s *Section) { s.icons = icons }
}

// WithRepos reads what the viewer may do in the repository of a modal
// from r, when it isn't the selected one, whose caps the app sends in a
// ui.CapsMsg. Until they are known, every change is offered, and GitHub
// refuses what it doesn't allow.
func WithRepos(r ui.Repos) Option {
	return func(s *Section) { s.repos = r }
}

// Viewer returns the login of the signed-in user. It may do I/O.
type Viewer func(ctx context.Context) (string, error)

// WithViewer sets how the section learns who the user is, once it starts,
// so that they may close and reopen their own issues in a repository they
// can only read. Without it, or until it answers, those changes are
// offered on every issue.
func WithViewer(v Viewer) Option {
	return func(s *Section) { s.readViewer = v }
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
