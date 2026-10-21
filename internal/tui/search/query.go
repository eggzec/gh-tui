package search

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// adopt makes the query as it is now the one the results are for, and
// shows its results of the kind on view only. Code waits to be asked for;
// the other kinds are read ahead by [Section.readOthers].
func (s *Section) adopt() tea.Cmd {
	text := strings.Join(strings.Fields(s.input.Value()), " ")
	if text == s.text {
		return nil
	}
	// What is still read for the text before is of no use now.
	s.cancelText()
	s.textCtx, s.cancelText = context.WithCancel(s.ctx)
	s.ahead.Reset(s.textCtx)
	s.text = text
	s.refreshCounts()
	if text == "" {
		return nil
	}
	// Code waits to be asked for, but the kinds still count, which the
	// search of repositories brings.
	k := s.kind
	if k == core.SearchCode {
		k = core.SearchRepos
	}
	return s.ensureHits(k)
}

// submit searches for the query, code too when its kind is on view,
// remembers it, and moves the focus to the results. Typing only edits the
// text: nothing is searched until this runs.
func (s *Section) submit() tea.Cmd {
	cmd := s.adopt()
	if s.text == "" {
		return cmd
	}
	cmd = tea.Batch(cmd, s.readOthers(othersEnter))
	s.remember(s.text)
	if s.kind == core.SearchCode {
		cmd = tea.Batch(cmd, s.searchCode())
	}
	s.focusArea(resultsArea)
	return cmd
}

// remember keeps text as the most recent search.
func (s *Section) remember(text string) {
	s.recent = slices.DeleteFunc(s.recent, func(r string) bool { return strings.EqualFold(r, text) })
	s.recent = slices.Insert(s.recent, 0, text)
	if len(s.recent) > maxRecent {
		s.recent = s.recent[:maxRecent]
	}
	s.starts.build(s.recent)
}

// Search puts query in the search box and searches for it, as typing it
// and pressing enter would, such as for the search command. The page must
// have the focus.
func (s *Section) Search(query string) tea.Cmd {
	cmd := tea.Batch(s.setQuery(query), s.spinTitle(), s.readAhead())
	s.render()
	return cmd
}

// setQuery puts text in the query and searches for it, as when a recent
// search is picked.
func (s *Section) setQuery(text string) tea.Cmd {
	s.input.SetValue(text)
	s.input.CursorEnd()
	return s.submit()
}

// Fresh empties the query and drops its results, as a new search starts,
// and puts the focus back in the query when the page has it. The recent
// searches stay. The lists of the query before go too, so that typing it
// again reads it anew, not through a list whose reads were canceled.
func (s *Section) Fresh() {
	s.input.SetValue("")
	clear(s.hits)
	clear(s.stale)
	s.code = nil
	s.adopt()
	s.focusArea(inputArea)
	s.render()
}
