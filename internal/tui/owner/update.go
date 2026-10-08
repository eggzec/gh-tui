package owner

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// Update handles the page's keys, the account the app gives it and its
// reads, and passes everything else to the lists, which ignore the
// messages of others.
func (s *Section) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(ui.AheadMsg); ok {
		return s.rested(msg)
	}
	updating := s.updating()
	cmd, all := s.update(msg)
	// What a change put on view, such as a pane focused or a header read,
	// is read now, and what the cursors rest on is read ahead.
	cmd = tea.Batch(cmd, s.startSide(), s.readAhead())
	if all {
		s.render()
		return cmd
	}
	if s.updating() != updating {
		// The profile says whether the list is read again.
		s.head = s.profile()
	}
	s.renderPane(listPane)
	s.renderPane(readmePane)
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
	case sideMsg:
		return s.sideLoaded(msg), true
	case ui.SyncMsg:
		return s.synced(msg), false
	case tea.KeyPressMsg:
		return s.press(msg), true
	case ui.SettingsMsg:
		s.configure(msg.Config)
		return nil, true
	case ui.OnlineMsg:
		// A rate limit is the token's, and has lifted unless one holds.
		if !msg.Limited {
			s.resumeAhead()
		}
		return s.online(), true
	case ui.ImagesMsg:
		// The profile may have gained or lost the avatar's rows, and the
		// README its images.
		s.layout()
		s.redrawSide()
		return nil, true
	}
	pages := s.pages()
	cmds := make([]tea.Cmd, 0, len(pages))
	for _, p := range pages {
		for _, l := range p.lists {
			if l == nil {
				continue
			}
			cmds = append(cmds, l.update(msg))
			if l.remeasure(s) {
				s.layoutList(l)
			}
		}
	}
	cmds = append(cmds, s.updateSide(msg))
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
		s.setFocus(s.nextPane(1))
		return nil
	case key.Matches(msg, k.Prev):
		s.setFocus(s.nextPane(-1))
		return nil
	case s.wide && key.Matches(msg, k.Zoom):
		s.setZoom(!s.zoom)
		return nil
	case key.Matches(msg, k.Back):
		return s.unzoom()
	case key.Matches(msg, k.Refresh):
		return s.refresh()
	}
	if i := k.pane(msg); i >= 0 {
		if s.hasPane(i) {
			s.setFocus(i)
		}
		return nil
	}
	switch p.focus {
	case listPane:
		if l := p.list(); l != nil {
			return l.update(msg)
		}
	case readmePane:
		return s.pressReadme(msg)
	case calendarPane:
		return s.pressCalendar(msg)
	default:
	}
	return nil
}

// unzoom shows every pane again while one is zoomed. Going back through the
// pages is the app's back key.
func (s *Section) unzoom() tea.Cmd {
	if s.zoomed() {
		s.setZoom(false)
	}
	return nil
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
				s.openedAhead()
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
		l := p.list()
		if l == nil {
			return nil, false
		}
		repos := p.tab == reposTab
		switch {
		case key.Matches(msg, k.NextTab):
			return s.stepTab(1), true
		case key.Matches(msg, k.PrevTab):
			return s.stepTab(-1), true
		case repos && key.Matches(msg, k.ClearFilter):
			return s.setFilter(""), true
		case repos && key.Matches(msg, k.Filter):
			return ui.OpenFilter(filterform.FiltersTab), true
		case repos && key.Matches(msg, k.Sort):
			return ui.OpenFilter(filterform.SortTab), true
		case key.Matches(msg, k.Select):
			s.openedAhead()
			return l.enter(s), true
		case key.Matches(msg, k.Open):
			if sel, ok := l.selection(s); ok && sel.URL != "" {
				return ui.Open(sel.URL), true
			}
			// With nothing under the cursor, as when the list failed or
			// is empty, the tab opens on GitHub instead.
			return ui.Open(s.tabURL(p.tab)), true
		}
	case readmePane:
		if sel, ok := s.readmeSelection(); ok && key.Matches(msg, k.Open) {
			return ui.Open(sel.URL), true
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

// focusPane focuses the list of the tab on view of the page on view while
// the page and its pane are focused, and blurs the others.
func (s *Section) focusPane() {
	p := s.page
	if p == nil {
		return
	}
	// The page blurs the README and the calendar with its lists, so they
	// are focused after it.
	s.blurPage(p)
	s.focusSide()
	if l := p.list(); l != nil && s.focused && p.focus == listPane {
		l.feed().Focus()
	}
}

// blurPage blurs the lists of p, which leaves the screen.
func (s *Section) blurPage(p *page) {
	blurSide(p)
	if p == nil {
		return
	}
	for _, l := range p.lists {
		if l != nil {
			l.feed().Blur()
		}
	}
}

// tabURL is the page on GitHub of tab t of the page on view: the people
// or the teams of an organization, or the profile of the account on the
// tab, as GitHub names its tabs.
func (s *Section) tabURL(t tab) string {
	login := s.Login()
	org := s.page.header.ok && s.page.header.value.Kind == core.OwnerOrg
	switch {
	case org && t == membersTab:
		return ui.WebURL(s.host, "orgs/"+login+"/people")
	case org && t == teamsTab:
		return ui.WebURL(s.host, "orgs/"+login+"/teams")
	case org && t == reposTab:
		return ui.WebURL(s.host, "orgs/"+login+"/repositories")
	}
	tabs := map[tab]string{reposTab: "repositories", starsTab: "stars", followersTab: "followers", followingTab: "following"}
	if name, ok := tabs[t]; ok {
		return s.profileURL(login) + "?tab=" + name
	}
	return s.profileURL(login)
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
