package dashboard

import (
	"context"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/dashboard"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
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

	// all holds what the filter searches: the pages read in a row from the
	// first, up to dashboard.MaxOwnerRepos. next is the cursor after them,
	// and complete is set once there are no more.
	all      []core.Repo
	next     string
	complete bool
	filling  bool
	fillErr  error
}

// key identifies the owner, whose login GitHub matches regardless of case.
func (o *owner) key() string {
	if o.q.Viewer {
		return "@me"
	}
	return strings.ToLower(o.q.Owner)
}

// repoTabs is the repositories pane: a tab per owner, each a list of
// repositories paged as it scrolls, and a filter that finds one by name
// among up to dashboard.MaxOwnerRepos of them.
type repoTabs struct {
	s    *Section
	tabs []*owner
	cur  int

	filtering bool
	picker    picker.Model
	// fillCtx bounds the reads that fill the filter, and stopFill cancels
	// them.
	fillCtx  context.Context
	stopFill context.CancelFunc

	width, height int
}

// fillMsg carries a page read to fill the filter of the owner with key.
type fillMsg struct {
	id     int64
	owner  string
	cursor string
	page   core.Page[core.Repo]
	err    error
}

func newRepoTabs(s *Section) repoTabs {
	t := repoTabs{s: s, fillCtx: s.ctx, stopFill: func() {}}
	t.tabs = []*owner{t.newOwner(yours, dashboard.ReposQuery{Viewer: true})}
	return t
}

func (t *repoTabs) newOwner(label string, q dashboard.ReposQuery) *owner {
	s := t.s
	read := func(ctx context.Context, cursor string) (core.Page[core.Repo], error) {
		q := q
		q.Cursor = cursor
		return s.svc.Repos(ctx, q)
	}
	empty := "You own no repository yet."
	if !q.Viewer {
		empty = label + " has no repository you can see."
	}
	o := &owner{label: label, q: q}
	render := func(r core.Repo, selected bool, _ int) string { return s.renderRepo(o.cols, r, selected) }
	o.feed = feed.New(ui.FeedPages("dashboard.repos", s.offline, read), render,
		feed.WithContext(s.ctx),
		feed.WithKey(func(r core.Repo) string { return r.Ref.String() }),
		feed.WithKeyMap(s.keys.feed),
		feed.WithEmptyText(empty),
		feed.WithStyles(s.theme.Feed()),
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

// reload reads the pages of every started tab again, and forgets what the
// filters read.
func (t *repoTabs) reload() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(t.tabs))
	for _, o := range t.tabs {
		o.all, o.next, o.complete, o.fillErr = nil, "", false, nil
		if o.started {
			cmds = append(cmds, o.feed.Reload())
		}
	}
	return tea.Batch(cmds...)
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
	if t.filtering {
		return
	}
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
	if t.filtering {
		t.picker.SetSize(width, max(height-1, 0))
	}
}

func (t *repoTabs) setTheme(th ui.Theme) {
	for _, o := range t.tabs {
		o.feed.SetStyles(th.Feed())
	}
	if t.filtering {
		t.picker.SetStyles(pickerStyles(th))
	}
}

// feedKeys returns the keys of the list on view, whose retry the list
// enables only while a fetch has failed.
func (t *repoTabs) feedKeys() feed.KeyMap { return t.current().feed.KeyMap() }

// selected returns the repository under the cursor.
func (t *repoTabs) selected() (core.Repo, bool) {
	return t.current().feed.Selected()
}

// update passes msg to the lists and the filter, which ignore the messages
// of others.
func (t *repoTabs) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case fillMsg:
		return t.filled(msg)
	case tea.KeyPressMsg:
		if t.filtering {
			return t.pressFilter(msg)
		}
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
	if t.filtering {
		var cmd tea.Cmd
		t.picker, cmd = t.picker.Update(msg)
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

// pressFilter passes msg to the filter. Choosing and cancelling close it
// at once rather than when the picker's message comes back, so the keys
// after them reach the dashboard.
func (t *repoTabs) pressFilter(msg tea.KeyPressMsg) tea.Cmd {
	k := t.picker.KeyMap()
	switch {
	case key.Matches(msg, k.Choose):
		it, ok := t.picker.Selected()
		if !ok {
			return nil
		}
		t.closeFilter()
		if r, ok := it.Value.(core.Repo); ok {
			return selectRepo(r.Ref)
		}
		return nil
	case key.Matches(msg, k.Cancel):
		t.closeFilter()
		return nil
	}
	var cmd tea.Cmd
	t.picker, cmd = t.picker.Update(msg)
	return cmd
}

// openFilter finds a repository of the tab on view among those read so
// far, and reads the rest in the background, a page at a time, up to
// dashboard.MaxOwnerRepos.
func (t *repoTabs) openFilter() tea.Cmd {
	o, s := t.current(), t.s
	first := o.q
	first.Cursor = ""
	if p, ok := s.svc.CachedAllRepos(first, dashboard.MaxOwnerRepos); ok && len(p.Items) >= len(o.all) {
		o.all, o.next = slices.Clone(p.Items), p.Next
		o.complete = p.Next == "" || len(p.Items) >= dashboard.MaxOwnerRepos
	}
	o.fillErr = nil
	o.feed.Blur()
	t.filtering = true
	t.picker = picker.New(nil,
		picker.WithItems(repoItems(o.all)),
		picker.WithContext(s.ctx),
		picker.WithGroupHeaders(false),
		picker.WithPlaceholder("Find a repository of "+ownerName(o)),
		picker.WithEmptyText("No repository matches. esc goes back to the list."),
		picker.WithStyles(pickerStyles(s.theme)),
		picker.WithSize(t.width, max(t.height-1, 0)),
	)
	t.fillCtx, t.stopFill = context.WithCancel(s.ctx)
	return tea.Batch(t.picker.Focus(), t.fill(o))
}

func (t *repoTabs) closeFilter() {
	t.stopFill()
	t.filtering = false
	t.picker.Blur()
	for _, o := range t.tabs {
		o.filling = false
	}
	if t.s.focused && t.s.focus == reposPane {
		t.current().feed.Focus()
	}
}

// fill reads the next page of o for its filter, unless it has them all.
func (t *repoTabs) fill(o *owner) tea.Cmd {
	if o.complete {
		o.filling = false
		return nil
	}
	o.filling = true
	ctx, svc, id, who, q := t.fillCtx, t.s.svc, t.s.id, o.key(), o.q
	q.Cursor = o.next
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "dashboard.repos.fill")
		p, err := svc.Repos(ctx, q)
		end(err, "span", "tui", "first", q.Cursor == "", "items", len(p.Items))
		return fillMsg{id: id, owner: who, cursor: q.Cursor, page: p, err: err}
	}
}

// filled adds a page to the filter, and reads the next one.
func (t *repoTabs) filled(msg fillMsg) tea.Cmd {
	if msg.id != t.s.id || !t.filtering {
		return nil
	}
	o := t.current()
	if o.key() != msg.owner || !o.filling || msg.cursor != o.next {
		return nil
	}
	if msg.err != nil {
		o.filling, o.fillErr = false, msg.err
		return ui.Notify(toast.Error, "Couldn't read every repository of "+ownerName(o)+": "+msg.err.Error())
	}
	o.all = append(slices.Clip(o.all), msg.page.Items...)
	o.next = msg.page.Next
	o.complete = o.next == "" || len(o.all) >= dashboard.MaxOwnerRepos
	t.picker.SetItems(repoItems(o.all))
	return t.fill(o)
}

func ownerName(o *owner) string {
	if o.q.Viewer {
		return "yours"
	}
	return o.label
}

// repoItems lists repos for the filter, which matches their names.
func repoItems(repos []core.Repo) []picker.Item {
	items := make([]picker.Item, len(repos))
	for i := range repos {
		r := &repos[i]
		items[i] = picker.Item{Title: r.Ref.Name, Detail: r.Description, Value: *r}
	}
	return items
}

func pickerStyles(th ui.Theme) picker.Styles {
	st := th.Picker()
	// The pane draws the frame.
	st.Frame = lipgloss.NewStyle()
	return st
}

// selectRepo returns the command that opens repo on the repository screen.
func selectRepo(repo core.RepoRef) tea.Cmd {
	return func() tea.Msg { return ui.RepoMsg{Repo: repo} }
}
