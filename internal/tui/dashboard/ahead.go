package dashboard

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// readers are what the pull requests and issues of the work pane are read
// ahead through.
type readers struct {
	pulls  details.Pulls
	issues details.Issues
}

// WithDetails gives the work pane pulls and issues to read its pull
// requests and issues ahead through. Whether and how it reads them is
// WithPrefetch's, which the settings may change while the app runs.
func WithDetails(pulls details.Pulls, issues details.Issues) Option {
	return func(s *Section) { s.readers = &readers{pulls: pulls, issues: issues} }
}

// WithPrefetch reads the work pane ahead as p says of the dashboard's
// waiting_on_you, while the pane has the focus: the row under the cursor
// and a window of rows around it, each time the cursor rests, so that they
// open at once. A pull request or an issue costs two requests, the detail
// and the first comments; what is cached is skipped. The default reads
// nothing ahead. The opener reads the inbox's threads ahead, as
// prefetch.dashboard.inbox says, while the dashboard is on view, and the
// repositories and pinned panes read through WithLanding's, as
// prefetch.dashboard.repositories and pinned say, while they have the
// focus.
func WithPrefetch(p config.PrefetchLayers) Option {
	return func(s *Section) { s.layers = p }
}

// WithSlots bounds the reads ahead of the dashboard's panes with those of
// every page and modal that shares s, so that together they keep to
// prefetch.parallel. Without it, each kind it reads has slots of its own.
// The inbox reads through the opener that WithOpener gives, which shares
// the session's slots itself.
func WithSlots(s *ui.Slots) Option {
	return func(x *Section) { x.slots = s }
}

// setPrefetch takes how the work and the repositories are read ahead
// from p. The opener takes the inbox's settings.
func (s *Section) setPrefetch(p config.PrefetchLayers) {
	s.aheadRepos.Configure(ui.Resolve(p, "dashboard", "repositories"))
	s.aheadPinned.Configure(ui.Resolve(p, "dashboard", "pinned"))
	work := ui.Resolve(p, "dashboard", "waiting_on_you")
	s.workAhead = work
	switch {
	case !work.Enabled:
		// Resetting cancels the reads in flight.
		s.ahead.Reset(s.ctx)
		s.ahead = nil
	case s.readers == nil:
		// Nothing was given to read with.
	case s.ahead == nil:
		s.ahead = details.NewAhead("work", s.readers.pulls, s.readers.issues, 0, work.Rest)
		s.ahead.Share(s.slots)
		s.ahead.Configure(work)
		s.ahead.Reset(s.ctx)
	default:
		s.ahead.Configure(work)
	}
}

// workAt returns the key of row i of the work on view.
func (s *Section) workAt(i int) (details.Key, bool) {
	t := s.tasks.current()
	if i >= t.items {
		return details.Key{}, false
	}
	return details.Of(*t.rows[i].hit)
}

// readWorkAhead reads the work around the cursor ahead, while the
// dashboard is on view and the work pane has the focus, once the cursor
// rests, if the rows around it changed since the last message.
func (s *Section) readWorkAhead() tea.Cmd {
	if s.ahead == nil || !s.started {
		return nil
	}
	if !s.focused || s.focus != workPane || !s.work.ok {
		// A rest that was due no longer reads anything.
		return s.ahead.Window(nil, -1)
	}
	return s.ahead.Window(s.workAt, s.tasks.current().sel)
}

// openHit returns the command that opens the pull request or issue of hit
// in its modal, on its checks if checks is set, which holds the reads
// ahead of the work while it loads.
func (s *Section) openHit(hit core.SearchHit, checks bool) tea.Cmd {
	is := hit.Issue
	var pause ui.Pauser
	if s.ahead != nil {
		pause = s.ahead
		if k, ok := details.Of(hit); ok {
			s.ahead.Opened(k)
		}
	}
	var msg tea.Msg = ui.OpenIssueMsg{Repo: is.Repo, Number: is.Number, Pause: pause}
	if hit.Kind == core.SearchPulls {
		msg = ui.OpenPullMsg{Repo: is.Repo, Number: is.Number, Checks: checks, Pause: pause}
	}
	return func() tea.Msg { return msg }
}
