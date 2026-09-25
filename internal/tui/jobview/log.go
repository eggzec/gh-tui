package jobview

import (
	"errors"
	"slices"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

// Show shows job j: its log from memory at once, or read, after the rest
// of WithRest if rest is set. A job that hasn't finished shows its steps,
// and its log once it is shown again, finished. Showing the job shown
// again keeps its log, and only takes its steps.
func (m *Model) Show(j core.Job, rest bool) tea.Cmd {
	defer m.layout()
	if j.ID == m.job.ID && m.state != None && (m.state != Pending || !j.Done()) {
		m.job = j
		return nil
	}
	m.Clear()
	m.job = j
	if !j.Done() {
		m.state = Pending
		return nil
	}
	if lg, ok := m.svc.CachedLog(m.repo, j.ID); ok {
		m.setLog(lg)
		return nil
	}
	m.state = Loading
	spin := m.view.SetLoading()
	// A read now, or a new rest, makes a rest in flight stale.
	m.seq++
	if rest && m.opts.rest > 0 {
		m.resting = true
		msg := restMsg{id: m.id, seq: m.seq}
		return tea.Batch(spin, tea.Tick(m.opts.rest, func(time.Time) tea.Msg { return msg }))
	}
	return tea.Batch(spin, m.read())
}

// Clear shows no job.
func (m *Model) Clear() {
	m.job, m.state, m.truncated, m.resting = core.Job{}, None, false, false
	m.layout()
}

// ReadNow reads the log of the job shown at once, if it waits for a rest,
// such as when the user moves to it.
func (m *Model) ReadNow() tea.Cmd {
	if !m.resting {
		return nil
	}
	m.resting = false
	m.seq++
	return m.read()
}

// Retry reads the log again once it failed to load, and does nothing
// otherwise.
func (m *Model) Retry() tea.Cmd {
	if m.state != Failed {
		return nil
	}
	m.state = Loading
	return tea.Batch(m.view.SetLoading(), m.read())
}

// restMsg reports that the rest of seq ended.
type restMsg struct {
	id  int64
	seq int
}

// logMsg carries the log of a job.
type logMsg struct {
	id    int64
	jobID int64
	log   core.Log
	err   error
}

// read reads the log of the job shown.
func (m *Model) read() tea.Cmd {
	if m.job.ID == 0 {
		return nil
	}
	svc, ctx, id, repo, jobID := m.svc, m.ctx, m.id, m.repo, m.job.ID
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.log")
		l, err := svc.Log(ctx, repo, jobID)
		end(err, "span", "tui", "job", jobID, "lines", len(l.Lines), "truncated", l.Truncated)
		return logMsg{id: id, jobID: jobID, log: l, err: err}
	}
}

func (m *Model) receive(msg logMsg) {
	if msg.jobID != m.job.ID || m.state != Loading {
		return
	}
	defer m.layout()
	switch {
	case errors.Is(msg.err, core.ErrLogPending):
		m.state = Pending
	case errors.Is(msg.err, core.ErrLogExpired):
		m.state = Expired
	case msg.err != nil:
		m.state = Failed
		m.view.SetError(msg.err)
	default:
		m.setLog(msg.log)
	}
}

// setLog shows lg, the log of the job shown, with its steps as sections,
// and the failed step open on its first error. A log without errors opens
// on its steps, folded, as the steps of a job in progress show.
func (m *Model) setLog(lg core.Log) {
	lines, secs := logLines(lg, m.job.Steps, m.opts.now())
	m.view.SetLines(lines, secs)
	if m.view.Errors() == 0 && !slices.ContainsFunc(secs, func(s logview.Section) bool { return s.Failed }) {
		m.view.CollapseAll()
	}
	m.state, m.truncated = Ready, lg.Truncated
}

// logLines turns a parsed log into the lines of a log view, and the steps
// of its job into sections of the lines each step wrote.
func logLines(lg core.Log, steps []core.Step, now time.Time) ([]logview.Line, []logview.Section) {
	byNumber := make(map[int]core.Step, len(steps))
	for _, st := range steps {
		byNumber[st.Number] = st
	}
	lines := make([]logview.Line, len(lg.Lines))
	var secs []logview.Section
	cur := 0
	for i, ln := range lg.Lines {
		lines[i] = logview.Line{Time: ln.Time, Text: ln.Text, Kind: logKind(ln.Kind)}
		if ln.Step == 0 || ln.Step == cur {
			continue
		}
		if n := len(secs); n > 0 {
			secs[n-1].End = i
		}
		cur = ln.Step
		st, ok := byNumber[cur]
		title := ui.OneLine(st.Name)
		if !ok || title == "" {
			title = "Step " + strconv.Itoa(cur)
		}
		d, _ := stepSpan(st, now)
		secs = append(secs, logview.Section{Title: title, Start: i, End: len(lines), Failed: st.Conclusion.Failed(), Duration: d})
	}
	return lines, secs
}

// stepSpan is how long st ran, or has run so far.
func stepSpan(st core.Step, now time.Time) (time.Duration, bool) {
	if st.Conclusion == core.ConclusionSkipped {
		return 0, false
	}
	var end time.Time
	if st.Status == core.RunCompleted {
		end = st.CompletedAt
	}
	return ui.Span(st.StartedAt, end, now)
}

// logKind is the kind of a log view's line for the kind of a parsed log's.
func logKind(k core.LogKind) logview.Kind {
	switch k {
	case core.LogGroup:
		return logview.Group
	case core.LogEndGroup:
		return logview.EndGroup
	case core.LogError:
		return logview.Error
	case core.LogWarning:
		return logview.Warning
	case core.LogNotice:
		return logview.Notice
	case core.LogDebug:
		return logview.Debug
	case core.LogCommand:
		return logview.Command
	default:
		return logview.Plain
	}
}
