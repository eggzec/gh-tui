package owner

import (
	"context"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Landing reads what the repository screen reads first when it opens a
// repository, so that a repository read ahead opens at once.
type Landing interface {
	// Read reads what opening repo reads first into the caches the
	// repository screen reads from.
	Read(ctx context.Context, repo core.RepoRef) error
	// Cached reports whether that is in memory already. It does no I/O.
	Cached(repo core.RepoRef) bool
}

// WithLanding gives the page l to read ahead what opening the
// repositories around the cursor reads first, in the repositories and
// stars tabs and the pinned pane. Whether and how far it reads is
// WithPrefetch's, as prefetch.owner.repositories says. Without it, no
// repository is read ahead.
func WithLanding(l Landing) Option {
	return func(s *Section) { s.landing = l }
}

// WithPrefetch reads ahead as p says for prefetch.owner, while the page is
// on view, so that what the user opens next opens at once:
//   - people: the header of the accounts around the cursor of a list of
//     people, each time the cursor rests, a query each;
//   - repositories: what opening the repositories around the cursor of a
//     list of repositories or of the pinned pane reads first, through
//     WithLanding's;
//   - other_tabs: the first page of each tab not on view, once the page
//     rests on the tab shown.
//
// What is cached is skipped. Without it, nothing is read ahead, as with
// the defaults.
func WithPrefetch(p config.PrefetchLayers) Option {
	return func(s *Section) { s.layers = p }
}

// WithSlots bounds the reads ahead of the page with those of every page
// and modal that shares s, so that together they keep to
// prefetch.parallel. Without it, each kind it reads has slots of its own.
func WithSlots(s *ui.Slots) Option {
	return func(x *Section) { x.slots = s }
}

// WithViewer names the user, whose own login opens the dashboard rather
// than a page, so that their header isn't read ahead.
func WithViewer(login string) Option {
	return func(s *Section) { s.viewer = login }
}

// aheads read ahead what the page on view may open next.
type aheads struct {
	// people reads the headers of the accounts of a list of people.
	people *ui.Ahead[string]
	// repos and pinned read through the landing, if there is one, the
	// repositories of a list and the pinned cards.
	repos  *ui.Ahead[core.RepoRef]
	pinned *ui.Ahead[core.RepoRef]
	// tabs reads the first page of the tabs not on view.
	tabs *ui.Ahead[tabRead]
	// page is the page the reads last looked at, and list its list on
	// view then. Another page stops every read of the last one, and
	// another list the reads of the rows of the last; each new one reads
	// its first window at once.
	page *page
	list lister
	// refreshed is the page last read again, until it shows another
	// tab: until then, only the tabs whose first page was read ahead
	// before are read ahead.
	refreshed *page
	// read holds the tabs whose first page was read ahead. The reads
	// add to it in their commands, so mu guards it.
	mu   sync.Mutex
	read map[tabRead]bool
}

// wasRead reports whether the first page of t was read ahead.
func (a *aheads) wasRead(t tabRead) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.read[t]
}

// noteRead records that the first page of t was read ahead.
func (a *aheads) noteRead(t tabRead) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.read[t] = true
}

// tabRead is the first page of a tab of an account, which reading ahead
// reads as the tab's list does.
type tabRead struct {
	login string
	kind  core.OwnerKind
	tab   tab
	// people is the list of people of a tab that lists them.
	people owners.PeopleList
}

// String names the tab read in the log, such as octocat followers.
func (t tabRead) String() string { return t.login + " " + shortTabs[t.tab] }

// newAheads makes the reads ahead of the page, which read nothing until
// setPrefetch turns them on.
func (s *Section) newAheads() {
	a := &s.ahead
	a.read = make(map[tabRead]bool)
	a.people = ui.NewAhead("owner", s.readPerson, s.svc.FreshHeader)
	a.tabs = ui.NewAhead("owner_tab", s.readTab, s.freshTab)
	if s.landing != nil {
		a.repos = ui.NewAhead("owner_repo", s.landing.Read, s.landing.Cached)
		a.pinned = ui.NewAhead("owner_pinned_repo", s.landing.Read, s.landing.Cached)
	}
	for _, sh := range a.all() {
		sh.Share(s.slots)
		sh.Configure(config.Resolved{})
		sh.Reset(s.ctx)
	}
}

// aheadOf is what the section does with each of its reads ahead, whatever
// it reads.
type aheadOf interface {
	ui.Pauser
	Share(s *ui.Slots)
	Configure(r config.Resolved)
	Reset(parent context.Context)
	Resume()
}

// all returns the reads ahead the page has.
func (a *aheads) all() []aheadOf {
	out := []aheadOf{a.people, a.tabs}
	if a.repos != nil {
		out = append(out, a.repos, a.pinned)
	}
	return out
}

// setPrefetch takes how the page reads ahead from p.
func (s *Section) setPrefetch(p config.PrefetchLayers) {
	a := &s.ahead
	a.people.Configure(ui.Resolve(p, "owner", "people"))
	repos := ui.Resolve(p, "owner", "repositories")
	a.repos.Configure(repos)
	a.pinned.Configure(repos)
	// The other tabs have no window: every tab of the page is in it.
	tabs := ui.Resolve(p, "owner", "other_tabs")
	tabs.Window = config.Window{Before: int(numTabs), After: int(numTabs)}
	a.tabs.Configure(tabs)
}

// stopAhead stops every read ahead, as the page leaves the screen.
func (s *Section) stopAhead() {
	for _, a := range s.ahead.all() {
		a.Reset(s.ctx)
	}
	s.ahead.page, s.ahead.list = nil, nil
}

// refreshAhead reads ahead again once the user read the page on view
// again, as resumeAhead does, but of its other tabs only those read
// ahead before, until it shows another tab.
func (s *Section) refreshAhead() {
	s.ahead.refreshed = s.page
	s.resumeAhead()
}

// resumeAhead reads ahead again after GitHub reported the rate limit, or
// the user read the page again, and tries again what failed lately.
func (s *Section) resumeAhead() {
	for _, a := range s.ahead.all() {
		a.Resume()
	}
}

// rested reads the window that the cursor, or the page, rested on.
func (s *Section) rested(msg ui.AheadMsg) tea.Cmd {
	a := &s.ahead
	return tea.Batch(a.people.Rested(msg), a.repos.Rested(msg), a.pinned.Rested(msg), a.tabs.Rested(msg))
}

// readAhead tells each read ahead where the cursor of its pane is, while
// the page is on view, which reads the window around it once it rests.
func (s *Section) readAhead() tea.Cmd {
	a := &s.ahead
	p := s.page
	if !s.started || !s.focused || p == nil {
		return tea.Batch(a.people.Window(nil, -1), a.repos.Window(nil, -1), a.pinned.Window(nil, -1), a.tabs.Window(nil, -1))
	}
	l := p.list()
	switch {
	case p != a.page:
		// The reads of another page stop.
		for _, r := range a.all() {
			r.Reset(s.ctx)
		}
		a.refreshed = nil
	case l != a.list:
		// Another tab is a new list, whose window is read at once.
		a.people.Reset(s.ctx)
		a.repos.Reset(s.ctx)
		a.refreshed = nil
	}
	a.page, a.list = p, l
	return tea.Batch(s.readPeopleAhead(l), s.readReposAhead(l), s.readPinnedAhead(), s.readTabsAhead(l))
}

// readPeopleAhead reads ahead the headers of the accounts around the
// cursor of l, the row under it too, while it is a list of people with
// the focus. The viewer's own row opens the dashboard, so it counts
// toward the window but isn't read.
func (s *Section) readPeopleAhead(l lister) tea.Cmd {
	pl, ok := l.(*peopleList)
	if !ok || s.page.focus != listPane || pl.Feed.Len() == 0 {
		return s.ahead.people.Window(nil, -1)
	}
	return s.ahead.people.Window(func(j int) (string, bool) {
		person, ok := pl.Feed.Item(j)
		return person.Login, ok && (s.viewer == "" || !strings.EqualFold(person.Login, s.viewer))
	}, pl.Feed.Index())
}

// readReposAhead reads ahead the repositories around the cursor of l
// while it is a list of repositories with the focus. The one under the
// cursor is left to opening it.
func (s *Section) readReposAhead(l lister) tea.Cmd {
	var t *tableTab
	switch l := l.(type) {
	case *repoList:
		t = &l.tableTab
	case *starList:
		t = &l.tableTab
	}
	if s.ahead.repos == nil {
		return nil
	}
	if t == nil || s.page.focus != listPane || t.Feed.Len() == 0 {
		return s.ahead.repos.Window(nil, -1)
	}
	i := t.Feed.Index()
	return s.ahead.repos.Window(func(j int) (core.RepoRef, bool) {
		r, ok := t.Feed.Item(j)
		return r.Ref, ok && j != i
	}, i)
}

// readPinnedAhead reads ahead the pinned cards around the cursor, in the
// order the pane lists them, while it has the focus. The card under the
// cursor is left to opening it.
func (s *Section) readPinnedAhead() tea.Cmd {
	if s.ahead.pinned == nil {
		return nil
	}
	c := &s.page.pinned
	if _, ok := c.Selected(); !ok || s.page.focus != pinnedPane {
		return s.ahead.pinned.Window(nil, -1)
	}
	return s.ahead.pinned.Window(func(j int) (core.RepoRef, bool) {
		if j == c.Sel || j >= len(c.Items) {
			return core.RepoRef{}, false
		}
		return c.Items[j].Repo.Ref, true
	}, c.Sel)
}

// readTabsAhead reads ahead the first page of each tab of the page on view
// that hasn't read it, once the list of the tab shown, l, has loaded and
// the page rests. The teams of an organization the viewer isn't a member
// of are never asked for, so they aren't read ahead. Once the page was
// read again, only the tabs read ahead before are, until it shows
// another tab.
func (s *Section) readTabsAhead(l lister) tea.Cmd {
	p := s.page
	if l == nil || !l.started() || !l.feed().Settled() || !p.header.ok {
		return s.ahead.tabs.Window(nil, -1)
	}
	o := p.header.value
	tabs := tabsOf(o.Kind)
	at := func(j int) (tabRead, bool) {
		if j >= len(tabs) || tabs[j] == p.tab {
			return tabRead{}, false
		}
		t := tabs[j]
		other := p.lists[t]
		if other == nil || other.started() {
			return tabRead{}, false
		}
		if tl, ok := other.(*teamList); ok && tl.membersOnly {
			return tabRead{}, false
		}
		k, ok := s.tabRead(t)
		if ok && s.ahead.refreshed == p && !s.ahead.wasRead(k) {
			return tabRead{}, false
		}
		return k, ok
	}
	i := 0
	for j, t := range tabs {
		if t == p.tab {
			i = j
		}
	}
	return s.ahead.tabs.Window(at, i)
}

// readPerson reads the header of the account login, as its page does.
func (s *Section) readPerson(ctx context.Context, login string) error {
	// A header an earlier session kept would come back at once, unread.
	_, err := s.svc.Header(ctx, owners.HeaderQuery{Login: login, Again: true})
	return err
}

// readTab reads the first page of the tab of t, as its list does.
func (s *Section) readTab(ctx context.Context, t tabRead) error {
	var err error
	switch t.tab {
	case reposTab:
		_, err = s.svc.Repos(ctx, owners.ReposQuery{Owner: t.login, Kind: t.kind, Again: true})
	case starsTab:
		_, err = s.svc.Stars(ctx, owners.StarsQuery{Login: t.login, Again: true})
	case teamsTab:
		_, err = s.svc.Teams(ctx, owners.TeamsQuery{Login: t.login, Again: true})
	default:
		_, err = s.svc.People(ctx, owners.PeopleQuery{Login: t.login, List: t.people, Again: true})
	}
	if err == nil {
		s.ahead.noteRead(t)
	}
	return err
}

// freshTab reports whether the first page of the tab of t is cached and
// fresh, so that reading it costs no request. It does no I/O.
func (s *Section) freshTab(t tabRead) bool {
	switch t.tab {
	case reposTab:
		return s.svc.FreshRepos(owners.ReposQuery{Owner: t.login, Kind: t.kind})
	case starsTab:
		return s.svc.FreshStars(owners.StarsQuery{Login: t.login})
	case teamsTab:
		return s.svc.FreshTeams(owners.TeamsQuery{Login: t.login})
	default:
		return s.svc.FreshPeople(owners.PeopleQuery{Login: t.login, List: t.people})
	}
}

// openedAhead records that what the cursor of the focused pane is on was
// opened, so that the summary counts it as a use if it was read ahead.
func (s *Section) openedAhead() {
	p := s.page
	switch {
	case p == nil:
	case p.focus == pinnedPane:
		if c, ok := p.pinned.Selected(); ok {
			s.ahead.pinned.Opened(c.Repo.Ref)
		}
	case p.focus == listPane:
		switch l := p.list().(type) {
		case *peopleList:
			if person, ok := l.Feed.Selected(); ok {
				s.ahead.people.Opened(person.Login)
			}
		case *repoList:
			if r, ok := l.Feed.Selected(); ok {
				s.ahead.repos.Opened(r.Ref)
			}
		case *starList:
			if r, ok := l.Feed.Selected(); ok {
				s.ahead.repos.Opened(r.Ref)
			}
		}
	}
}

// openedTab records that tab t of the page on view was shown, so that the
// summary counts its first page as a use if it was read ahead.
func (s *Section) openedTab(t tab) {
	if k, ok := s.tabRead(t); ok {
		s.ahead.tabs.Opened(k)
	}
}

// tabRead returns the first page of tab t of the page on view, once its
// header says whose it is.
func (s *Section) tabRead(t tab) (tabRead, bool) {
	p := s.page
	if p == nil || !p.header.ok {
		return tabRead{}, false
	}
	o := p.header.value
	k := tabRead{login: o.Profile.Login, kind: o.Kind, tab: t}
	if l, ok := p.lists[t].(*peopleList); ok {
		k.people = l.q.List
	}
	return k, true
}
