package actions

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
)

// logState is what the log pane shows.
type logState int

const (
	// logNone shows no job.
	logNone logState = iota
	// logPending shows the steps of a job that hasn't finished, whose log
	// GitHub doesn't publish yet.
	logPending
	logLoading
	logReady
	// logExpired shows the steps of a job whose log GitHub no longer
	// keeps.
	logExpired
	logFailed
)

// jobLog is the log of the job under the cursor of the jobs.
type jobLog struct {
	view  logview.Model
	jobID int64
	state logState
	// truncated reports that only the end of the log was read.
	truncated bool
	// resting is set while the log waits for the cursor of the jobs to
	// rest before it is read.
	resting bool
}

func newJobLog(k logview.KeyMap) jobLog {
	return jobLog{view: logview.New(logview.WithFocusFailed(true), logview.WithKeyMap(k))}
}

// clear shows no job.
func (l *jobLog) clear() {
	l.jobID, l.state, l.truncated, l.resting = 0, logNone, false, false
}

// showJob shows the log of the job under the cursor of the jobs: from
// memory at once, or read, once the cursor rests if rest is set. A job
// that hasn't finished shows its steps, and its log once it finished.
func (m *Modal) showJob(rest bool) tea.Cmd {
	j, ok := m.jobs.selected()
	if !ok {
		m.log.clear()
		m.layout()
		return nil
	}
	l := &m.log
	if j.ID == l.jobID && l.state != logNone && (l.state != logPending || !j.Done()) {
		return nil
	}
	l.clear()
	l.jobID = j.ID
	defer m.layout()
	if !j.Done() {
		l.state = logPending
		return nil
	}
	if lg, ok := m.svc.CachedLog(m.repo, j.ID); ok {
		m.setLog(j, lg)
		return nil
	}
	l.state = logLoading
	spin := l.view.SetLoading()
	if rest {
		l.resting = true
		return tea.Batch(spin, m.rest(restLog))
	}
	// A read now makes a rest in flight stale.
	m.seq[restLog]++
	return tea.Batch(spin, m.readLog())
}

// logMsg carries the log of a job.
type logMsg struct {
	id    int64
	jobID int64
	log   core.Log
	err   error
}

// readLog reads the log of the job the log pane shows.
func (m *Modal) readLog() tea.Cmd {
	if m.log.jobID == 0 {
		return nil
	}
	svc, ctx, id, repo, jobID := m.svc, m.ctx, m.id, m.repo, m.log.jobID
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.log")
		l, err := svc.Log(ctx, repo, jobID)
		end(err, "span", "tui", "job", jobID, "lines", len(l.Lines), "truncated", l.Truncated)
		return logMsg{id: id, jobID: jobID, log: l, err: err}
	}
}

func (m *Modal) receiveLog(msg logMsg) {
	l := &m.log
	if msg.jobID != l.jobID || l.state != logLoading {
		return
	}
	defer m.layout()
	switch {
	case errors.Is(msg.err, core.ErrLogPending):
		l.state = logPending
	case errors.Is(msg.err, core.ErrLogExpired):
		l.state = logExpired
	case msg.err != nil:
		l.state = logFailed
		l.view.SetError(msg.err)
	default:
		j, _ := m.jobs.selected()
		m.setLog(j, msg.log)
	}
}

// setLog shows lg, the log of j, with the steps of j as sections, and the
// failed step open on its first error. A log without errors opens on its
// steps, folded, as the steps of a job in progress show.
func (m *Modal) setLog(j core.Job, lg core.Log) {
	lines, secs := logLines(lg, j.Steps, m.now())
	m.log.view.SetLines(lines, secs)
	if m.log.view.Errors() == 0 && !slices.ContainsFunc(secs, func(s logview.Section) bool { return s.Failed }) {
		m.log.view.CollapseAll()
	}
	m.log.state, m.log.truncated = logReady, lg.Truncated
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
		title := oneLine(st.Name)
		if !ok || title == "" {
			title = "Step " + strconv.Itoa(cur)
		}
		d, _ := stepSpan(st, now)
		secs = append(secs, logview.Section{Title: title, Start: i, End: len(lines), Failed: st.Conclusion.Failed(), Duration: d})
	}
	return lines, secs
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

// logNotice is the line above the log, if any: that only its end was read.
func (m *Modal) logNotice() string {
	if m.log.state != logReady || !m.log.truncated {
		return ""
	}
	text := "Only the end of this log: it is too large to read whole."
	if k := m.keys.Open.Help().Key; k != "" {
		text += " " + k + " opens it on GitHub."
	}
	return m.st.warning.Render(text)
}

// logBody renders the log pane's body, h lines of w cells.
func (m *Modal) logBody(w, h int) []string {
	st := &m.st
	switch m.log.state {
	case logNone:
		text := "Pick a job to see its log."
		if m.hasRun && m.jobs.loaded && len(m.jobs.items) == 0 {
			text = "No jobs, so no log."
		}
		return fitLines([]string{st.muted.Render(text)}, w, h)
	case logPending, logExpired:
		return m.stepLines(w, h)
	case logLoading, logReady, logFailed:
	}
	var lines []string
	if n := m.logNotice(); n != "" {
		lines = wrap(n, w, "")[:1]
	}
	lines = append(lines, strings.Split(m.log.view.View(), "\n")...)
	return padLines(lines, w, h)
}

// stepLines renders the steps of the job shown, for a job whose log isn't
// there to read, after why.
func (m *Modal) stepLines(w, h int) []string {
	st := &m.st
	j, _ := m.jobs.selected()
	var why string
	switch {
	case m.log.state == logExpired:
		why = st.muted.Render("GitHub no longer keeps this log.")
	case j.Status == core.RunInProgress:
		why = st.muted.Render("The log is available when the job finishes.")
	default:
		why = st.muted.Render("The job hasn't started yet.")
	}
	lines := wrap(why, w, "")
	if len(j.Steps) == 0 {
		return fitLines(lines, w, h)
	}
	lines = append(lines, strings.Repeat(" ", w))
	now := m.now()
	for _, s := range j.Steps {
		if len(lines) >= h {
			break
		}
		state := ui.RunStateOf(s.Status, s.Conclusion)
		lines = append(lines, spread(st.glyphs[state]+" "+st.text.Render(oneLine(s.Name)), m.took(s.Status, s.Conclusion, s.StartedAt, s.CompletedAt, now), w))
	}
	return fitLines(lines, w, h)
}

// logTitle names the job of the log pane.
func (m *Modal) logTitle() string {
	j, ok := m.jobs.selected()
	if !ok || m.log.state == logNone {
		return ""
	}
	return oneLine(j.Name)
}
