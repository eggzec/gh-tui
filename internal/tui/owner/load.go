package owner

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// loadedMsg carries the header of page p of the section with id, read in
// p's refresh gen.
type loadedMsg struct {
	id    int64
	p     *page
	gen   int
	owner core.Owner
	err   error
}

// load reads the header of p, which serves what the cache has at once, so
// a fresh cache costs no request, and the list of the tab on view.
func (s *Section) load(p *page) tea.Cmd {
	if p == nil || !s.started {
		return nil
	}
	return tea.Batch(s.readHeader(p, false), s.startList(p))
}

func (s *Section) readHeader(p *page, again bool) tea.Cmd {
	p.header.loading = true
	ctx, id, gen, svc, login := s.ctx, s.id, p.gen, s.svc, p.login
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "owner.header")
		o, err := svc.Header(ctx, owners.HeaderQuery{Login: login, Again: again})
		end(err, "span", "tui", "stale", o.Stale, "offline", o.Offline, "limited", o.Limited)
		return loadedMsg{id: id, p: p, gen: gen, owner: o, err: err}
	}
}

// startList fetches the first page of the list of the tab of p on view,
// once its header says whose it is.
func (s *Section) startList(p *page) tea.Cmd {
	l := p.list()
	if l == nil || !s.started {
		return nil
	}
	return l.start()
}

// loaded takes the header of a page, and reads it again if it was kept
// by an earlier session.
func (s *Section) loaded(msg loadedMsg) tea.Cmd {
	p := msg.p
	if msg.id != s.id || msg.gen != p.gen {
		return nil
	}
	p.header.loading = false
	p.header.err = msg.err
	if msg.err != nil {
		// A failed read keeps the header shown before.
		return nil
	}
	p.header.value, p.header.ok = msg.owner, true
	s.setHeader(p)
	if msg.owner.Stale {
		return tea.Batch(s.readHeader(p, true), s.startList(p))
	}
	return s.startList(p)
}

// setHeader shows the pins of the header of p, and makes the lists of its
// tabs, now that the header says whose they are.
func (s *Section) setHeader(p *page) {
	h := p.header.value
	p.pinned.Set(h.Pinned)
	if h.Profile.Login == "" {
		return
	}
	if s.makeLists(p, h) && p == s.page {
		s.layout()
		s.focusPane()
	}
}

// makeLists makes the lists of the tabs that the page p of o has and
// lacks, and reports whether it made any. What the viewer may see of an
// organization follows its header each time.
func (s *Section) makeLists(p *page, o core.Owner) bool {
	login, member := o.Profile.Login, o.Viewer.Member
	p.tab = tabIn(p.tab, o.Kind)
	made := false
	for _, t := range tabsOf(o.Kind) {
		if p.lists[t] != nil {
			continue
		}
		made = true
		switch t {
		case reposTab:
			p.lists[t] = s.newRepoList(o)
		case starsTab:
			p.lists[t] = s.newStarList(login)
		case membersTab:
			p.lists[t] = s.newPeopleList(t, login, member)
		case teamsTab:
			p.lists[t] = s.newTeamList(login, !member)
		default:
			p.lists[t] = s.newPeopleList(t, login, false)
		}
	}
	if l, ok := p.lists[membersTab].(*peopleList); ok {
		l.roles = member
	}
	if l, ok := p.lists[teamsTab].(*teamList); ok {
		l.membersOnly = !member
	}
	return made
}

// refresh reads the page on view again from GitHub, and leaves those to go
// back to as they are.
func (s *Section) refresh() tea.Cmd {
	p := s.page
	s.svc.InvalidateLogin(p.login)
	p.gen++
	cmd := s.readHeader(p, false)
	if l := p.list(); l != nil && l.started() {
		cmd = tea.Batch(cmd, l.feed().Reload())
	}
	return cmd
}

// online reads again, now that GitHub answers again, what of the page on
// view failed for want of an answer from it, or was served from what an
// earlier read kept while GitHub couldn't be reached or rate limited it.
// What is being read already is left to finish.
func (s *Section) online() tea.Cmd {
	p := s.page
	if !s.started || p == nil {
		return nil
	}
	var cmds []tea.Cmd
	h := p.header
	if !h.loading && (ui.Unreached(h.err) || h.ok && h.err == nil && (h.value.Offline || h.value.Limited)) {
		cmds = append(cmds, s.readHeader(p, true))
	}
	if l := p.list(); l != nil && l.started() {
		cmds = append(cmds, ui.RetryUnreached(l.feed()))
	}
	return tea.Batch(cmds...)
}

// Revisit reads again what of the page on view went past its TTL while
// another screen was on view. The page shows what it has until the new
// values arrive, and what is still fresh isn't read.
func (s *Section) Revisit() tea.Cmd {
	p := s.page
	if !s.started || p == nil {
		return nil
	}
	// Each read sets Again: what is shown may be a value an earlier
	// session kept, served stale, and this read must reach GitHub anyway.
	var cmds []tea.Cmd
	if !p.header.loading && !s.svc.FreshHeader(p.login) {
		cmds = append(cmds, s.readHeader(p, true))
	}
	if l := p.list(); l != nil && l.started() && l.feed().Settled() && !l.fresh(s.svc) {
		cmds = append(cmds, l.feed().Reload())
	}
	cmd := tea.Batch(cmds...)
	if cmd != nil {
		// What is being read again shows as updating.
		s.render()
	}
	return cmd
}

// updating reports whether something shown is being read again, such as
// what an earlier session kept, or what went stale while another screen
// was on view.
func (s *Section) updating() bool {
	p := s.page
	if p == nil {
		return false
	}
	l := p.list()
	return p.header.ok && p.header.loading || l != nil && l.started() && l.feed().Len() > 0 && !l.feed().Settled()
}

// setTab shows tab t of the list pane of the page on view, and reads its
// list: the first page the first time, and again if it went stale or
// failed for want of an answer from GitHub meanwhile.
func (s *Section) setTab(t tab) tea.Cmd {
	p := s.page
	p.tab = t
	s.focusPane()
	l := p.list()
	switch {
	case l == nil || !s.started:
		return nil
	case !l.started():
		return l.start()
	case l.feed().Settled() && !l.fresh(s.svc):
		return l.feed().Reload()
	}
	return ui.RetryUnreached(l.feed())
}

// stepTab shows the tab by from the one on view, among those of the page,
// round from the last to the first.
func (s *Section) stepTab(by int) tea.Cmd {
	p := s.page
	if !p.header.ok {
		return nil
	}
	tabs := tabsOf(p.header.value.Kind)
	i := 0
	for j, t := range tabs {
		if t == p.tab {
			i = j
		}
	}
	return s.setTab(tabs[(i+by+len(tabs))%len(tabs)])
}
