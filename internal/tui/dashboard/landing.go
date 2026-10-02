package dashboard

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Landing reads what the repository screen reads first when it opens a
// repository, so that a repository read ahead opens at once.
type Landing interface {
	// Read reads what opening repo reads first into the caches the
	// repository screen reads from.
	Read(ctx context.Context, repo core.RepoRef) error
	// Cached reports whether that is in memory already. It does no I/O.
	Cached(repo core.RepoRef) bool
}

// WithLanding gives the repositories and pinned panes l to read ahead
// what opening the repositories around their cursors reads first. Whether
// and how far they read is WithPrefetch's, as prefetch.dashboard's
// repositories and pinned say. Without it, they read nothing ahead.
func WithLanding(l Landing) Option {
	return func(s *Section) { s.landing = l }
}

// newLandingAheads makes the reads ahead of the repositories and pinned
// panes, if the section has a Landing to read with.
func (s *Section) newLandingAheads() {
	if s.landing == nil {
		return
	}
	s.aheadRepos = ui.NewAhead("repo", s.landing.Read, s.landing.Cached, 0, 0)
	s.aheadRepos.Share(s.slots)
	s.aheadRepos.Reset(s.ctx)
	s.aheadPinned = ui.NewAhead("pinned_repo", s.landing.Read, s.landing.Cached, 0, 0)
	s.aheadPinned.Share(s.slots)
	s.aheadPinned.Reset(s.ctx)
}

// readReposAhead reads ahead the repositories around the cursor of the
// repositories pane, while it has the focus, once the cursor rests. The
// repository under the cursor is left to opening it.
func (s *Section) readReposAhead() tea.Cmd {
	if s.aheadRepos == nil || !s.started {
		return nil
	}
	f := &s.repos.current().feed
	if _, ok := f.Selected(); !ok || !s.focused || s.focus != reposPane {
		// A rest that was due no longer reads anything.
		return s.aheadRepos.Window(nil, -1)
	}
	i := f.Index()
	return s.aheadRepos.Window(func(j int) (core.RepoRef, bool) {
		r, ok := f.Item(j)
		return r.Ref, ok && j != i
	}, i)
}

// readPinnedAhead reads ahead the cards around the cursor of the pinned
// pane, in the order the pane lists them, while it has the focus, once
// the cursor rests. The card under the cursor is left to opening it.
func (s *Section) readPinnedAhead() tea.Cmd {
	if s.aheadPinned == nil || !s.started {
		return nil
	}
	c := &s.pinned
	if _, ok := c.selected(); !ok || !s.focused || s.focus != pinnedPane {
		return s.aheadPinned.Window(nil, -1)
	}
	return s.aheadPinned.Window(func(j int) (core.RepoRef, bool) {
		if j == c.sel || j >= len(c.items) {
			return core.RepoRef{}, false
		}
		return c.items[j].repo.Ref, true
	}, c.sel)
}

// setPinned lists pinned in the pinned pane, and stops the reads ahead of
// the cards that were listed if the repositories changed.
func (s *Section) setPinned(pinned []core.Repo) {
	if s.pinned.set(pinned) {
		s.aheadPinned.Reset(s.ctx)
	}
}
