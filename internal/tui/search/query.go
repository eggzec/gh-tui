package search

import (
	"context"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// debounceMsg says the user stopped typing edit seq of the query, on the
// page with id.
type debounceMsg struct {
	id  int64
	seq int
}

// othersMsg says the query rested long enough after edit edits of it for
// the first pages of the kinds not on view to be read, on the page with id.
type othersMsg struct {
	id    int64
	edits int
}

// edited starts the wait after an edit of the query, after which the page
// searches for it, and the longer one after which it reads the other kinds
// ahead.
func (s *Section) edited() tea.Cmd {
	s.seq++
	s.edits++
	id, seq, edits := s.id, s.seq, s.edits
	// The other kinds wait at least as long as the kind on view.
	others := tea.Tick(max(s.othersWait, s.debounce), func(time.Time) tea.Msg { return othersMsg{id: id, edits: edits} })
	if s.debounce == 0 {
		return tea.Batch(s.settle(), others)
	}
	return tea.Batch(tea.Tick(s.debounce, func(time.Time) tea.Msg { return debounceMsg{id: id, seq: seq} }), others)
}

// settle shows the results of the query as it is now, of the kind on view
// only. Code waits to be asked for; the other kinds wait for the query to
// rest ([Section.readOthers]).
func (s *Section) settle() tea.Cmd {
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
	// Code waits to be asked for, but the kinds still count as the user
	// types, which the search of repositories brings.
	k := s.kind
	if k == core.SearchCode {
		k = core.SearchRepos
	}
	return s.ensureHits(k)
}

// submit searches for the query at once, code too when its kind is on
// view, remembers it, and moves the focus to the results.
func (s *Section) submit() tea.Cmd {
	s.seq++
	cmd := s.settle()
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
	s.seq++
	clear(s.hits)
	clear(s.stale)
	s.code = nil
	s.settle()
	s.focusArea(inputArea)
	s.render()
}
