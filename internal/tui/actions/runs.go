package actions

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/feed"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/termtext"
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
	case f.Statuses != "" || f.Actors != "":
		return tabAll, false
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
	f.Status, f.Actor, f.Statuses, f.Actors = "", "", "", ""
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

// formFiltered reports whether f has something the filter form set: the
// tabs set the status and the actor, and a link to a commit the head SHA.
func formFiltered(f core.RunFilter) bool {
	return f.Branch != "" || f.Event != "" || f.WorkflowID != 0 || f.Words != "" ||
		f.Workflows != "" || f.Branches != "" || f.Events != "" || f.Statuses != "" || f.Actors != ""
}

// cleared returns f without what the filter form set, keeping the tab it
// is on and the commit it was opened for.
func cleared(f core.RunFilter) core.RunFilter {
	return core.RunFilter{Status: f.Status, Actor: f.Actor, HeadSHA: f.HeadSHA}
}

// setFilter shows the runs that f selects, from the first.
func (m *Modal) setFilter(f core.RunFilter) tea.Cmd {
	if f == m.filter {
		return nil
	}
	m.filter = f
	m.runs = m.newRuns()
	m.clearRun()
	m.aheadJobs.Reset(m.ctx)
	m.aheadLogs.Reset(m.ctx)
	return m.runs.Init()
}

var errNoViewer = errors.New("who you are is unknown")

// newRuns returns the feed of the runs that the filter selects.
func (m *Modal) newRuns() feed.Model[core.Run] {
	svc, repo, f, viewer := m.svc, m.repo, m.filter, m.opts.viewer
	query := func(cursor string) actionssvc.RunsQuery {
		return actionssvc.RunsQuery{Repo: repo, Filter: f.Remote(), Cursor: cursor}
	}
	words := ui.Words(f.Words)
	local := f
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
		keep := local
		if actors := keep.Actors.List(); slices.Contains(actors, me) {
			if viewer == nil {
				return core.Page[core.Run]{}, errNoViewer
			}
			login, err := viewer(ctx)
			if err != nil {
				return core.Page[core.Run]{}, err
			}
			keep.Actors = core.AnyOfValues(slices.Replace(slices.Clone(actors), slices.Index(actors, me), slices.Index(actors, me)+1, login)...)
		}
		p, err := svc.Runs(ctx, q)
		if keep.Multi() {
			// GitHub takes one value of a field, so a field of several is
			// matched here, and an emptied page leaves the feed to fetch the
			// next one.
			p.Items = slices.DeleteFunc(slices.Clone(p.Items), func(r core.Run) bool { return !keep.Keeps(r) })
		}
		if len(words) > 0 {
			// GitHub can't search the runs, so the page is cut to the matches.
			p.Items = slices.DeleteFunc(slices.Clone(p.Items), func(r core.Run) bool {
				return !ui.HasWords(words, r.DisplayTitle, r.Name)
			})
		}
		return p, err
	}
	runs := feed.New(ui.FeedPages("list.runs", query, read), m.renderRun,
		feed.WithContext(m.ctx),
		feed.WithFirstItem(),
		feed.WithKey(func(r core.Run) string { return strconv.FormatInt(r.ID, 10) }),
		feed.WithKeyMap(m.keys.Runs),
		feed.WithPromptKeys(m.keys.search),
		feed.WithStyles(m.theme.Feed(m.opts.icons)),
		feed.WithItemHeight(1),
		feed.WithChildren(m.jobCount, m.renderJob),
		feed.WithChildrenFolded(),
		feed.WithSize(m.paneWidth(runsPane), max(m.bodyHeight()-1, 0)),
		feed.WithFocused(m.focus == runsPane),
		feed.WithEmptyText(m.emptyText()),
		feed.WithErrorText(ui.ErrorText("load the runs", m.repo.String(), *m.opts.voice)),
	)
	// Nothing is loaded yet, so setting the order starts no read.
	_ = runs.SetGroups(m.groupFuncs())
	_ = runs.SetSort(m.sortFunc())
	return runs
}

// emptyText tells the user what to do when no runs match.
func (m *Modal) emptyText() string {
	if m.filter == (core.RunFilter{}) {
		return ui.None("workflow runs")
	}
	if formFiltered(m.filter) {
		return ui.NoMatch("workflow runs", ui.KeyOf(m.opts.icons, m.keys.ClearFilter))
	}
	return ui.Press("No workflow runs match the filters.", ui.KeyOf(m.opts.icons, m.keys.Filter), "change them")
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
	if p, ok := msg.(tea.KeyPressMsg); ok && m.onGroupRow() {
		// A group of jobs folds by its own state, which the feed doesn't hold.
		// Each key goes one way: the other cases are the feed's, which steps
		// up to the run or down to the first job.
		// The feed enables these keys on its own copy only, while its rows are
		// lined, which they are on a group.
		expand, collapse := m.keys.Runs.Expand, m.keys.Runs.Collapse
		expand.SetEnabled(true)
		collapse.SetEnabled(true)
		open := m.groupOpen()
		if keymap.Matches(p, expand) && !open || keymap.Matches(p, collapse) && open {
			m.toggleGroup()
			return nil
		}
	}
	return m.feedRuns(msg)
}

// feedRuns passes msg to the feed without the folding of the groups of jobs,
// and follows its cursor.
func (m *Modal) feedRuns(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.runs, cmd = m.runs.Update(msg)
	if m.runs.Seeking() {
		// The run to open on isn't listed yet: showing the first would read
		// its jobs for nothing.
		return cmd
	}
	m.dropJobsOf()
	sel := m.selectRun()
	// A run opened or folded, or one that left the feed, changes what to keep
	// up to date.
	m.follow()
	return tea.Batch(cmd, sel, m.readJobsAround(), m.startTick())
}

// shownAt is where the cursor was, a run and the row of its jobs, when the job
// shown was last chosen.
type shownAt struct {
	run int64
	k   int
	ok  bool
}

// cursorRun returns the run under the cursor, or the run a job row is of.
func (m *Modal) cursorRun() (core.Run, bool) {
	if r, _, ok := m.runs.SelectedChild(); ok {
		return r, true
	}
	return m.runs.Selected()
}

// cursorJob returns the job under the cursor, when it is on a row of the jobs
// of the run shown, or the job a group row opens on.
func (m *Modal) cursorJob() (core.Job, bool) {
	j := m.cur()
	k, ok := m.childOf(j)
	if !ok {
		return core.Job{}, false
	}
	return j.selectedAt(k)
}

// onGroupRow reports whether the cursor is on a group of jobs.
func (m *Modal) onGroupRow() bool {
	j := m.cur()
	k, ok := m.childOf(j)
	return ok && j.onGroupAt(k)
}

// groupOpen reports whether the group of jobs under the cursor is open.
func (m *Modal) groupOpen() bool {
	j := m.cur()
	k, ok := m.childOf(j)
	if !ok || !j.onGroupAt(k) {
		return false
	}
	return j.isOpen(j.lines[k].group.key)
}

// toggleGroup folds the group under the cursor, or opens it.
func (m *Modal) toggleGroup() {
	j := m.cur()
	if k, ok := m.childOf(j); ok && j.onGroupAt(k) {
		j.toggleAt(k)
		m.runs.Resort()
	}
}

// jobCount is how many rows the jobs of r take under it.
func (m *Modal) jobCount(r core.Run) int {
	if j := m.jobsOf[r.ID]; j != nil {
		return j.count()
	}
	return 1
}

// selectRun shows the jobs of the run under the cursor, once it changed:
// those in memory at once, and the others once the cursor rests. While it
// stays, it follows the cursor among the jobs.
func (m *Modal) selectRun() tea.Cmd {
	r, ok := m.cursorRun()
	if !ok {
		return nil
	}
	r = m.current(r)
	if m.hasRun && r.ID == m.run.ID {
		m.run = r
		// The cursor may have moved among the jobs of the run.
		_, k, onChild := m.runs.SelectedChild()
		if at := (shownAt{r.ID, k, onChild}); at != m.at {
			m.at = at
			return m.showJob()
		}
		return nil
	}
	m.at = shownAt{}
	first := !m.hasRun
	m.run, m.hasRun = r, true
	j := m.entry(r)
	// A failed read of an earlier visit must not show instead of the loading
	// line, and what was served from the cache then may be fresh now.
	j.err, j.kept = nil, false
	m.noteCursor, m.noteTop = 0, 0
	// The logs read ahead for the run shown before stop.
	m.aheadLogs.Reset(m.ctx)
	var cmd tea.Cmd
	if p, ok := m.svc.CachedAllJobs(m.jobsQuery()); ok {
		m.aheadJobs.Opened(m.jobsQuery())
		cmd = m.setJobs(m.cur(), p)
	} else {
		m.cur().loading = true
		cmd = m.startSpinner()
	}
	m.follow()
	// The first run shows at once; the others wait for the cursor to rest,
	// so that a scroll doesn't read each one.
	var read tea.Cmd
	if first {
		read = m.readJobs(m.run)
	} else {
		read = m.rest()
	}
	return tea.Batch(cmd, read, m.startTick())
}

// clearRun forgets the run shown, for a new list of runs.
func (m *Modal) clearRun() {
	m.run, m.hasRun = core.Run{}, false
	m.at = shownAt{}
	m.noteCursor, m.noteTop = 0, 0
	m.unfollow()
}

// cur returns the jobs of the run shown, or empty ones when none is.
func (m *Modal) cur() *jobs {
	if j := m.jobsOf[m.run.ID]; j != nil && m.hasRun {
		return j
	}
	return &jobs{}
}

// entry returns the jobs kept for r, made anew when there are none or they
// are of another attempt. Jobs kept keep their items, folds, cursor and
// scroll, so that a run comes back as it was left; selectRun resets what a
// new visit must not show, its error and its kept mark.
func (m *Modal) entry(r core.Run) *jobs {
	if j := m.jobsOf[r.ID]; j != nil && j.attempt == r.Attempt {
		return j
	}
	if m.jobsOf == nil {
		m.jobsOf = map[int64]*jobs{}
	}
	j := newJobs(r)
	m.jobsOf[r.ID] = &j
	return &j
}

// dropJobsOf forgets the jobs of the runs that are no longer in the feed.
func (m *Modal) dropJobsOf() {
	// Until every run is loaded, one that isn't yet may still be in the feed.
	if len(m.jobsOf) == 0 || !m.runs.Settled() || !m.runs.Done() {
		return
	}
	listed := map[int64]bool{}
	for i := range m.runs.Len() {
		if r, ok := m.runs.LoadedItem(i); ok {
			listed[r.ID] = true
		}
	}
	for id := range m.jobsOf {
		if !listed[id] && (!m.hasRun || id != m.run.ID) {
			delete(m.jobsOf, id)
		}
	}
}

// renderRun renders a run in one line, the parent of its jobs in the tree:
// its state, workflow, number and title, cut to the
// width. What else it has shows in the details.
func (m *Modal) renderRun(r core.Run, selected bool, w int) string {
	r = m.current(r)
	st := &m.st.row[ui.Look(selected, m.inRuns())]
	state := ui.RunStateOf(r.Status, r.Conclusion)
	name := st.Strong.Render(ui.OneLine(r.Name))
	// The workflow, the number and the title link to the run's page.
	head := st.Glyphs[state] + " " + m.links.Link(r.URL,
		name+" "+st.Muted.Render("#"+strconv.Itoa(r.Number))+" "+st.Text.Render(ui.OneLine(r.DisplayTitle)))
	return termtext.Truncate(head, w, m.opts.icons.Ellipsis)
}

// runsCount is the forms of the count at the bottom of the runs box, the
// longest first: how many are loaded, with a "+" while there are more.
func (m *Modal) runsCount() []string {
	n := m.runs.Len()
	return countForms(n, !m.runs.Done(), "run", "runs")
}

// countForms are the forms of "n things" for a bottom slot: none for 0, and
// the bare number as the short form.
func countForms(n int, more bool, one, many string) []string {
	if n == 0 {
		return nil
	}
	s := strconv.Itoa(n)
	if more {
		s += "+"
	}
	word := many
	if n == 1 && !more {
		word = one
	}
	return []string{s + " " + word, s}
}

// filterText is the filter as a query, for the title of the runs when it
// is more than a tab.
func (m *Modal) filterText() string {
	if _, ok := tabOf(m.filter); ok && !formFiltered(m.filter) {
		return ""
	}
	f := m.filter
	f.Words = ""
	return strings.TrimSpace(ansi.Strip(queryOf(f, m.workflows.items)) + " " + ui.QuoteWords(m.filter.Words))
}
