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

// startList fetches the first page of the list of p, once its header says
// whose it is.
func (s *Section) startList(p *page) tea.Cmd {
	if p.repos == nil || !s.started {
		return nil
	}
	return p.repos.start()
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

// setHeader shows the pins of the header of p, and makes its list of
// repositories, now that the header says whose they are.
func (s *Section) setHeader(p *page) {
	h := p.header.value
	p.pinned.Set(h.Pinned)
	if p.repos == nil && h.Profile.Login != "" {
		p.repos = s.newRepoList(h)
		if p == s.page {
			s.layout()
			s.focusPane()
		}
	}
}

// refresh reads the page on view again from GitHub, and leaves those to go
// back to as they are.
func (s *Section) refresh() tea.Cmd {
	p := s.page
	s.svc.InvalidateLogin(p.login)
	p.gen++
	cmd := s.readHeader(p, false)
	if p.repos != nil && p.repos.started {
		cmd = tea.Batch(cmd, p.repos.Feed.Reload())
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
	if p.repos != nil && p.repos.started {
		cmds = append(cmds, ui.RetryUnreached(&p.repos.Feed))
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
	if l := p.repos; l != nil && l.started && l.Feed.Settled() && !s.svc.FreshRepos(l.q) {
		cmds = append(cmds, l.Feed.Reload())
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
	l := p.repos
	return p.header.ok && p.header.loading || l != nil && l.started && l.Feed.Len() > 0 && !l.Feed.Settled()
}
