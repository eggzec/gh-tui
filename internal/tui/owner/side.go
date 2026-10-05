package owner

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// sideService is what the panes beside the list read: the README, the
// contribution calendar and the sponsors, and the follower count of an
// organization, which its profile shows.
type sideService interface {
	FreshReadme(q owners.ReadmeQuery) bool
	Readme(ctx context.Context, q owners.ReadmeQuery) (owners.Readme, error)
	FreshContributions(login string) bool
	Contributions(ctx context.Context, q owners.ContributionsQuery) (core.Contributions, error)
	FreshOrgFollowers(login string) bool
	OrgFollowers(ctx context.Context, q owners.OrgFollowersQuery) (owners.FollowerCount, error)
	FreshPeople(q owners.PeopleQuery) bool
	People(ctx context.Context, q owners.PeopleQuery) (core.Page[core.Person], error)
}

// side is what a page shows beside its list, and the follower count of an
// organization. Each is read once the header says whose page it is, and
// the panes only once they are on view.
type side struct {
	readme sideRead[owners.Readme]
	// sponsors and sponsoring end the README, on github.com only.
	sponsors   sideRead[core.Page[core.Person]]
	sponsoring sideRead[core.Page[core.Person]]
	contribs   sideRead[core.Contributions]
	followers  sideRead[owners.FollowerCount]
	// pager shows the README, and cal the calendar, once made.
	pager *pager.Model
	cal   *calendar.Model
	// shown is the source the pager shows, so that it is set again only
	// when that changed, which keeps the place in it.
	shown string
}

// sideRead is a read of the side: what the section knows of it, and the
// refresh of the page it was last asked in.
type sideRead[V any] struct {
	read[V]
	asked bool
	gen   int
}

// due reports whether r is to be read for page p: never asked, or asked
// before the page was refreshed.
func (r *sideRead[V]) due(p *page) bool {
	return !r.asked || r.gen != p.gen
}

// kept is how a value of the side was served.
type kept struct{ stale, offline, limited bool }

// sideMsg carries what a read of the side of page p of the section with id
// found, in p's refresh gen. take keeps it in the read it is of, and
// returns how it was served, and again reads it past what was kept.
type sideMsg struct {
	id    int64
	p     *page
	gen   int
	take  func() kept
	again func() tea.Cmd
}

// readSide returns the command that reads r of page p with get, which
// takes again to read past a kept value. marks says how a value was
// served.
func readSide[V any](s *Section, p *page, r *sideRead[V], span string, marks func(V) kept, get func(ctx context.Context, again bool) (V, error), again bool) tea.Cmd {
	r.loading, r.asked, r.gen = true, true, p.gen
	ctx, id, gen := s.ctx, s.id, p.gen
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, span)
		v, err := get(ctx, again)
		m := marks(v)
		end(err, "span", "tui", "stale", m.stale, "offline", m.offline, "limited", m.limited)
		take := func() kept {
			r.loading, r.err = false, err
			if err != nil {
				// A failed read keeps what was shown before.
				return kept{}
			}
			r.value, r.ok = v, true
			return m
		}
		again := func() tea.Cmd { return readSide(s, p, r, span, marks, get, true) }
		return sideMsg{id: id, p: p, gen: gen, take: take, again: again}
	}
}

func readmeMarks(r owners.Readme) kept { return kept{r.Stale, r.Offline, r.Limited} }
func peopleMarks(p core.Page[core.Person]) kept {
	return kept{p.Stale, p.Offline, p.Limited}
}
func contribsMarks(c core.Contributions) kept    { return kept{c.Stale, c.Offline, c.Limited} }
func followersMarks(f owners.FollowerCount) kept { return kept{f.Stale, f.Offline, f.Limited} }

// sideReads are the reads of the side of a page, each with whether it is
// to be read: the pane it shows in is on view, or it is the profile's.
type sideReads struct {
	readme, sponsors, contribs, followers bool
}

// wanted returns which reads of the side page p shows now.
func (s *Section) wanted(p *page) sideReads {
	if p == nil || !p.header.ok || p != s.page {
		return sideReads{}
	}
	readme := s.onView(readmePane)
	org := p.header.value.Kind == core.OwnerOrg
	return sideReads{
		readme:    readme,
		sponsors:  readme && s.sponsorsShown(),
		contribs:  !org && s.onView(calendarPane),
		followers: org,
	}
}

// onView reports whether pane p of the page on view shows.
func (s *Section) onView(p paneID) bool {
	if s.page == nil || !s.hasPane(p) {
		return false
	}
	return !s.onePane() || s.page.focus == p
}

// sponsorsShown reports whether the README ends with the sponsors, which
// only github.com has.
func (s *Section) sponsorsShown() bool {
	return s.host == "" || s.host == core.DefaultHost
}

// startSide reads what of the side of the page on view shows and wasn't
// read yet, or was read before a refresh.
func (s *Section) startSide() tea.Cmd {
	p := s.page
	if !s.started || p == nil {
		return nil
	}
	w := s.wanted(p)
	sd := &p.side
	var cmds []tea.Cmd
	if w.readme && sd.readme.due(p) {
		cmds = append(cmds, s.readReadme(p, false))
	}
	if w.sponsors && sd.sponsors.due(p) {
		cmds = append(cmds, s.readSponsors(p, owners.Sponsors, false))
	}
	if w.sponsors && sd.sponsoring.due(p) {
		cmds = append(cmds, s.readSponsors(p, owners.Sponsoring, false))
	}
	if w.contribs && sd.contribs.due(p) {
		cmds = append(cmds, s.readContribs(p, false))
	}
	if w.followers && sd.followers.due(p) {
		cmds = append(cmds, s.readFollowers(p, false))
	}
	return tea.Batch(cmds...)
}

func (s *Section) readReadme(p *page, again bool) tea.Cmd {
	q, svc := s.readmeQuery(p), s.svc
	return readSide(s, p, &p.side.readme, "owner.readme", readmeMarks, func(ctx context.Context, again bool) (owners.Readme, error) {
		q.Again = again
		return svc.Readme(ctx, q)
	}, again)
}

// readSponsors reads the first page of list, the sponsors or those
// sponsored.
func (s *Section) readSponsors(p *page, list owners.PeopleList, again bool) tea.Cmd {
	r := &p.side.sponsors
	if list == owners.Sponsoring {
		r = &p.side.sponsoring
	}
	q, svc := owners.PeopleQuery{Login: p.header.value.Profile.Login, List: list}, s.svc
	return readSide(s, p, r, "owner."+list.String(), peopleMarks, func(ctx context.Context, again bool) (core.Page[core.Person], error) {
		q.Again = again
		return svc.People(ctx, q)
	}, again)
}

func (s *Section) readContribs(p *page, again bool) tea.Cmd {
	login, svc := p.header.value.Profile.Login, s.svc
	return readSide(s, p, &p.side.contribs, "owner.contributions", contribsMarks, func(ctx context.Context, again bool) (core.Contributions, error) {
		return svc.Contributions(ctx, owners.ContributionsQuery{Login: login, Again: again})
	}, again)
}

func (s *Section) readFollowers(p *page, again bool) tea.Cmd {
	login, svc := p.header.value.Profile.Login, s.svc
	return readSide(s, p, &p.side.followers, "owner.followers", followersMarks, func(ctx context.Context, again bool) (owners.FollowerCount, error) {
		return svc.OrgFollowers(ctx, owners.OrgFollowersQuery{Login: login, Again: again})
	}, again)
}

// readmeQuery selects the README of p, as its header says whose it is.
func (s *Section) readmeQuery(p *page) owners.ReadmeQuery {
	h := p.header.value
	return owners.ReadmeQuery{Login: h.Profile.Login, Kind: h.Kind, Member: h.Viewer.Member}
}

// sideLoaded takes what a read of the side found, shows it, and reads it
// again past what an earlier session kept.
func (s *Section) sideLoaded(msg sideMsg) tea.Cmd {
	p := msg.p
	if msg.id != s.id || msg.gen != p.gen {
		return nil
	}
	m := msg.take()
	s.setSide(p)
	if !m.stale {
		return nil
	}
	return msg.again()
}

// againSide reads again, past what was kept, each read of the side of p
// that was asked and isn't being read, whose value again selects or
// which GitHub didn't answer.
func (s *Section) againSide(p *page, again func(kept) bool) tea.Cmd {
	sd := &p.side
	var cmds []tea.Cmd
	if r := &sd.readme; r.asked && !r.loading && (r.ok && again(readmeMarks(r.value)) || ui.Unreached(r.err)) {
		cmds = append(cmds, s.readReadme(p, true))
	}
	for _, l := range []owners.PeopleList{owners.Sponsors, owners.Sponsoring} {
		r := &sd.sponsors
		if l == owners.Sponsoring {
			r = &sd.sponsoring
		}
		if r.asked && !r.loading && (r.ok && again(peopleMarks(r.value)) || ui.Unreached(r.err)) {
			cmds = append(cmds, s.readSponsors(p, l, true))
		}
	}
	if r := &sd.contribs; r.asked && !r.loading && (r.ok && again(contribsMarks(r.value)) || ui.Unreached(r.err)) {
		cmds = append(cmds, s.readContribs(p, true))
	}
	if r := &sd.followers; r.asked && !r.loading && (r.ok && again(followersMarks(r.value)) || ui.Unreached(r.err)) {
		cmds = append(cmds, s.readFollowers(p, true))
	}
	return tea.Batch(cmds...)
}

// onlineSide reads again what of the side of the page on view GitHub
// didn't answer, or was served kept while it couldn't be reached or rate
// limited the read.
func (s *Section) onlineSide() tea.Cmd {
	if s.page == nil {
		return nil
	}
	return s.againSide(s.page, func(m kept) bool { return m.offline || m.limited })
}

// revisitSide reads again what of the side of the page on view went past
// its TTL.
func (s *Section) revisitSide() tea.Cmd {
	p := s.page
	if p == nil || !p.header.ok {
		return nil
	}
	sd, login := &p.side, p.header.value.Profile.Login
	var cmds []tea.Cmd
	if r := &sd.readme; r.asked && !r.loading && !s.svc.FreshReadme(s.readmeQuery(p)) {
		cmds = append(cmds, s.readReadme(p, true))
	}
	for _, l := range []owners.PeopleList{owners.Sponsors, owners.Sponsoring} {
		r := &sd.sponsors
		if l == owners.Sponsoring {
			r = &sd.sponsoring
		}
		if r.asked && !r.loading && r.err == nil && !s.svc.FreshPeople(owners.PeopleQuery{Login: login, List: l}) {
			cmds = append(cmds, s.readSponsors(p, l, true))
		}
	}
	if r := &sd.contribs; r.asked && !r.loading && !s.svc.FreshContributions(login) {
		cmds = append(cmds, s.readContribs(p, true))
	}
	if r := &sd.followers; r.asked && !r.loading && !s.svc.FreshOrgFollowers(login) {
		cmds = append(cmds, s.readFollowers(p, true))
	}
	return tea.Batch(cmds...)
}

// sideUpdating reports whether a value of the side shown is being read
// again.
func (s *Section) sideUpdating() bool {
	p := s.page
	if p == nil {
		return false
	}
	sd := &p.side
	return sd.readme.ok && sd.readme.loading || sd.contribs.ok && sd.contribs.loading ||
		sd.followers.ok && sd.followers.loading
}

// setSide shows what was read of the side of p.
func (s *Section) setSide(p *page) {
	s.setReadme(p)
	s.setCalendar(p)
}

// updateSide passes msg to the pagers of the pages, which ignore the
// messages of others.
func (s *Section) updateSide(msg tea.Msg) tea.Cmd {
	pages := s.pages()
	cmds := make([]tea.Cmd, 0, len(pages))
	for _, p := range pages {
		if pg := p.side.pager; pg != nil {
			var cmd tea.Cmd
			*pg, cmd = pg.Update(msg)
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// focusSide focuses the pager or the calendar of the page on view while
// its pane has the focus, and blurs them otherwise.
func (s *Section) focusSide() {
	p := s.page
	if p == nil {
		return
	}
	if pg := p.side.pager; pg != nil {
		if s.focused && p.focus == readmePane {
			pg.Focus()
		} else {
			pg.Blur()
		}
	}
	if c := p.side.cal; c != nil {
		if s.focused && p.focus == calendarPane {
			c.Focus()
		} else {
			c.Blur()
		}
	}
}

// blurSide blurs the pager and the calendar of p, which leaves the screen.
func blurSide(p *page) {
	if p == nil {
		return
	}
	if pg := p.side.pager; pg != nil {
		pg.Blur()
	}
	if c := p.side.cal; c != nil {
		c.Blur()
	}
}

// resizeSide sizes the pager and the calendar of the page on view to their
// panes.
func (s *Section) resizeSide() {
	p := s.page
	if p == nil {
		return
	}
	if pg := p.side.pager; pg != nil {
		w, h := s.inside(readmePane)
		pg.SetSize(max(w-2, 0), h)
	}
	if c := p.side.cal; c != nil {
		w, h := s.inside(calendarPane)
		c.SetSize(min(max(w-2, 0), c.FitWidth()), min(h, calendarLines))
	}
}

// orgFollowers returns how many follow the organization on view, once
// read.
func (s *Section) orgFollowers() (int, bool) {
	r := s.page.side.followers
	return r.value.Count, r.ok
}

// hasPane reports whether the page on view has pane id: only a user's has
// the calendar, once its header says it is one.
func (s *Section) hasPane(id paneID) bool {
	if id != calendarPane {
		return true
	}
	p := s.page
	return p != nil && p.header.ok && p.header.value.Kind == core.OwnerUser
}

// nextPane returns the pane dir steps from the focused one, past those the
// page doesn't have.
func (s *Section) nextPane(dir int) paneID {
	p := s.page.focus
	for range numPanes {
		p = (p + paneID(dir) + numPanes) % numPanes
		if s.hasPane(p) {
			break
		}
	}
	return p
}

// sideLayer returns the keys of the README's pager or of the calendar,
// whichever has the focus.
func (s *Section) sideLayer() (keyhelp.Layer, bool) {
	p := s.page
	if p == nil {
		return keyhelp.Layer{}, false
	}
	switch {
	case p.focus == readmePane && p.side.pager != nil:
		return keyhelp.FromHelp("readme", s.sc.keys, false), true
	case p.focus == calendarPane && p.side.cal != nil && s.hasPane(calendarPane):
		return keyhelp.FromHelp("calendar", p.side.cal.KeyMap(), false), true
	}
	return keyhelp.Layer{}, false
}

// sideSelected is what the focused pane beside the list is on, for the
// copy command: the README, or the account.
func (s *Section) sideSelected() (ui.Selection, bool) {
	if s.page.focus == readmePane {
		return s.readmeSelection()
	}
	return s.profileSelection()
}
