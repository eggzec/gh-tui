package actions

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
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
	// loaded is set once a page arrived, and loading while one is read
	// with none to show yet.
	loaded, loading bool
	err             error
	cursor, top     int
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
	return actionssvc.JobsQuery{Repo: m.repo, RunID: m.jobs.runID, Attempt: m.jobs.attempt}
}

// jobsMsg carries the jobs of an attempt of a run.
type jobsMsg struct {
	id      int64
	runID   int64
	attempt int
	page    core.Page[core.Job]
	err     error
}

// readJobs reads the jobs of the run shown.
func (m *Modal) readJobs() tea.Cmd {
	if !m.hasRun {
		return nil
	}
	svc, ctx, id, q, off := m.svc, m.ctx, m.id, m.jobsQuery(), m.opts.offline
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.jobs")
		p, err := svc.AllJobs(ctx, q)
		end(err, "span", "tui", "run", q.RunID, "attempt", q.Attempt, "jobs", len(p.Items), "offline", p.Offline, "limited", p.Limited)
		switch {
		case p.Offline:
			off.Mark()
		case p.Limited:
			off.MarkLimited()
		}
		return jobsMsg{id: id, runID: q.RunID, attempt: q.Attempt, page: p, err: err}
	}
}

func (m *Modal) receiveJobs(msg jobsMsg) tea.Cmd {
	if !m.hasRun || msg.runID != m.jobs.runID || msg.attempt != m.jobs.attempt {
		return nil
	}
	m.jobs.loading = false
	if msg.err != nil {
		// Jobs shown already stay, and the error only shows without them.
		m.jobs.err = msg.err
		return nil
	}
	m.jobs.err = nil
	return m.setJobs(msg.page, false)
}

// setJobs shows page. The first page of a run opens on its first failed
// job, or else the first running one; a later one keeps the cursor on the
// group or the job it was on. The log of the job under the cursor is read
// once the cursor rests, if rest is set.
func (m *Modal) setJobs(p core.Page[core.Job], rest bool) tea.Cmd {
	j := &m.jobs
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
		j.cursor = j.lineOf(firstJob(j.items))
	}
	j.loaded = true
	j.loading = false
	m.scrollJobs()
	return m.showJob(rest)
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
	j, k := &m.jobs, m.keys.List
	n := len(j.lines)
	if n == 0 {
		return nil
	}
	page := max(m.bodyHeight()-1, 1)
	to := j.cursor
	switch {
	case key.Matches(msg, k.Up):
		to--
	case key.Matches(msg, k.Down):
		to++
	case key.Matches(msg, k.PageUp):
		to -= page
	case key.Matches(msg, k.PageDown):
		to += page
	case key.Matches(msg, k.Home):
		to = 0
	case key.Matches(msg, k.End):
		to = n - 1
	default:
		return nil
	}
	to = min(max(to, 0), n-1)
	if to == j.cursor {
		return nil
	}
	j.cursor = to
	m.scrollJobs()
	return m.showJob(true)
}

// scrollJobs keeps the cursor of the jobs in view.
func (m *Modal) scrollJobs() {
	j := &m.jobs
	h := max(m.jobRows(), 1)
	j.top = min(j.top, j.cursor)
	if j.cursor >= j.top+h {
		j.top = j.cursor - h + 1
	}
	j.top = max(min(j.top, len(j.lines)-h), 0)
}

// jobRows is how many jobs the pane shows.
func (m *Modal) jobRows() int {
	h := m.bodyHeight()
	if m.jobs.more {
		h--
	}
	return h
}

// jobLines renders the jobs pane's body, h lines of w cells.
func (m *Modal) jobLines(w, h int) []string {
	j, st := &m.jobs, &m.st
	switch {
	case !m.hasRun:
		return ui.FitLines([]string{st.Muted.Render("Pick a run to see its jobs.")}, w, h)
	case !j.loaded && j.err != nil:
		return ui.FitLines(m.errorLines("load the jobs", jobview.RunName(m.run), j.err, w), w, h)
	case !j.loaded:
		return ui.FitLines([]string{m.spin.View() + st.Muted.Render("Loading the jobs…")}, w, h)
	case len(j.items) == 0:
		return ui.FitLines(ui.Wrap(st.Muted.Render(m.noJobsText()), w), w, h)
	}
	lines := make([]string, 0, h)
	focused := m.focus == jobsPane
	now := m.now()
	for i := j.top; i < len(j.lines) && len(lines) < m.jobRows(); i++ {
		lines = append(lines, m.lineRow(j.lines[i], i == j.cursor, focused, w, now))
	}
	if j.more && len(lines) < h {
		text := "First " + strconv.Itoa(len(j.items)) + " jobs"
		if k := m.keys.Open.Help().Key; k != "" {
			text += " · " + k + " shows all"
		}
		lines = append(lines, ui.Fit(st.noGutter+st.Subtle.Render(ansi.Truncate(text, w-2, "…")), w))
	}
	return ui.PadLines(lines, w, h)
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
		text += " · " + k + " opens it on GitHub"
	}
	return text
}

// lineRow renders a row of the jobs, a job or a group, indented as deep
// as it is nested.
func (m *Modal) lineRow(l jobLine, cursor, focused bool, w int, now time.Time) string {
	gutter := m.rowGutter(cursor, focused) + strings.Repeat("  ", l.depth)
	if l.group != nil {
		return m.groupRow(l, gutter, cursor, w)
	}
	return m.jobRow(m.jobs.items[l.job], l.label, gutter, cursor, w, now)
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

// jobRow renders a job: its state and label, and how long it ran.
func (m *Modal) jobRow(j core.Job, label, gutter string, cursor bool, w int, now time.Time) string {
	st := &m.st
	name := st.Text.Render(label)
	if cursor {
		name = st.Strong.Render(label)
	}
	state := ui.RunStateOf(j.Status, j.Conclusion)
	return ui.Spread(gutter+st.Glyphs[state]+" "+m.links.Link(j.URL, name), st.Took(j.Status, j.Conclusion, j.StartedAt, j.CompletedAt, now), w)
}

// groupRow renders a group: whether it is open, its name, how many jobs
// it holds, and the state of the worst. A long name gives way to the
// count and the state. A group has no page to link.
func (m *Modal) groupRow(l jobLine, gutter string, cursor bool, w int) string {
	st, g := &m.st, l.group
	name := st.Text.Render(l.label)
	if cursor {
		name = st.Strong.Render(l.label)
	}
	marker := st.folded
	if m.jobs.isOpen(g.key) {
		marker = st.unfolded
	}
	return ui.Spread(gutter+marker+name, st.Subtle.Render(strconv.Itoa(len(g.jobs)))+" "+st.Glyphs[g.state], w)
}

// jobsTitle is the detail of the jobs pane's title: how many there are,
// and how many failed.
func (m *Modal) jobsTitle() string {
	j := &m.jobs
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
		s += " · " + strconv.Itoa(failed) + " failed"
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
