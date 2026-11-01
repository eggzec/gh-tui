package actions

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// jobs are the jobs of the attempt of the run shown.
type jobs struct {
	runID   int64
	attempt int
	items   []core.Job
	// tree nests the items in their groups, and lines are the rows it
	// shows under the run, as the groups are folded. folds holds the folds
	// of the groups by key, so they stay while polls add jobs, and for a new
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
	kept bool
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

// jobLine is a row under a run: a job, or a group, depth groups in.
type jobLine struct {
	jobNode
	depth int
}

// selectedAt returns the job of row k, or the job a group opens on: its first
// that failed, else that runs, else its first.
func (j *jobs) selectedAt(k int) (core.Job, bool) {
	if k < 0 || k >= len(j.lines) {
		return core.Job{}, false
	}
	l := j.lines[k]
	if l.group != nil {
		return j.items[l.group.first], true
	}
	return j.items[l.job], true
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

// onGroupAt reports whether row k is a group.
func (j *jobs) onGroupAt(k int) bool {
	return k >= 0 && k < len(j.lines) && j.lines[k].group != nil
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

// toggleAt folds the group of row k, or opens it.
func (j *jobs) toggleAt(k int) {
	g := j.lines[k].group
	f := j.folds[g.key]
	f.open, f.chosen = !f.open, true
	j.folds[g.key] = f
	j.relines()
}

// count is how many rows the jobs take under their run: one for the note
// that stands for them while there are none, else a row each, and one that
// says there are more.
func (j *jobs) count() int {
	n := len(j.lines)
	switch {
	case n == 0:
		return 1
	case j.more:
		return n + 1
	}
	return n
}

// childOf returns the row of the jobs of j that the cursor is on, if it is
// on one.
func (m *Modal) childOf(j *jobs) (int, bool) {
	r, k, ok := m.runs.SelectedChild()
	return k, ok && r.ID == j.runID
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
// its log and notes follow the page.
func (m *Modal) receiveJobs(msg jobsMsg) tea.Cmd {
	j := m.jobsOf[msg.runID]
	if !m.hasRun || j == nil || msg.attempt != j.attempt {
		return nil
	}
	j.loading = false
	if msg.err != nil {
		// Jobs shown already stay, and the error only shows without them.
		j.err = msg.err
		m.runs.Resort()
		return nil
	}
	j.err, j.kept = nil, msg.page.Offline || msg.page.Limited
	return m.setJobs(j, msg.page)
}

// setJobs shows page. The cursor stays where it is, on the group or the job
// it was on if it was on one of these rows, and a deep link to a job
// moves it there.
func (m *Modal) setJobs(j *jobs, p core.Page[core.Job]) tea.Cmd {
	// A run that isn't shown takes the page into its entry and nothing else:
	// the deep-link job, the log and the notes are the shown run's.
	shown := m.hasRun && j == m.jobsOf[m.run.ID]
	want := int64(0)
	if shown {
		want = m.wantJob
		m.wantJob = 0
	}
	var prev core.Job
	var group string
	var had bool
	if k, ok := m.childOf(j); ok {
		prev, had = j.selectedAt(k)
		if j.onGroupAt(k) {
			group = j.lines[k].group.key
		}
	}
	j.items, j.more = slices.Clone(p.Items), p.Next != ""
	j.regroup()
	at := -1
	switch {
	case had:
		if i := slices.IndexFunc(j.lines, func(l jobLine) bool { return l.group != nil && l.group.key == group }); i >= 0 {
			at = i
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
		at = j.lineOf(max(i, 0))
	case want != 0:
		if k := slices.IndexFunc(j.items, func(x core.Job) bool { return x.ID == want }); k >= 0 {
			j.reveal(k)
			at = j.lineOf(k)
		}
	}
	// A read in flight still clears loading when it arrives, so that a
	// poll that shows the cached jobs meanwhile doesn't let a wake start
	// a second read.
	j.loaded = true
	m.runs.Resort()
	if at >= 0 {
		m.runs.SelectChild(strconv.FormatInt(j.runID, 10), at)
	}
	if !shown {
		return nil
	}
	return tea.Batch(m.showJob(), m.readNotes())
}

// jobsNote is the row that stands for the jobs of run while there are none
// to list: the error, the loading, or why there are no jobs, in one line of
// w cells. It reports false when there are jobs to list.
func (m *Modal) jobsNote(j *jobs, run core.Run, w int) (string, bool) {
	st := &m.st
	// Under a run, the note sits where the job rows do.
	indent := strings.Repeat(" ", jobsIndent)
	room := max(w-jobsIndent, 1)
	var note string
	switch {
	case !j.loaded && j.err != nil:
		lines := m.errorLines("load the jobs", jobview.RunName(run), j.err, room)
		if len(lines) > 0 {
			note = lines[0]
		}
	case !j.loaded:
		note = m.spin.View() + st.Muted.Render("Loading the jobs"+st.ic.Ellipsis)
	case len(j.items) == 0:
		note = st.Muted.Render(termtext.Truncate(m.noJobsText(run), room, st.ic.Ellipsis))
	default:
		return "", false
	}
	return ui.Fit(indent+note, w), true
}

// jobsIndent is the indent of the jobs under their run.
const jobsIndent = 2

// noJobsText tells why run has no jobs, and what to do: a run of a pull
// request from a fork waits for a maintainer, and one that couldn't start
// shows why only on GitHub.
func (m *Modal) noJobsText(run core.Run) string {
	var text string
	switch run.Conclusion {
	case core.ConclusionActionRequired:
		text = "Waiting for a maintainer to approve this run"
	case core.ConclusionStartupFailure:
		text = "This run failed to start, often for an error in its workflow file"
	case core.ConclusionSkipped:
		return "This run was skipped, so none of its jobs ran."
	default:
		return "This run has no jobs yet."
	}
	return text
}

// renderJob renders row k under run in one line: its jobs, a group of them,
// the line that says there are more, or what stands for them while there are
// none. Its name is cut to the width, and the row drops what it shows at the
// right before the name is cut short.
func (m *Modal) renderJob(run core.Run, k int, selected bool, w int) string {
	st := &m.st
	j := m.jobsOf[run.ID]
	if j == nil {
		j = &jobs{}
	}
	if note, ok := m.jobsNote(j, run, w); ok {
		return note
	}
	if k >= len(j.lines) {
		text := "First " + strconv.Itoa(len(j.items)) + " jobs"
		return ui.Fit(strings.Repeat(" ", jobsIndent)+st.Subtle.Render(termtext.Truncate(text, w-jobsIndent, st.ic.Ellipsis)), w)
	}
	row := &st.row[ui.Look(selected, m.inRuns())]
	style := row.Text
	if selected {
		style = row.Strong
	}
	l := j.lines[k]
	var right, url, mark string
	if g := l.group; g != nil {
		// A group has no page to link.
		right = st.Subtle.Render(strconv.Itoa(len(g.jobs))) + " " + st.Glyphs[g.state]
		mark = st.folded
		if j.isOpen(g.key) {
			mark = st.unfolded
		}
	} else {
		x := &j.items[l.job]
		right = st.Took(x.Status, x.Conclusion, x.StartedAt, x.CompletedAt, m.now())
		url = x.URL
		mark = st.Glyphs[ui.RunStateOf(x.Status, x.Conclusion)] + " "
	}
	indent := strings.Repeat(" ", jobsIndent*(l.depth+1))
	room := w - ansi.StringWidth(indent+mark)
	if rw := ansi.StringWidth(right); room-rw-1 >= 8 {
		room -= rw + 1
	} else {
		right = ""
	}
	name := style.Render(termtext.Truncate(ui.OneLine(l.label), max(room, 1), st.ic.Ellipsis))
	if url != "" {
		name = m.links.Link(url, name)
	}
	return m.spreadWhole(indent+mark+name, right, w)
}

// spreadWhole puts right at the end of the row after left, and drops right
// when the two don't fit, so the name on the left is never cut for it.
func (m *Modal) spreadWhole(left, right string, w int) string {
	if ansi.StringWidth(left)+1+ansi.StringWidth(right) > w {
		return ui.Fit(left, w)
	}
	return termtext.Spread(left, right, w, m.opts.icons.Ellipsis)
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

// errorLines renders err, which stopped action on object, in lines of w
// cells. Access is granted by repository, so a refusal names it rather than
// object. The refresh key reads it again.
func (m *Modal) errorLines(action, object string, err error, w int) []string {
	v := *m.opts.voice
	v.Retry = m.keys.Refresh
	if errors.Is(err, core.ErrForbidden) {
		object = m.repo.String()
	}
	text, hint := ui.ErrorText(action, object, v)(err)
	return ui.ErrorLine(m.errs, text, hint, w)
}
