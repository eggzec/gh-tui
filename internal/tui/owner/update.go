package owner

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Update handles the page's keys, the account the app gives it and its
// reads, and passes everything else to the lists, which ignore the
// messages of others.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	updating := s.updating()
	cmd, all := s.update(msg)
	if all {
		s.render()
		return cmd
	}
	if s.updating() != updating {
		// The profile says whether the list is read again.
		s.head = s.profile()
	}
	s.renderPane(listPane)
	s.compose()
	return cmd
}

// update handles msg, and reports whether more than the list may look
// different.
func (s *Section) update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case ui.OwnerMsg:
		return s.open(msg.Login), true
	case loadedMsg:
		return s.loaded(msg), true
	case tea.KeyPressMsg:
		return s.press(msg), true
	case ui.SettingsMsg:
		s.configure(msg.Config)
		return nil, true
	case ui.OnlineMsg:
		return s.online(), true
	case ui.ImagesMsg:
		// The profile may have gained or lost the avatar's rows.
		s.layout()
		return nil, true
	}
	pages := s.pages()
	cmds := make([]tea.Cmd, 0, len(pages))
	for _, p := range pages {
		if l := p.repos; l != nil {
			var cmd tea.Cmd
			l.Feed, cmd = l.Feed.Update(msg)
			cmds = append(cmds, cmd)
			if l.Remeasure(s.icons) {
				s.layoutList(l)
			}
		}
	}
	return tea.Batch(cmds...), false
}

// press handles a key: the focused pane takes its own, then the page's,
// then the list's navigation.
func (s *Section) press(msg tea.KeyPressMsg) tea.Cmd {
	p := s.page
	if !s.focused || p == nil {
		return nil
	}
	if cmd, ok := s.pressPane(msg); ok {
		return cmd
	}
	k := &s.keys
	switch {
	case key.Matches(msg, k.Next):
		s.setFocus((p.focus + 1) % numPanes)
		return nil
	case key.Matches(msg, k.Prev):
		s.setFocus((p.focus + numPanes - 1) % numPanes)
		return nil
	case s.wide && key.Matches(msg, k.Zoom):
		s.setZoom(!s.zoom)
		return nil
	case key.Matches(msg, k.Back):
		return s.stepBack()
	case key.Matches(msg, k.Refresh):
		return s.refresh()
	}
	if i := k.pane(msg); i >= 0 {
		s.setFocus(i)
		return nil
	}
	if l := p.repos; l != nil && p.focus == listPane {
		var cmd tea.Cmd
		l.Feed, cmd = l.Feed.Update(msg)
		return cmd
	}
	return nil
}

// stepBack takes one step back: out of the zoom, or else to the page this one
// was opened from, or else to the screen before the page.
func (s *Section) stepBack() tea.Cmd {
	switch {
	case s.zoomed():
		s.setZoom(false)
		return nil
	case s.goBack():
		// The page gone back to may have gone stale meanwhile.
		return s.Revisit()
	}
	return func() tea.Msg { return ui.BackMsg{} }
}

// pressPane handles the keys of the focused pane, and reports whether msg
// was one.
func (s *Section) pressPane(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	k, p := &s.keys, s.page
	switch p.focus {
	case pinnedPane:
		c := &p.pinned
		switch {
		case key.Matches(msg, k.Left):
			c.Move(-1)
		case key.Matches(msg, k.Right):
			c.Move(1)
		case key.Matches(msg, k.Up):
			c.Move(-c.Cols)
		case key.Matches(msg, k.Down):
			c.Move(c.Cols)
		case key.Matches(msg, k.Select):
			if it, ok := c.Selected(); ok {
				return selectRepo(it.Repo.Ref), true
			}
		case key.Matches(msg, k.Open):
			if it, ok := c.Selected(); ok {
				return ui.Open(s.repoURL(it.Repo)), true
			}
		default:
			return nil, false
		}
		return nil, true
	case listPane:
		l := p.repos
		if l == nil {
			return nil, false
		}
		switch {
		case key.Matches(msg, k.ClearFilter):
			return s.setFilter(""), true
		case key.Matches(msg, k.Filter, k.Sort):
			// The app opens the filter. Its keys don't reach the list,
			// whose page down f is too.
			return nil, true
		case key.Matches(msg, k.Select):
			if r, ok := l.Feed.Selected(); ok {
				return selectRepo(r.Ref), true
			}
			return nil, true
		case key.Matches(msg, k.Open):
			if r, ok := l.Feed.Selected(); ok {
				return ui.Open(s.repoURL(r)), true
			}
			return nil, true
		}
	default:
	}
	return nil, false
}

// setFocus moves the focus of the page on view to pane p.
func (s *Section) setFocus(p paneID) {
	s.page.focus = p
	s.focusPane()
}

// focusPane focuses the list of the page on view while the page and its
// pane are focused, and blurs it otherwise.
func (s *Section) focusPane() {
	p := s.page
	if p == nil || p.repos == nil {
		return
	}
	if s.focused && p.focus == listPane {
		p.repos.Feed.Focus()
		return
	}
	p.repos.Feed.Blur()
}

// blurPage blurs the list of p, which leaves the screen.
func (s *Section) blurPage(p *page) {
	if p != nil && p.repos != nil {
		p.repos.Feed.Blur()
	}
}

// repoURL is the page of r on GitHub.
func (s *Section) repoURL(r core.Repo) string {
	if r.URL != "" {
		return r.URL
	}
	return ui.WebURL(s.host, r.Ref.String())
}

// selectRepo returns the command that opens repo on the repository screen.
func selectRepo(repo core.RepoRef) tea.Cmd {
	return func() tea.Msg { return ui.RepoMsg{Repo: repo} }
}
