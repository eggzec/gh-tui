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
// of WithRest if rest is set, and the annotations of a failed job with it.
// A job that hasn't finished shows its steps, or once it runs what GitHub
// publishes of its log, and its whole log once it is shown again,
// finished. Showing the job shown again keeps its log, and only takes its
// steps and what the poll read of a partial log.
func (m *Model) Show(j core.Job, rest bool, h Hints) tea.Cmd {
	defer m.layout()
	running := m.state == Pending || m.state == Partial
	if j.ID == m.job.ID && m.state != None && (!running || !j.Done()) {
		m.job, m.hints = j, h
		if running {
			// A job that started, or a view shown again after a pause, has
			// the poll read on.
			if m.unwatch == nil && j.Status == core.RunInProgress {
				m.unwatch = m.svc.WatchLog(m.repo, j.RunID, j.ID)
			}
			m.fromPartial()
		}
		return nil
	}
	m.Clear()
	m.job, m.hints = j, h
	if !j.Done() {
		m.state = Pending
		if j.Status != core.RunInProgress {
			// A job that waits for a runner has no log yet.
			return nil
		}
		return m.watch(rest)
	}
	notes := m.cachedNotes()
	if lg, ok := m.svc.CachedLog(m.repo, j.ID); ok {
		m.setLog(lg)
		if notes {
			return nil
		}
		return m.readNotes()
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
	if m.unwatch != nil {
		m.unwatch()
	}
	m.job, m.hints, m.state, m.truncated, m.resting = core.Job{}, Hints{}, None, false, false
	m.part, m.unwatch = shownPart{}, nil
	m.view.SetTitle("")
	m.notes = notes{}
	m.focusLog(true)
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

// ReadLost reads again what was being read while the view was hidden,
// such as behind a file preview opened from an annotation, whose answers
// went to the preview and were lost: the annotations, and the log unless
// it waited for a rest, which is lost too and read at once instead.
func (m *Model) ReadLost() tea.Cmd {
	if m.state == None {
		return nil
	}
	var notes tea.Cmd
	if m.notes.loading {
		notes = m.readNotes()
	}
	if m.state != Loading {
		return notes
	}
	// A rest or a read still to come is stale.
	m.resting = false
	m.seq++
	return tea.Batch(notes, m.read())
}

// Err returns why the log failed to load, else why the annotations did,
// or nil if neither failed.
func (m Model) Err() error {
	if m.state == Failed {
		return m.failed
	}
	return m.notes.err
}

// Retry reads the log or the annotations again once they failed to load,
// and does nothing otherwise.
func (m *Model) Retry() tea.Cmd {
	var notes tea.Cmd
	if m.notes.err != nil {
		m.notes.err = nil
		notes = m.readNotes()
	}
	if m.state != Failed {
		return notes
	}
	m.state, m.failed = Loading, nil
	return tea.Batch(notes, m.view.SetLoading(), m.read())
}

// restMsg reports that the rest of seq ended.
type restMsg struct {
	id  int64
	seq int
}

// logMsg carries the log of a job, and what the view shows of it, which
// the read prepared, since a big log takes a while to prepare.
type logMsg struct {
	id    int64
	jobID int64
	log   core.Log
	shown shownLog
	err   error
}

// read reads the log of the job shown, and its annotations unless they are
// in memory.
func (m *Model) read() tea.Cmd {
	if m.job.ID == 0 {
		return nil
	}
	if !m.job.Done() {
		return m.readPartial()
	}
	var notes tea.Cmd
	if !m.notes.loaded && !m.notes.loading {
		notes = m.readNotes()
	}
	svc, ctx, id, repo, jobID := m.svc, m.ctx, m.id, m.repo, m.job.ID
	// The view and the steps as they are now, since the command runs
	// apart from Update.
	view, steps, now := m.view, m.job.Steps, m.opts.now
	return tea.Batch(notes, func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.log")
		l, err := svc.Log(ctx, repo, jobID)
		end(err, "span", "tui", "job", jobID, "lines", len(l.Lines), "truncated", l.Truncated)
		msg := logMsg{id: id, jobID: jobID, log: l, err: err}
		if err == nil {
			msg.shown = prepare(view, l, steps, now())
		}
		return msg
	})
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
		m.state, m.failed = Failed, msg.err
		m.view.SetError(msg.err)
	default:
		m.show(msg.log, msg.shown)
	}
}

// setLog shows lg, the log of the job shown, with its steps as sections,
// and the failed step open on its first error. A log without errors opens
// on its steps, folded, as the steps of a job in progress show.
func (m *Model) setLog(lg core.Log) {
	m.show(lg, prepare(m.view, lg, m.job.Steps, m.opts.now()))
}

// show shows lg as setLog does, from what prepare made of it.
func (m *Model) show(lg core.Log, p shownLog) {
	m.showLines(p)
	m.state, m.truncated = Ready, lg.Truncated
}

// shownLog is a log read into the lines of the view, with whether a step
// failed.
type shownLog struct {
	log    logview.Log
	failed bool
}

// prepare reads lg into the lines of view, with steps as its sections as
// of now. It only reads view, so it may run in a tea.Cmd.
func prepare(view logview.Model, lg core.Log, steps []core.Step, now time.Time) shownLog {
	lines, secs := logLines(lg, steps, now)
	return shownLog{
		log:    view.Prepare(lines, secs),
		failed: slices.ContainsFunc(secs, func(s logview.Section) bool { return s.Failed }),
	}
}

// showLines shows p in the view, folded as setLog says.
func (m *Model) showLines(p shownLog) {
	m.view.SetLog(p.log)
	if m.view.Errors() == 0 && !p.failed {
		m.view.CollapseAll()
	}
}

// logLines turns a parsed log into the lines of a log view, and the steps
// of its job into sections of the lines each step wrote.
func logLines(lg core.Log, steps []core.Step, now time.Time) ([]logview.Line, []logview.Section) {
	return viewLines(lg.Lines), stepSections(lg.Lines, steps, now)
}

// viewLines turns lines of a parsed log into lines of a log view.
func viewLines(lines []core.LogLine) []logview.Line {
	out := make([]logview.Line, len(lines))
	for i, ln := range lines {
		out[i] = logview.Line{Time: ln.Time, Text: ln.Text, Kind: logKind(ln.Kind)}
	}
	return out
}

// stepSections returns the sections of the lines each of steps wrote.
func stepSections(lines []core.LogLine, steps []core.Step, now time.Time) []logview.Section {
	byNumber := make(map[int]core.Step, len(steps))
	for _, st := range steps {
		byNumber[st.Number] = st
	}
	var secs []logview.Section
	cur := 0
	for i, ln := range lines {
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
	return secs
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
