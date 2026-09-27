package actions

import (
	"context"
	"errors"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
)

// me stands for the signed-in user in a filter until a read asks who that
// is, as @me does in GitHub's queries.
const me = "@me"

// tab is one of the tabs of the runs: a preset of the filter's status and
// actor.
type tab int

const (
	tabAll tab = iota
	tabFailing
	tabRunning
	tabMine
	numTabs
)

var tabNames = []string{"All", "Failing", "Running", "Mine"}

// Statuses of the Failing and Running tabs, as the runs API takes them.
const (
	statusFailing = string(core.ConclusionFailure)
	statusRunning = string(core.RunInProgress)
)

// tabOf returns the tab that f matches, if any.
func tabOf(f core.RunFilter) (tab, bool) {
	switch {
	case f.Status == "" && f.Actor == "":
		return tabAll, true
	case f.Status == statusFailing && f.Actor == "":
		return tabFailing, true
	case f.Status == statusRunning && f.Actor == "":
		return tabRunning, true
	case f.Status == "" && f.Actor == me:
		return tabMine, true
	}
	return tabAll, false
}

// apply returns f with the status and actor of tab t, keeping the rest.
func (t tab) apply(f core.RunFilter) core.RunFilter {
	f.Status, f.Actor = "", ""
	switch t {
	case tabFailing:
		f.Status = statusFailing
	case tabRunning:
		f.Status = statusRunning
	case tabMine:
		f.Actor = me
	case tabAll, numTabs:
	}
	return f
}

// numTabs is how many tabs the modal has: Mine needs to know who the user
// is.
func (m *Modal) numTabs() int {
	if m.opts.viewer == nil {
		return int(tabMine)
	}
	return int(numTabs)
}

// switchTab shows the tab d after the one shown, or the first when the
// filter matches none.
func (m *Modal) switchTab(d int) tea.Cmd {
	n := m.numTabs()
	t, ok := tabOf(m.filter)
	next := tabAll
	if ok {
		next = tab((int(t) + d + n) % n)
	}
	return m.setFilter(next.apply(m.filter))
}

// setFilter shows the runs that f selects, from the first.
func (m *Modal) setFilter(f core.RunFilter) tea.Cmd {
	if f == m.filter {
		return nil
	}
	m.filter = f
	m.runs = m.newRuns()
	m.clearRun()
	return m.runs.Init()
}

var errNoViewer = errors.New("who you are is unknown")

// newRuns returns the feed of the runs that the filter selects.
func (m *Modal) newRuns() feed.Model[core.Run] {
	svc, repo, f, viewer := m.svc, m.repo, m.filter, m.opts.viewer
	query := func(cursor string) actionssvc.RunsQuery {
		return actionssvc.RunsQuery{Repo: repo, Filter: f, Cursor: cursor}
	}
	read := func(ctx context.Context, q actionssvc.RunsQuery, again bool) (core.Page[core.Run], error) {
		q.Again = again
		if q.Filter.Actor == me {
			if viewer == nil {
				return core.Page[core.Run]{}, errNoViewer
			}
			login, err := viewer(ctx)
			if err != nil {
				return core.Page[core.Run]{}, err
			}
			q.Filter.Actor = login
		}
		return svc.Runs(ctx, q)
	}
	return feed.New(ui.FeedPages("list.runs", m.opts.offline, query, read), m.renderRun,
		feed.WithContext(m.ctx),
		feed.WithKey(func(r core.Run) string { return strconv.FormatInt(r.ID, 10) }),
		feed.WithKeyMap(m.keys.List),
		feed.WithStyles(m.theme.Feed()),
		feed.WithItemHeight(2),
		feed.WithSize(m.paneWidth(runsPane), m.bodyHeight()),
		feed.WithFocused(m.focus == runsPane),
		feed.WithEmptyText(m.emptyText()),
		feed.WithErrorText(ui.ErrorText("load the runs", m.repo.String(), *m.opts.voice)),
	)
}

// emptyText tells the user what to do when no runs match.
func (m *Modal) emptyText() string {
	if m.filter == (core.RunFilter{}) {
		return "No workflow runs yet."
	}
	hint := "No runs match."
	if k := m.keys.Filter.Help().Key; k != "" {
		hint += " " + k + " changes the filter."
	}
	return hint
}

// current returns r as the cache last had it after a change or a poll,
// unless the feed read it again since.
func (m *Modal) current(r core.Run) core.Run {
	if l, ok := m.live[r.ID]; ok && !l.UpdatedAt.Before(r.UpdatedAt) {
		return l
	}
	return r
}

// updateRuns passes msg to the feed, and follows its cursor.
func (m *Modal) updateRuns(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.runs, cmd = m.runs.Update(msg)
	return tea.Batch(cmd, m.selectRun())
}

// selectRun shows the jobs of the run under the cursor, once it changed:
// those in memory at once, and the others once the cursor rests.
func (m *Modal) selectRun() tea.Cmd {
	r, ok := m.runs.Selected()
	if !ok {
		return nil
	}
	r = m.current(r)
	if m.hasRun && r.ID == m.run.ID {
		m.run = r
		return nil
	}
	first := !m.hasRun
	m.run, m.hasRun = r, true
	m.jobs = newJobs(r)
	m.log.Clear()
	var cmd tea.Cmd
	if p, ok := m.svc.CachedAllJobs(m.jobsQuery()); ok {
		cmd = m.setJobs(p, !first)
	} else {
		m.jobs.loading = true
		cmd = m.startSpinner()
	}
	m.follow()
	// The first run shows at once; the others wait for the cursor to rest,
	// so that a scroll doesn't read each one.
	var read tea.Cmd
	if first {
		read = m.readJobs()
	} else {
		read = m.rest()
	}
	return tea.Batch(cmd, read, m.startTick())
}

// clearRun forgets the run shown, for a new list of runs.
func (m *Modal) clearRun() {
	m.run, m.hasRun = core.Run{}, false
	m.jobs = jobs{}
	m.log.Clear()
	m.unfollow()
}

// renderRun renders a run in two lines: its state, workflow, number,
// title and age, and below them its branch, event, actor and how long it
// ran.
func (m *Modal) renderRun(r core.Run, selected bool, w int) string {
	r = m.current(r)
	st := &m.st
	now := m.now()
	state := ui.RunStateOf(r.Status, r.Conclusion)
	name := st.Strong.Render(ui.OneLine(r.Name))
	if !selected {
		name = st.Text.Render(ui.OneLine(r.Name))
	}
	// The workflow, the number and the title link to the run's page.
	head := st.Glyphs[state] + " " + m.links.Link(r.URL,
		name+" "+st.Muted.Render("#"+strconv.Itoa(r.Number))+"  "+st.Text.Render(ui.OneLine(r.DisplayTitle)))
	first := ui.Spread(head, st.Subtle.Render(ui.Ago(r.CreatedAt, now)), w)

	parts := make([]string, 0, 3)
	for _, p := range []string{r.Branch, r.Event, r.Actor} {
		if p = ui.OneLine(p); p != "" {
			parts = append(parts, p)
		}
	}
	detail := "  " + st.Muted.Render(strings.Join(parts, " · "))
	var took string
	switch d, ok := runSpan(r, now); {
	case r.Status == core.RunCancelling:
		took = st.Warning.Render("cancelling")
	case !r.Done() && r.Status != core.RunInProgress:
		took = st.Warning.Render(ui.StatusText(r.Status))
	case !ok:
	case r.Done():
		took = st.Subtle.Render(ui.Duration(d))
	default:
		took = st.States[ui.RunInProgress].Render(ui.Duration(d))
	}
	return first + "\n" + ui.Spread(detail, took, w)
}

// runsTitle is the detail of the runs pane's title: how many are loaded.
func (m *Modal) runsTitle() string {
	n := m.runs.Len()
	if n == 0 {
		return ""
	}
	s := strconv.Itoa(n)
	if !m.runs.Done() {
		s += "+"
	}
	return s
}

// filterText is the filter as a query, for the title of the runs when it
// is more than a tab.
func (m *Modal) filterText() string {
	if _, ok := tabOf(m.filter); ok && m.filter.Branch == "" && m.filter.Event == "" && m.filter.WorkflowID == 0 {
		return ""
	}
	return ansi.Strip(queryOf(m.filter, m.workflows.items))
}
