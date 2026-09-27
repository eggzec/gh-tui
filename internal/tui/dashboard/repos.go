package dashboard

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/dashboard"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// yours labels the tab of the viewer's own repositories.
const yours = "Yours"

// owner is a tab of the repositories pane: the repositories of the viewer
// or of one organization.
type owner struct {
	label string
	q     dashboard.ReposQuery
	feed  feed.Model[core.Repo]
	// started is set once the feed has fetched its first page.
	started bool
	// measure sizes cols, the columns of the list.
	measure repoMeasure
	cols    repoCols

	// filter is the filter of the pane, which the feed's reads use in
	// their commands.
	filter atomic.Pointer[repoFilter]
}

// emptyText is what the list of o says when it has no repository.
func (o *owner) emptyText(f *repoFilter) string {
	switch {
	case f.active():
		return "No repository of " + ownerName(o) + " matches the filter."
	case o.q.Viewer:
		return "You own no repository yet."
	}
	return o.label + " has no repository you can see."
}

func ownerName(o *owner) string {
	if o.q.Viewer {
		return "yours"
	}
	return o.label
}

// key identifies the owner, whose login GitHub matches regardless of case.
func (o *owner) key() string {
	if o.q.Viewer {
		return "@me"
	}
	return strings.ToLower(o.q.Owner)
}

// repoTabs is the repositories pane: a tab per owner, each a list of
// repositories paged as it scrolls. A filter lists those it keeps of up to
// dashboard.MaxOwnerRepos of them instead.
type repoTabs struct {
	s    *Section
	tabs []*owner
	cur  int

	width, height int
}

func newRepoTabs(s *Section) repoTabs {
	t := repoTabs{s: s}
	t.tabs = []*owner{t.newOwner(yours, dashboard.ReposQuery{Viewer: true})}
	return t
}

func (t *repoTabs) newOwner(label string, q dashboard.ReposQuery) *owner {
	s := t.s
	o := &owner{label: label, q: q}
	if len(t.tabs) > 0 {
		o.filter.Store(t.tabs[0].filter.Load())
	}
	query := func(cursor string) dashboard.ReposQuery {
		q := q
		q.Cursor = cursor
		return q
	}
	read := func(ctx context.Context, q dashboard.ReposQuery, again bool) (core.Page[core.Repo], error) {
		q.Again = again
		f := o.filter.Load()
		if f == nil || !f.active() {
			return s.svc.Repos(ctx, q)
		}
		// The filter runs over every repository, so that it sorts them
		// all. The pages the list read are cached and cost nothing.
		p, err := s.svc.AllRepos(ctx, q, dashboard.MaxOwnerRepos)
		p.Items, p.Next = f.apply(p.Items), ""
		return p, err
	}
	empty := o.emptyText(&repoFilter{})
	if f := o.filter.Load(); f != nil {
		empty = o.emptyText(f)
	}
	render := func(r core.Repo, selected bool, _ int) string { return s.renderRepo(o.cols, r, selected) }
	o.feed = feed.New(ui.FeedPages("dashboard.repos", s.offline, query, read), render,
		feed.WithContext(s.ctx),
		feed.WithKey(func(r core.Repo) string { return r.Ref.String() }),
		feed.WithKeyMap(s.keys.feed),
		feed.WithEmptyText(empty),
		feed.WithStyles(s.theme.Feed()),
		feed.WithErrorText(ui.ErrorText("load your repositories", "", s.voice)),
	)
	return o
}

// listTop is how many lines the tabs and the headers of the columns take
// above the list.
const listTop = 2

// remeasure measures the repositories of o read since it last did, and
// lays the columns out again if they need more room.
func (t *repoTabs) remeasure(o *owner) {
	n := o.feed.Len()
	if n == o.measure.seen {
		return
	}
	m := o.measure
	for i := range n {
		if r, ok := o.feed.Item(i); ok {
			m.add(r, t.s.icons)
		}
	}
	m.seen = n
	if m != o.measure {
		o.measure = m
		t.layout(o)
	}
}

// layout fits the columns of o in the list, inside the cursor's gutter.
func (t *repoTabs) layout(o *owner) {
	o.cols = layoutCols(max(t.width-gutterWidth, 0), o.measure, t.s.icons.Star)
}

// gutterWidth is the room the list leaves for its cursor.
const gutterWidth = 2

// current returns the tab on view.
func (t *repoTabs) current() *owner { return t.tabs[t.cur] }

// setOrgs gives each organization of the viewer a tab after their own,
// keeping the tabs that were there.
func (t *repoTabs) setOrgs(orgs []core.Org, login string) {
	if login != "" {
		t.tabs[0].label = yours
	}
	cur := t.current()
	tabs := make([]*owner, 0, len(orgs)+1)
	tabs = append(tabs, t.tabs[0])
	for _, org := range orgs {
		login := strings.ToLower(org.Login)
		if i := slices.IndexFunc(t.tabs, func(o *owner) bool { return o.key() == login }); i >= 0 {
			tabs = append(tabs, t.tabs[i])
			continue
		}
		o := t.newOwner(org.Login, dashboard.ReposQuery{Owner: org.Login})
		o.feed.SetSize(t.width, max(t.height-listTop, 0))
		t.layout(o)
		tabs = append(tabs, o)
	}
	t.tabs = tabs
	t.cur = max(slices.Index(tabs, cur), 0)
}

// start fetches the first page of the tab on view, once the dashboard has
// started.
func (t *repoTabs) start() tea.Cmd {
	o := t.current()
	if !t.s.started || o.started {
		return nil
	}
	o.started = true
	return o.feed.Init()
}

// reload reads the pages of every started tab again.
func (t *repoTabs) reload() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(t.tabs))
	for _, o := range t.tabs {
		if o.started {
			cmds = append(cmds, o.feed.Reload())
		}
	}
	return tea.Batch(cmds...)
}

// revisit reads again the lists of the tabs started whose first page went
// past its TTL, showing their rows until the new ones arrive. A list still
// loading is left to finish.
func (t *repoTabs) revisit() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(t.tabs))
	for _, o := range t.tabs {
		if o.started && o.feed.Settled() && !t.s.svc.FreshRepos(o.q) {
			cmds = append(cmds, o.feed.Reload())
		}
	}
	return tea.Batch(cmds...)
}

// reloading reports whether a list that shows repositories is reading its
// first page again.
func (t *repoTabs) reloading() bool {
	for _, o := range t.tabs {
		if o.started && o.feed.Len() > 0 && !o.feed.Settled() {
			return true
		}
	}
	return false
}

// switchTab shows the tab delta tabs away, and starts it.
func (t *repoTabs) switchTab(delta int) tea.Cmd {
	n := len(t.tabs)
	if n < 2 {
		return nil
	}
	focused := t.current().feed.Focused()
	t.current().feed.Blur()
	t.cur = ((t.cur+delta)%n + n) % n
	if focused {
		t.current().feed.Focus()
	}
	return t.start()
}

func (t *repoTabs) focus() {
	t.current().feed.Focus()
}

func (t *repoTabs) blur() {
	for _, o := range t.tabs {
		o.feed.Blur()
	}
}

func (t *repoTabs) resize(width, height int) {
	t.width, t.height = width, height
	for _, o := range t.tabs {
		o.feed.SetSize(width, max(height-listTop, 0))
		t.layout(o)
	}
}

func (t *repoTabs) setTheme(th ui.Theme) {
	for _, o := range t.tabs {
		o.feed.SetStyles(th.Feed())
	}
}

// feedKeys returns the keys of the list on view, whose retry the list
// enables only while a fetch has failed.
func (t *repoTabs) feedKeys() feed.KeyMap { return t.current().feed.KeyMap() }

// selected returns the repository under the cursor.
func (t *repoTabs) selected() (core.Repo, bool) {
	return t.current().feed.Selected()
}

// update passes msg to the lists, which ignore the messages of others.
func (t *repoTabs) update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		var cmd tea.Cmd
		o := t.current()
		o.feed, cmd = o.feed.Update(msg)
		return cmd
	}
	cmds := make([]tea.Cmd, 0, len(t.tabs)+1)
	for _, o := range t.tabs {
		var cmd tea.Cmd
		o.feed, cmd = o.feed.Update(msg)
		cmds = append(cmds, cmd)
		t.remeasure(o)
	}
	return tea.Batch(cmds...)
}

// selectRepo returns the command that opens repo on the repository screen.
func selectRepo(repo core.RepoRef) tea.Cmd {
	return func() tea.Msg { return ui.RepoMsg{Repo: repo} }
}
