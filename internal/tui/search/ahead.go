package search

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// WithDetails gives the results pulls and issues to read their pull
// requests and issues ahead through, as the settings say.
func WithDetails(pulls details.Pulls, issues details.Issues) Option {
	return func(s *Section) { s.reader = details.Reader{Pulls: pulls, Issues: issues} }
}

// WithPrefetch reads ahead as p says for prefetch.search, so that what the
// user opens next opens at once:
//   - details and comments: the pull request or issue of the results in
//     the window around the cursor, and its first comments, each time the
//     cursor rests while the results have the focus, through what
//     WithDetails gives. Each costs a request; what is cached is skipped.
//   - other_kinds: the first page of the kinds of results not on view,
//     once the query rests, so that switching kinds is instant. Each
//     costs a search.
//
// Without it, only the other kinds are read, as the defaults say.
func WithPrefetch(p config.PrefetchLayers) Option {
	return func(s *Section) { s.prefetch = &p }
}

// readAhead tells the reads ahead where the cursor is in the results on
// view, while they have the focus, which read the window around it once it
// rests there.
func (s *Section) readAhead() tea.Cmd {
	l, shown := s.visibleHits()
	if !shown || !s.focused || s.area != resultsArea || l.feed.Len() == 0 && !l.feed.Settled() {
		// The cursor left the results, so a rest still to come reads
		// nothing.
		return s.ahead.Window(nil, 0)
	}
	return s.ahead.Window(func(i int) (details.Key, bool) {
		hit, ok := l.feed.Item(i)
		if !ok {
			return details.Key{}, false
		}
		// A repository has nothing to read, and counts toward the window.
		return details.Of(hit)
	}, l.feed.Index())
}

// openHit returns the command that opens hit: a repository on its screen,
// and a pull request or issue in its modal, on its checks if checks is set,
// which holds the reads ahead of the results while it loads.
func (s *Section) openHit(hit core.SearchHit, checks bool) tea.Cmd {
	var pause ui.Pauser
	if s.ahead.On() {
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
