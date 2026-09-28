package dashboard

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// aheadRows is how many of the first rows of the work on view are read
// ahead.
const aheadRows = 3

// prefetch is how the work is read ahead, and whether it is.
type prefetch struct {
	pulls  details.Pulls
	issues details.Issues
	delay  time.Duration
	on     bool
}

// WithPrefetch reads the pull requests and issues of the work pane ahead
// through pulls and issues while the pane has the focus: the first rows of
// the list on view, and the row under the cursor once it has rested there
// for delay, so that they open at once. Each costs two requests, the
// detail and the first comments; what is cached is skipped. The default
// reads nothing ahead.
func WithPrefetch(pulls details.Pulls, issues details.Issues, delay time.Duration) Option {
	return func(s *Section) { s.prefetch = &prefetch{pulls: pulls, issues: issues, delay: delay, on: true} }
}

// WithDetails gives the work pane pulls and issues to read its pull
// requests and issues ahead through, without reading them ahead, so that
// the settings may turn that on while the app runs. WithPrefetch gives
// them too, and wins.
func WithDetails(pulls details.Pulls, issues details.Issues) Option {
	return func(s *Section) {
		if s.prefetch == nil {
			s.prefetch = &prefetch{pulls: pulls, issues: issues}
		}
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

// readWorkAhead reads the first rows of the work on view and the row under
// the cursor ahead, while the dashboard is on view and the work pane has
// the focus, if they changed since the last message.
func (s *Section) readWorkAhead() tea.Cmd {
	if s.ahead == nil || !s.started {
		return nil
	}
	if !s.focused || s.focus != workPane {
		// The delay of a row the cursor rested on no longer reads it.
		return s.ahead.Moved(details.Key{}, false)
	}
	first := s.ahead.First(s.workAt)
	hit, ok := s.tasks.selected()
	k, readable := details.Of(hit)
	hover := s.ahead.Moved(k, ok && readable)
	switch {
	case first == nil:
		return hover
	case hover == nil:
		return first
	}
	return tea.Batch(first, hover)
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
