package search

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// Update handles the page's keys and the countdown of code search, and
// passes everything else to the results, whose lists ignore the messages
// of others.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	cmd := s.update(msg)
	cmd = tea.Batch(cmd, s.spinTitle(), s.readAhead())
	s.render()
	return cmd
}

func (s *Section) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return s.press(msg)
	case ui.AheadMsg:
		if !s.focused {
			return nil
		}
		return s.ahead.Rested(msg)
	case codeTickMsg:
		if msg.id != s.id {
			return nil
		}
		return s.ticked()
	case ui.SettingsMsg:
		s.configure(msg.Config)
		return nil
	case ui.OnlineMsg:
		// A rate limit is the token's, and has lifted unless one holds.
		if !msg.Limited {
			s.ahead.Resume()
		}
		return s.online()
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
	if s.typing {
		return s.pressTyping(msg)
	}
	if cmd, ok := s.pressFeed(msg); ok {
		return cmd
	}
	k := &s.keys
	switch {
	case k.focusOf(msg) >= 0:
		s.focusArea(k.focusOf(msg))
		return nil
	case keymap.Matches(msg, k.Insert):
		s.startTyping()
		return nil
	case keymap.Matches(msg, k.NextTab):
		return s.moveKind(1)
	case keymap.Matches(msg, k.PrevTab):
		return s.moveKind(-1)
	case keymap.Matches(msg, k.Next), keymap.Matches(msg, k.Prev):
		s.cyclePane(keymap.Matches(msg, k.Next))
		return nil
	}
	if s.area == inputArea {
		switch {
		case keymap.Matches(msg, k.Select):
			return s.submit()
		case keymap.Matches(msg, k.Refresh):
			return s.refresh()
		}
		return nil
	}
	return s.pressResults(msg)
}

// pressTyping handles a key while the query types: the few keys that type
// nothing act, and the rest edit the query without searching it.
func (s *Section) pressTyping(msg tea.KeyPressMsg) tea.Cmd {
	k := &s.keys
	// A printable key is typed before any key that holds it.
	switch {
	case keyhelp.Printable(msg.String()):
	case keymap.Matches(msg, k.query.Cancel):
		s.focusArea(resultsArea)
		return nil
	case keymap.Matches(msg, k.query.Submit):
		return s.submit()
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return cmd
}

// startTyping focuses the query and makes it take the keys.
func (s *Section) startTyping() {
	s.focusArea(inputArea)
	s.typing = true
	s.input.Focus()
}

// moveKind shows the kind delta kinds away, going round at either end.
func (s *Section) moveKind(delta int) tea.Cmd {
	i := (slices.Index(kinds, s.kind) + delta + len(kinds)) % len(kinds)
	return s.showKind(kinds[i])
}

func (s *Section) pressResults(msg tea.KeyPressMsg) tea.Cmd {
	k := &s.keys
	switch {
	case keymap.Matches(msg, k.Select):
		return s.open(false)
	case keymap.Matches(msg, k.Open):
		return s.open(true)
	case keymap.Matches(msg, k.Checks):
		return s.openChecks()
	case keymap.Matches(msg, k.Refresh):
		return s.refresh()
	case keymap.Matches(msg, k.Filter):
		return ui.OpenFilter()
	case keymap.Matches(msg, k.Sort):
		return ui.OpenSort()
	case s.kind == core.SearchRepos && keymap.Matches(msg, k.GroupBy):
		return ui.OpenModal(s.groupModal())
	case s.kind == core.SearchRepos && keymap.Matches(msg, k.ToggleGroups):
		if !s.grouping.Toggle() {
			return ui.NothingToSwitch()
		}
		return s.regroup()
	}
	if s.text == "" {
		switch {
		case keymap.Matches(msg, k.feed.Up):
			s.starts.move(-1)
		case keymap.Matches(msg, k.feed.Down):
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
	s.ahead.Resume()
	clear(s.hits)
	kind := s.kind
	if kind == core.SearchCode {
		s.code = nil
	}
	return s.showKind(kind)
}

// open opens the result under the cursor: a repository on its screen,
// and an issue, pull request or file in a modal over the page, so that
// closing it comes back to the results. With browser set, it opens it on
// GitHub instead.
func (s *Section) open(browser bool) tea.Cmd {
	if s.text == "" {
		it, ok := s.starts.selected()
		switch {
		case !ok:
			return nil
		case it.query != "":
			return s.setQuery(it.query)
		case browser:
			return ui.Open(s.repoURL(*it.repo))
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
			return ui.Open(s.hitURL(hit))
		}
		return s.openHit(hit, false)
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
		file := ui.OpenFileMsg{Repo: hit.Repo, Path: hit.Path, SHA: hit.SHA, Find: firstMatch(hit)}
		return func() tea.Msg { return file }
	}
	if s.kind == core.SearchCode && !browser {
		// Code waits to be asked for.
		return s.searchCode()
	}
	return nil
}

// firstMatch returns the text of the first match of hit, on one line,
// which the preview of the file opens on.
func firstMatch(hit core.CodeHit) string {
	for _, f := range hit.Fragments {
		for _, m := range f.Matches {
			if m[0] < 0 || m[1] > len(f.Text) || m[0] >= m[1] {
				continue
			}
			text, _, _ := strings.Cut(f.Text[m[0]:m[1]], "\n")
			if text = strings.TrimSpace(text); text != "" {
				return text
			}
		}
	}
	return ""
}

// focusArea moves the focus to a, in normal mode, and focuses the bubble
// in it. Typing starts with startTyping.
func (s *Section) focusArea(a area) { s.focusPane(a, false) }

// focusPane is focusArea, with the focus on the preview of the results'
// area when preview is set.
func (s *Section) focusPane(a area, preview bool) {
	s.area = a
	s.preview = preview
	s.typing = false
	s.input.Blur()
	results := s.focused && a == resultsArea && !preview
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

// openChecks opens the pull request under the cursor on its checks.
func (s *Section) openChecks() tea.Cmd {
	l, ok := s.visibleHits()
	if !ok || s.text == "" {
		return nil
	}
	hit, ok := l.feed.Selected()
	if !ok || hit.Kind != core.SearchPulls {
		return nil
	}
	s.remember(s.text)
	return s.openHit(hit, true)
}

func (s *Section) hitURL(hit core.SearchHit) string {
	if hit.Kind == core.SearchRepos {
		return s.repoURL(hit.Repo)
	}
	return hit.Issue.URL
}

func (s *Section) repoURL(r core.Repo) string {
	if r.URL != "" {
		return r.URL
	}
	return ui.WebURL(s.host, r.Ref.String())
}
