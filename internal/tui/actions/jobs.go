package actions

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// jobs are the jobs of the attempt of the run shown.
type jobs struct {
	runID   int64
	attempt int
	items   []core.Job
	// tree nests the items in their groups, and lines are the rows it
	// shows, as the groups are folded. folds holds the folds of the
	// groups by key, so they stay while polls add jobs, and for a new
	// attempt.
	tree  []jobNode
	lines []jobLine
	folds map[string]fold
	// more reports that the run has more jobs than the pages read list.
	more bool
	// loaded is set once a page arrived, and loading while one is read.
	loaded, loading bool
	err             error
	// kept reports that the jobs shown were served from what an earlier
	// read kept, because GitHub couldn't be reached or rate limited the
	// read, so they are read again once it answers. Only a read clears
	// it: the cache holds such a page without its marks, so a page from
	// CachedAllJobs may be the kept one.
	kept        bool
	cursor, top int
}

func newJobs(r core.Run) jobs {
	return jobs{runID: r.ID, attempt: r.Attempt, folds: map[string]fold{}}
}

// fold is the fold of a group of jobs: whether it is open, whether the
// user folded or opened it, and whether one of its jobs had failed when
// the jobs were last read.
type fold struct {
	open, chosen, failed bool
}

// isOpen reports whether the group of key k is open.
func (j *jobs) isOpen(k string) bool {
	return j.folds[k].open
}

// jobLine is a row of the jobs pane: a job, or a group, depth groups in.
type jobLine struct {
	jobNode
	depth int
}

// selected returns the job under the cursor, or the job a group under it
// opens on: its first that failed, else that runs, else its first.
func (j *jobs) selected() (core.Job, bool) {
	if j.cursor < 0 || j.cursor >= len(j.lines) {
		return core.Job{}, false
	}
	l := j.lines[j.cursor]
	if l.group != nil {
		return j.items[l.group.first], true
	}
	return j.items[l.job], true
}

// onGroup reports whether the cursor is on a group.
func (j *jobs) onGroup() bool {
	return j.cursor >= 0 && j.cursor < len(j.lines) && j.lines[j.cursor].group != nil
}

// regroup nests the items in their groups. A group opens when one of its
// jobs fails, or had failed when it was first seen, so failures show at
// once, unless the user folded or opened it.
func (j *jobs) regroup() {
	j.tree = groupJobs(j.items)
	var visit func([]jobNode)
	visit = func(nodes []jobNode) {
		for _, n := range nodes {
			g := n.group
			if g == nil {
				continue
			}
			f := j.folds[g.key]
			failed := slices.ContainsFunc(g.jobs, func(i int) bool { return j.items[i].Conclusion.Failed() })
			if failed && !f.failed && !f.chosen {
				f.open = true
			}
			f.failed = failed
			j.folds[g.key] = f
			visit(g.kids)
		}
	}
	visit(j.tree)
	j.relines()
}

// reveal opens the groups the job of index i is in, such as one a poll
// just made of it and the jobs that followed it, so the job stays in
// sight.
func (j *jobs) reveal(i int) {
	var visit func([]jobNode) bool
	visit = func(nodes []jobNode) bool {
		for _, n := range nodes {
			if g := n.group; g != nil && slices.Contains(g.jobs, i) {
				f := j.folds[g.key]
				f.open = true
				j.folds[g.key] = f
				return visit(g.kids)
			}
			if n.job == i {
				return true
			}
		}
		return false
	}
	visit(j.tree)
	j.relines()
}

// relines lays the rows out as the groups are folded.
func (j *jobs) relines() {
	j.lines = nil
	var walk func([]jobNode, int)
	walk = func(nodes []jobNode, depth int) {
		for _, n := range nodes {
			j.lines = append(j.lines, jobLine{jobNode: n, depth: depth})
			if n.group != nil && j.isOpen(n.group.key) {
				walk(n.group.kids, depth+1)
			}
		}
	}
	walk(j.tree, 0)
}

// lineOf returns the row of the job of index i: its own, or that of the
// folded group it is in.
func (j *jobs) lineOf(i int) int {
	for k, l := range j.lines {
		if l.job == i || l.group != nil && !j.isOpen(l.group.key) && slices.Contains(l.group.jobs, i) {
			return k
		}
	}
	return 0
}

// toggle folds the group under the cursor, or opens it.
func (j *jobs) toggle() {
	g := j.lines[j.cursor].group
	f := j.folds[g.key]
	f.open, f.chosen = !f.open, true
	j.folds[g.key] = f
	// The rows before the group stay, so the cursor stays on it.
	j.relines()
}

// jobsQuery reads the jobs of the attempt of the run shown, which is kept
// for good once the attempt completed.
func (m *Modal) jobsQuery() actionssvc.JobsQuery {
	return actionssvc.JobsQuery{Repo: m.repo, RunID: m.cur().runID, Attempt: m.cur().attempt}
}

// jobsMsg carries the jobs of an attempt of a run.
type jobsMsg struct {
	id      int64
	runID   int64
	attempt int
	page    core.Page[core.Job]
	err     error
}

// readJobs reads the jobs of the attempt of run.
func (m *Modal) readJobs(run core.Run) tea.Cmd {
	if !m.hasRun {
		return nil
	}
	svc, ctx, id := m.svc, m.ctx, m.id
	q := actionssvc.JobsQuery{Repo: m.repo, RunID: run.ID, Attempt: run.Attempt}
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.jobs")
		p, err := svc.AllJobs(ctx, q)
		end(err, "span", "tui", "run", q.RunID, "attempt", q.Attempt, "jobs", len(p.Items), "offline", p.Offline, "limited", p.Limited)
		return jobsMsg{id: id, runID: q.RunID, attempt: q.Attempt, page: p, err: err}
	}
}

// receiveJobs keeps a page in the entry of its run. Only the run shown has
// its cursor, log and notes follow the page.
func (m *Modal) receiveJobs(msg jobsMsg) tea.Cmd {
	j := m.jobsOf[msg.runID]
	if !m.hasRun || j == nil || msg.attempt != j.attempt {
		return nil
	}
	j.loading = false
	if msg.err != nil {
		// Jobs shown already stay, and the error only shows without them.
		j.err = msg.err
		return nil
	}
	j.err, j.kept = nil, msg.page.Offline || msg.page.Limited
	return m.setJobs(j, msg.page)
}

// setJobs shows page. The first page of a run opens on its first failed
// job, or else the first running one; a later one keeps the cursor on the
// group or the job it was on.
func (m *Modal) setJobs(j *jobs, p core.Page[core.Job]) tea.Cmd {
	// A run that isn't shown takes the page into its entry and nothing else:
	// the deep-link job, the scroll, the log and the notes are the shown run's.
	shown := m.hasRun && j == m.jobsOf[m.run.ID]
	want := int64(0)
	if shown {
		want = m.wantJob
		m.wantJob = 0
	}
	prev, had := j.selected()
	var group string
	if j.onGroup() {
		group = j.lines[j.cursor].group.key
	}
	j.items, j.more = slices.Clone(p.Items), p.Next != ""
	j.regroup()
	switch {
	case had && j.loaded:
		if i := slices.IndexFunc(j.lines, func(l jobLine) bool { return l.group != nil && l.group.key == group }); i >= 0 {
			j.cursor = i
			break
		}
		// A new attempt has new IDs, but the same names.
		i := slices.IndexFunc(j.items, func(x core.Job) bool { return x.ID == prev.ID })
		if i < 0 {
			i = slices.IndexFunc(j.items, func(x core.Job) bool { return x.Name == prev.Name })
		}
		if i >= 0 && group == "" {
			// The cursor stays on its job, and so does the log, even if a
			// poll made a group of it.
			j.reveal(i)
		}
		j.cursor = j.lineOf(max(i, 0))
	default:
		i := firstJob(j.items)
		if k := slices.IndexFunc(j.items, func(x core.Job) bool { return x.ID == want }); want != 0 && k >= 0 {
			i = k
			j.reveal(k)
		}
		j.cursor = j.lineOf(i)
	}
	// A read in flight still clears loading when it arrives, so that a
	// poll that shows the cached jobs meanwhile doesn't let a wake start
	// a second read.
	j.loaded = true
	if !shown {
		return nil
	}
	m.scrollJobs()
	return tea.Batch(m.showJob(), m.readNotes())
}

// firstJob is the job a run opens on: the first that failed, else the first
// that runs, else the first.
func firstJob(items []core.Job) int {
	if i := slices.IndexFunc(items, func(j core.Job) bool { return j.Conclusion.Failed() }); i >= 0 {
		return i
	}
	if i := slices.IndexFunc(items, func(j core.Job) bool { return j.Status == core.RunInProgress }); i >= 0 {
		return i
	}
	return 0
}

// pressJobs moves the cursor of the jobs, and shows the log of the job it
// rests on.
func (m *Modal) pressJobs(msg tea.KeyPressMsg) tea.Cmd {
	j, k := m.cur(), m.keys.Jobs
	n := len(j.lines)
	if n == 0 {
		if keymap.Matches(msg, k.Up) {
			// While the jobs load there is nothing to move through but the
			// run they belong to.
			m.setFocus(runsPane)
		}
		return nil
	}
	page := max(m.jobRows()-1, 1)
	to := j.cursor
	switch {
	case keymap.Matches(msg, k.Up):
		to--
	case keymap.Matches(msg, k.Down):
		to++
	case keymap.Matches(msg, k.PageUp):
		to -= page
	case keymap.Matches(msg, k.PageDown):
		to += page
	case keymap.Matches(msg, k.HalfPageUp):
		to -= max(page/2, 1)
	case keymap.Matches(msg, k.HalfPageDown):
		to += max(page/2, 1)
	case keymap.Matches(msg, k.Home):
		to = 0
	case keymap.Matches(msg, k.End):
		to = n - 1
	default:
		return nil
	}
	if to < 0 && j.cursor == 0 && keymap.Matches(msg, k.Up) {
		// Up from the first job is the run it belongs to.
		m.setFocus(runsPane)
		return nil
	}
	if to >= n && j.cursor == n-1 && keymap.Matches(msg, k.Down) {
		// Down from the last job is the next run.
		m.setFocus(runsPane)
		return m.updateRuns(msg)
	}
	to = min(max(to, 0), n-1)
	if to == j.cursor {
		return nil
	}
	j.cursor = to
	m.scrollJobs()
	return m.showJob()
}

// scrollJobs keeps the cursor of the jobs in view.
func (m *Modal) scrollJobs() {
	j := m.cur()
	h := max(m.jobRows(), 1)
	j.top = min(j.top, j.cursor)
	// The rows are counted in the lines they take, as a long name wraps.
	for j.top < j.cursor && m.linesSpan(j.top, j.cursor+1) > h {
		j.top++
	}
	for j.top > 0 && m.linesSpan(j.top-1, len(j.lines)) <= h {
		j.top--
	}
	j.top = max(j.top, 0)
}

// linesSpan is how many lines the rows from to to take, to exclusive.
func (m *Modal) linesSpan(from, to int) int {
	n := 0
	for i := from; i < to; i++ {
		n += len(m.nameLines(i, m.inner))
	}
	return n
}

// jobRows is how many jobs the jobs section shows, less the line that says
// there are more.
func (m *Modal) jobRows() int {
	h := max(m.rowsOf(jobsPane), 0)
	if m.cur().more {
		h--
	}
	return max(h, 0)
}

// jobLines renders the jobs pane's body, h lines of w cells.
func (m *Modal) jobLines(w, h int) []string {
	j, st := m.cur(), &m.st
	if note, ok := m.jobsNote(w); ok {
		return ui.FitLines(note, w, h)
	}
	lines := make([]string, 0, h)
	focused := m.inJobs()
	now := m.now()
	for i := j.top; i < len(j.lines) && len(lines) < m.jobRows(); i++ {
		lines = append(lines, m.lineRows(i, i == j.cursor, focused, w, now)...)
	}
	lines = lines[:min(len(lines), m.jobRows())]
	if j.more && len(lines) < h {
		text := "First " + strconv.Itoa(len(j.items)) + " jobs"
		if k := m.keys.Open.Help().Key; k != "" {
			text += st.ic.Separator + st.ic.Key(k) + " shows all"
		}
		lines = append(lines, ui.Fit(st.noGutter+st.Subtle.Render(termtext.Truncate(text, w-2, st.ic.Ellipsis)), w))
	}
	return ui.PadLines(lines, w, h)
}

// jobsNote returns what shows in place of the jobs while there are none to
// list: the error, the loading, or why there are no jobs, in lines of w
// cells. It reports false when there are jobs to list.
func (m *Modal) jobsNote(w int) ([]string, bool) {
	j, st := m.cur(), &m.st
	// Under a run, the note sits where the job rows do.
	indent := st.noGutter + m.jobsIndent()
	room := max(w-ansi.StringWidth(indent), 1)
	var note []string
	switch {
	case !m.hasRun:
		return []string{st.Muted.Render("Pick a run to see its jobs.")}, true
	case !j.loaded && j.err != nil:
		note = m.errorLines("load the jobs", jobview.RunName(m.run), j.err, room)
	case !j.loaded:
		note = []string{m.spin.View() + st.Muted.Render("Loading the jobs"+st.ic.Ellipsis)}
	case len(j.items) == 0:
		note = ui.Wrap(st.Muted.Render(m.noJobsText()), room)
	default:
		return nil, false
	}
	for i, l := range note {
		note[i] = indent + l
	}
	return note, true
}

// noJobsText tells why the run shown has no jobs, and what to do: a run
// of a pull request from a fork waits for a maintainer, and one that
// couldn't start shows why only on GitHub.
func (m *Modal) noJobsText() string {
	var text string
	switch m.run.Conclusion {
	case core.ConclusionActionRequired:
		text = "Waiting for a maintainer to approve this run"
	case core.ConclusionStartupFailure:
		text = "This run failed to start, often for an error in its workflow file"
	case core.ConclusionSkipped:
		return "This run was skipped, so none of its jobs ran."
	default:
		return "This run has no jobs yet."
	}
	if k := m.keys.Open.Help().Key; k != "" {
		text += m.st.ic.Separator + m.st.ic.Key(k) + " opens it on GitHub"
	}
	return text
}

// rowMark is what stands before the name of line i: the state of a job, or
// whether a group is open.
func (m *Modal) rowMark(i int) string {
	cur := m.cur()
	l := cur.lines[i]
	if g := l.group; g != nil {
		if cur.isOpen(g.key) {
			return m.st.unfolded
		}
		return m.st.folded
	}
	j := &cur.items[l.job]
	return m.st.Glyphs[ui.RunStateOf(j.Status, j.Conclusion)] + " "
}

// nameLines is the name of line i, in as many lines as it takes to show it
// whole after its guides and mark, in w cells.
func (m *Modal) nameLines(i, w int) []string {
	l := &m.cur().lines[i]
	// Each guide, and the branch, is two cells.
	head := ansi.StringWidth(m.st.noGutter) + len(m.jobsIndent()) + 2*(l.depth+1) + ansi.StringWidth(m.rowMark(i))
	room := max(w-head, 1)
	if ansi.StringWidth(l.label) <= room {
		return []string{l.label}
	}
	return ui.Wrap(l.label, room)
}

// jobsIndent is the space before the outline of the jobs: the indent the runs
// have under their groups, and one level more, so the jobs nest under the run
// they hang from.
func (m *Modal) jobsIndent() string {
	return strings.Repeat(" ", 2*(len(m.groupFuncs())+1))
}

// lineRows renders a row of the jobs, a job or a group, indented as deep as
// it is nested. Its name wraps under its guides where it doesn't fit, and
// the row then drops what it shows at the right.
func (m *Modal) lineRows(i int, cursor, focused bool, w int, now time.Time) []string {
	st := &m.st
	cur := m.cur()
	l := cur.lines[i]
	style := st.Text
	if cursor {
		style = st.Strong
	}
	var right, url string
	if g := l.group; g != nil {
		// A group has no page to link.
		right = st.Subtle.Render(strconv.Itoa(len(g.jobs))) + " " + st.Glyphs[g.state]
	} else {
		j := &cur.items[l.job]
		right = st.Took(j.Status, j.Conclusion, j.StartedAt, j.CompletedAt, now)
		url = j.URL
	}
	paint := func(s string) string {
		s = style.Render(s)
		if url == "" {
			return s
		}
		return m.links.Link(url, s)
	}
	mark := m.rowMark(i)
	names := m.nameLines(i, w)
	indent := m.jobsIndent()
	head := m.rowGutter(cursor, focused) + indent + st.Subtle.Render(m.guides(i, false)) + mark
	if len(names) == 1 {
		return []string{m.spreadWhole(head+paint(names[0]), right, w)}
	}
	rows := []string{ui.Fit(head+paint(names[0]), w)}
	rest := st.noGutter + indent + st.Subtle.Render(m.guides(i, true)) + strings.Repeat(" ", ansi.StringWidth(mark))
	for _, n := range names[1:] {
		rows = append(rows, ui.Fit(rest+paint(n), w))
	}
	return rows
}

// guides draws the outline in front of line i: a bar for each ancestor with
// more rows to come, and a branch to the row, which ends on its last sibling.
// For the lines a name wraps onto, the branch is a bar or a gap.
func (m *Modal) guides(i int, wrapped bool) string {
	bar, branch, last := "│ ", "├ ", "└ "
	if m.st.ic.Set() == config.IconsASCII {
		bar, branch, last = "| ", "|-", "`-"
	}
	lines := m.cur().lines
	// more reports that a row of depth d follows line i before the level
	// closes.
	more := func(d int) bool {
		for _, n := range lines[i+1:] {
			if n.depth < d {
				return false
			}
			if n.depth == d {
				return true
			}
		}
		return false
	}
	d := lines[i].depth
	var b strings.Builder
	for k := range d {
		// The ancestor at depth k continues while a row of its depth follows.
		if more(k) {
			b.WriteString(bar)
		} else {
			b.WriteString("  ")
		}
	}
	switch {
	case wrapped && more(d):
		b.WriteString(bar)
	case wrapped:
		b.WriteString("  ")
	case more(d):
		b.WriteString(branch)
	default:
		b.WriteString(last)
	}
	return b.String()
}

// spreadWhole puts right at the end of the row after left, and drops right
// when the two don't fit, so the name on the left is never cut for it.
func (m *Modal) spreadWhole(left, right string, w int) string {
	if ansi.StringWidth(left)+1+ansi.StringWidth(right) > w {
		return ui.Fit(left, w)
	}
	return ui.Spread(left, right, w, m.opts.icons.Ellipsis)
}

// rowGutter renders the mark of the row under the cursor, or its blank.
func (m *Modal) rowGutter(cursor, focused bool) string {
	switch {
	case cursor && focused:
		return m.st.gutter
	case cursor:
		return m.st.blurGutter
	}
	return m.st.noGutter
}

// jobsTitle is the detail of the jobs pane's title: how many there are,
// and how many failed.
func (m *Modal) jobsTitle() string {
	j := m.cur()
	if !j.loaded || len(j.items) == 0 {
		return ""
	}
	s := strconv.Itoa(len(j.items))
	if j.more {
		s += "+"
	}
	failed := 0
	for i := range j.items {
		if j.items[i].Conclusion.Failed() {
			failed++
		}
	}
	if failed > 0 {
		s += m.st.ic.Separator + strconv.Itoa(failed) + " failed"
	}
	return s
}

// errorLines renders err, which stopped action on object, in lines of w
// cells. Access is granted by repository, so a refusal names it rather than
// object. The refresh key reads it again, and the open key opens the run.
func (m *Modal) errorLines(action, object string, err error, w int) []string {
	v := *m.opts.voice
	v.Retry, v.Open = m.keys.Refresh, m.keys.Open
	if errors.Is(err, core.ErrForbidden) {
		object = m.repo.String()
	}
	text, hint := ui.ErrorText(action, object, v)(err)
	return ui.ErrorLine(m.errs, text, hint, w)
}
