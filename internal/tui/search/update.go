package search

import (
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Update handles the page's keys, the waits after typing and the
// countdown of code search, and passes everything else to the results,
// whose lists ignore the messages of others.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	cmd := s.update(msg)
	cmd = tea.Batch(cmd, s.spinTitle())
	s.render()
	return cmd
}

func (s *Section) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return s.press(msg)
	case debounceMsg:
		if msg.id != s.id || msg.seq != s.seq {
			return nil
		}
		return s.settle()
	case codeTickMsg:
		if msg.id != s.id {
			return nil
		}
		return s.ticked()
	case startMsg:
		if msg.id == s.id {
			s.started(msg)
		}
		return nil
	case prefetchMsg:
		if msg.id == s.id && msg.text == s.text {
			s.refreshCounts()
		}
		return nil
	case spinner.TickMsg:
		if msg.ID == s.spin.ID() {
			if _, ok := s.staleView(); !ok {
				s.spinning = false
				return nil
			}
			var cmd tea.Cmd
			s.spin, cmd = s.spin.Update(msg)
			return cmd
		}
	}
	cmds := make([]tea.Cmd, 0, len(s.hits)+2)
	for _, l := range s.hits {
		var cmd tea.Cmd
		l.feed, cmd = l.feed.Update(msg)
		cmds = append(cmds, cmd)
	}
	if s.code != nil {
		var cmd tea.Cmd
		s.code.feed, cmd = s.code.feed.Update(msg)
		cmds = append(cmds, cmd, s.codeFailed())
	}
	s.refreshCounts()
	return tea.Batch(cmds...)
}

// press handles a key in the part of the page that has the focus.
func (s *Section) press(msg tea.KeyPressMsg) tea.Cmd {
	if !s.focused {
		return nil
	}
	k := &s.keys
	switch s.area {
	case inputArea:
		switch {
		case key.Matches(msg, typing(k.Back)):
			return back
		case key.Matches(msg, typing(k.Select)):
			return s.submit()
		case key.Matches(msg, typing(k.Next)), key.Matches(msg, arrowUp):
			s.focusArea(kindsArea)
			return s.settleNow()
		case key.Matches(msg, typing(k.Prev)), key.Matches(msg, arrowDown):
			s.focusArea(resultsArea)
			return s.settleNow()
		}
		before := s.input.Value()
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		if s.input.Value() != before {
			return tea.Batch(cmd, s.edited())
		}
		return cmd
	case kindsArea:
		switch {
		case key.Matches(msg, k.Back):
			return back
		case key.Matches(msg, k.Up):
			return s.moveKind(-1)
		case key.Matches(msg, k.Down):
			return s.moveKind(1)
		case key.Matches(msg, k.Select), key.Matches(msg, k.Right), key.Matches(msg, k.Next):
			s.focusArea(resultsArea)
			if s.kind == core.SearchCode {
				return s.searchCode()
			}
		case key.Matches(msg, k.Prev):
			s.focusArea(inputArea)
		}
		return nil
	case resultsArea:
		return s.pressResults(msg)
	}
	return nil
}

// settleNow searches for the query without waiting for the debounce.
func (s *Section) settleNow() tea.Cmd {
	s.seq++
	return s.settle()
}

func back() tea.Msg { return ui.BackMsg{} }

// moveKind shows the kind delta kinds away.
func (s *Section) moveKind(delta int) tea.Cmd {
	i := slices.Index(kinds, s.kind) + delta
	if i < 0 || i >= len(kinds) {
		return nil
	}
	return s.showKind(kinds[i])
}

func (s *Section) pressResults(msg tea.KeyPressMsg) tea.Cmd {
	k := &s.keys
	switch {
	case key.Matches(msg, k.Back):
		return back
	case key.Matches(msg, k.Left), key.Matches(msg, k.Prev):
		s.focusArea(kindsArea)
		return nil
	case key.Matches(msg, k.Next):
		s.focusArea(inputArea)
		return nil
	case key.Matches(msg, k.Select):
		return s.open(false)
	case key.Matches(msg, k.Open):
		return s.open(true)
	case key.Matches(msg, k.Refresh):
		return s.refresh()
	}
	if s.text == "" {
		switch {
		case key.Matches(msg, k.Up):
			s.starts.move(-1)
		case key.Matches(msg, k.Down):
			s.starts.move(1)
		}
		return nil
	}
	if l, ok := s.visibleHits(); ok {
		var cmd tea.Cmd
		l.feed, cmd = l.feed.Update(msg)
		return cmd
	}
	if l, ok := s.visibleCode(); ok {
		var cmd tea.Cmd
		l.feed, cmd = l.feed.Update(msg)
		return cmd
	}
	return nil
}

// refresh searches again for the query, from GitHub.
func (s *Section) refresh() tea.Cmd {
	if s.text == "" {
		return nil
	}
	s.svc.Invalidate()
	clear(s.hits)
	kind := s.kind
	if kind == core.SearchCode {
		s.code = nil
	}
	return s.showKind(kind)
}

// open opens the result under the cursor: a repository on its screen, an
// issue or pull request in its modal, a file in the preview of its
// repository. With browser set, it opens it on GitHub instead.
func (s *Section) open(browser bool) tea.Cmd {
	if s.text == "" {
		it, ok := s.starts.selected()
		switch {
		case !ok:
			return nil
		case it.query != "":
			return s.setQuery(it.query)
		case browser:
			return ui.Open(repoURL(*it.repo))
		}
		return selectRepo(it.repo.Ref)
	}
	if l, ok := s.visibleHits(); ok {
		hit, ok := l.feed.Selected()
		if !ok {
			return nil
		}
		s.remember(s.text)
		if browser {
			return ui.Open(hitURL(hit))
		}
		return openHit(hit)
	}
	if l, ok := s.visibleCode(); ok {
		hit, ok := l.feed.Selected()
		if !ok {
			return nil
		}
		s.remember(s.text)
		if browser {
			return ui.Open(hit.URL)
		}
		// The repository opens first, so the preview opens over it.
		file := ui.OpenFileMsg{Repo: hit.Repo, Path: hit.Path, SHA: hit.SHA}
		return tea.Sequence(selectRepo(hit.Repo), func() tea.Msg { return file })
	}
	return nil
}

// focusArea moves the focus to a, and focuses the bubble in it.
func (s *Section) focusArea(a area) {
	s.area = a
	if s.focused && a == inputArea {
		s.input.Focus()
	} else {
		s.input.Blur()
	}
	results := s.focused && a == resultsArea
	for kind, l := range s.hits {
		if results && kind == s.kind {
			l.feed.Focus()
		} else {
			l.feed.Blur()
		}
	}
	if s.code != nil {
		if results && s.kind == core.SearchCode {
			s.code.feed.Focus()
		} else {
			s.code.feed.Blur()
		}
	}
}

func selectRepo(repo core.RepoRef) tea.Cmd {
	return func() tea.Msg { return ui.RepoMsg{Repo: repo} }
}

func openHit(hit core.SearchHit) tea.Cmd {
	var msg tea.Msg
	switch hit.Kind {
	case core.SearchRepos:
		msg = ui.RepoMsg{Repo: hit.Repo.Ref}
	case core.SearchPulls:
		msg = ui.OpenPullMsg{Repo: hit.Issue.Repo, Number: hit.Issue.Number}
	default:
		msg = ui.OpenIssueMsg{Repo: hit.Issue.Repo, Number: hit.Issue.Number}
	}
	return func() tea.Msg { return msg }
}

func hitURL(hit core.SearchHit) string {
	if hit.Kind == core.SearchRepos {
		return repoURL(hit.Repo)
	}
	return hit.Issue.URL
}

func repoURL(r core.Repo) string {
	if r.URL != "" {
		return r.URL
	}
	return "https://github.com/" + r.Ref.String()
}
