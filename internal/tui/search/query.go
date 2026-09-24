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

// edited starts the wait after an edit of the query, after which the page
// searches for it.
func (s *Section) edited() tea.Cmd {
	s.seq++
	if s.debounce == 0 {
		return s.settle()
	}
	id, seq := s.id, s.seq
	return tea.Tick(s.debounce, func(time.Time) tea.Msg { return debounceMsg{id: id, seq: seq} })
}

// settle shows the results of the query as it is now. Repositories, issues
// and pull requests are searched at once; code waits to be asked for.
func (s *Section) settle() tea.Cmd {
	text := strings.Join(strings.Fields(s.input.Value()), " ")
	if text == s.text {
		return nil
	}
	// What is still read for the text before is of no use now.
	s.cancelText()
	s.textCtx, s.cancelText = context.WithCancel(s.ctx)
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
	return tea.Batch(s.ensureHits(k), s.prefetch(k))
}

// submit searches for the query at once, code too when its kind is on
// view, remembers it, and moves the focus to the results.
func (s *Section) submit() tea.Cmd {
	s.seq++
	cmd := s.settle()
	if s.text == "" {
		return cmd
	}
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

// setQuery puts text in the query and searches for it, as when a recent
// search is picked.
func (s *Section) setQuery(text string) tea.Cmd {
	s.input.SetValue(text)
	s.input.CursorEnd()
	return s.submit()
}
