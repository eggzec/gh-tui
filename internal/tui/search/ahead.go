package search

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// prefetch is how the results are read ahead, and whether they are.
type prefetch struct {
	pulls  details.Pulls
	issues details.Issues
	delay  time.Duration
	on     bool
}

// WithPrefetch reads the pull request or issue under the cursor ahead
// through pulls and issues, once the cursor has rested on it for delay
// while the results have the focus, so that it opens at once. It costs two
// requests, the detail and the first comments; what is cached is skipped.
// The first results aren't read, since a search is mostly a guess. The
// default reads nothing ahead.
func WithPrefetch(pulls details.Pulls, issues details.Issues, delay time.Duration) Option {
	return func(s *Section) { s.prefetch = &prefetch{pulls: pulls, issues: issues, delay: delay, on: true} }
}

// WithDetails gives the results pulls and issues to read their pull
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

// readAhead starts the delay after which the result under the cursor is
// read ahead, if it moved since the last message, while the results have
// the focus.
func (s *Section) readAhead() tea.Cmd {
	if s.ahead == nil {
		return nil
	}
	var (
		k  details.Key
		ok bool
	)
	if l, shown := s.visibleHits(); shown && s.focused && s.area == resultsArea {
		if hit, sel := l.feed.Selected(); sel {
			k, ok = details.Of(hit)
		}
	}
	return s.ahead.Moved(k, ok)
}

// openHit returns the command that opens hit: a repository on its screen,
// and a pull request or issue in its modal, on its checks if checks is set,
// which holds the reads ahead of the results while it loads.
func (s *Section) openHit(hit core.SearchHit, checks bool) tea.Cmd {
	var pause ui.Pauser
	if s.ahead != nil {
		pause = s.ahead
		if k, ok := details.Of(hit); ok {
			s.ahead.Opened(k)
		}
	}
	var msg tea.Msg
	switch hit.Kind {
	case core.SearchRepos:
		msg = ui.RepoMsg{Repo: hit.Repo.Ref}
	case core.SearchPulls:
		msg = ui.OpenPullMsg{Repo: hit.Issue.Repo, Number: hit.Issue.Number, Checks: checks, Pause: pause}
	default:
		msg = ui.OpenIssueMsg{Repo: hit.Issue.Repo, Number: hit.Issue.Number, Pause: pause}
	}
	return func() tea.Msg { return msg }
}
