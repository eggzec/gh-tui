package actions

import (
	"slices"
	"strconv"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// jobs are the jobs of the attempt of the run shown.
type jobs struct {
	runID   int64
	attempt int
	items   []core.Job
	// more reports that the run has more jobs than one page lists.
	more bool
	// loaded is set once a page arrived, and loading while one is read
	// with none to show yet.
	loaded, loading bool
	err             error
	cursor, top     int
}

func newJobs(r core.Run) jobs {
	return jobs{runID: r.ID, attempt: r.Attempt}
}

// selected returns the job under the cursor.
func (j *jobs) selected() (core.Job, bool) {
	if j.cursor < 0 || j.cursor >= len(j.items) {
		return core.Job{}, false
	}
	return j.items[j.cursor], true
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
		p, err := svc.Jobs(ctx, q)
		end(err, "span", "tui", "run", q.RunID, "attempt", q.Attempt, "jobs", len(p.Items), "offline", p.Offline)
		if p.Offline {
			off.Mark()
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
// job it was on. The log of the job under the cursor is read once the
// cursor rests, if rest is set.
func (m *Modal) setJobs(p core.Page[core.Job], rest bool) tea.Cmd {
	j := &m.jobs
	prev, had := j.selected()
	j.items, j.more = slices.Clone(p.Items), p.Next != ""
	switch {
	case had && j.loaded:
		// A new attempt has new IDs, but the same names.
		i := slices.IndexFunc(j.items, func(x core.Job) bool { return x.ID == prev.ID })
		if i < 0 {
			i = slices.IndexFunc(j.items, func(x core.Job) bool { return x.Name == prev.Name })
		}
		j.cursor = max(i, 0)
	default:
		j.cursor = firstJob(j.items)
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
	n := len(j.items)
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
	j.top = max(min(j.top, len(j.items)-h), 0)
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
		return ui.FitLines(ui.Wrap(m.errorLine("Couldn't load the jobs: ", j.err), w), w, h)
	case !j.loaded:
		return ui.FitLines([]string{m.spin.View() + st.Muted.Render("Loading the jobs…")}, w, h)
	case len(j.items) == 0:
		return ui.FitLines(ui.Wrap(st.Muted.Render(m.noJobsText()), w), w, h)
	}
	lines := make([]string, 0, h)
	focused := m.focus == jobsPane
	now := m.now()
	for i := j.top; i < len(j.items) && len(lines) < m.jobRows(); i++ {
		lines = append(lines, m.jobRow(j.items[i], i == j.cursor, focused, w, now))
	}
	if j.more && len(lines) < h {
		text := "More jobs than GitHub lists at once."
		if k := m.keys.Open.Help().Key; k != "" {
			text += " " + k + " shows them all."
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

// jobRow renders a job: its state and name, and how long it ran.
func (m *Modal) jobRow(j core.Job, cursor, focused bool, w int, now time.Time) string {
	st := &m.st
	gutter := st.noGutter
	if cursor {
		gutter = st.blurGutter
		if focused {
			gutter = st.gutter
		}
	}
	state := ui.RunStateOf(j.Status, j.Conclusion)
	name := st.Text.Render(ui.OneLine(j.Name))
	if cursor {
		name = st.Strong.Render(ui.OneLine(j.Name))
	}
	return ui.Spread(gutter+st.Glyphs[state]+" "+name, m.st.Took(j.Status, j.Conclusion, j.StartedAt, j.CompletedAt, now), w)
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

// errorLine renders an error that the refresh key reads again.
func (m *Modal) errorLine(what string, err error) string {
	text := m.st.Error.Render("✗ " + what + ui.FirstLine(err.Error()))
	if k := m.keys.Refresh.Help().Key; k != "" {
		text += m.st.Subtle.Render(" · " + k + " to retry")
	}
	return text
}
