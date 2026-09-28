package jobview

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// shownPart is what the view shows of a partial log: the lines of its
// generation, as of when they were read.
type shownPart struct {
	gen, lines int
	at         time.Time
}

// watch has the poll of the run of the job shown, which is in progress,
// read what GitHub adds to its log while the view shows it. What was read
// shows at once; otherwise the log is read now, or after the rest of
// WithRest if rest is set.
func (m *Model) watch(rest bool) tea.Cmd {
	m.unwatch = m.svc.WatchLog(m.repo, m.job.RunID, m.job.ID)
	if m.fromPartial() {
		return nil
	}
	m.seq++
	if rest && m.opts.rest > 0 {
		m.resting = true
		msg := restMsg{id: m.id, seq: m.seq}
		return tea.Tick(m.opts.rest, func(time.Time) tea.Msg { return msg })
	}
	return m.read()
}

// Pause stops the reads of the partial log of the job shown while the
// parent is hidden, and keeps what shows. Showing the job again reads on.
func (m *Model) Pause() {
	if m.unwatch == nil {
		return
	}
	m.unwatch()
	m.unwatch = nil
	// The poll reads the log anew, in generations of its own, which
	// replace what shows.
	m.part.gen = -1
}

// partialMsg carries the partial log of a job in progress.
type partialMsg struct {
	id    int64
	jobID int64
	log   core.PartialLog
	err   error
}

// readPartial reads what GitHub publishes of the log of the job shown.
func (m *Model) readPartial() tea.Cmd {
	svc, ctx, id, repo, jobID := m.svc, m.ctx, m.id, m.repo, m.job.ID
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "actions.partial_log")
		l, err := svc.PartialLog(ctx, repo, jobID)
		logged := err
		if errors.Is(err, core.ErrLogPending) {
			// A log GitHub publishes nothing of yet is no failure.
			logged = nil
		}
		end(logged, "span", "tui", "job", jobID, "lines", len(l.Lines), "pending", err != nil && logged == nil)
		return partialMsg{id: id, jobID: jobID, log: l, err: err}
	}
}

// receivePartial shows a partial log read. One that failed to read leaves
// the view as it is, for the poll to read again.
func (m *Model) receivePartial(msg partialMsg) {
	if msg.jobID != m.job.ID || m.job.Done() || (m.state != Pending && m.state != Partial) || msg.err != nil {
		return
	}
	defer m.layout()
	m.setPartial(msg.log)
}

// fromPartial shows what the poll read of the log of the job shown, and
// reports whether there was any.
func (m *Model) fromPartial() bool {
	l, ok := m.svc.CachedPartialLog(m.repo, m.job.ID)
	if !ok {
		return false
	}
	m.setPartial(l)
	return true
}

// setPartial shows l, the partial log of the job shown, unless it has no
// lines yet, or it is older than the one shown, as a read that took longer
// than the poll after it is. Lines added to the ones shown are appended,
// which keeps the cursor, the folds and the search; a log that started
// over replaces them.
func (m *Model) setPartial(l core.PartialLog) {
	p, shown := &m.part, m.state == Partial
	if len(l.Lines) == 0 || shown && (l.Gen < p.gen || l.Gen == p.gen && l.At.Before(p.at)) {
		return
	}
	switch {
	case shown && l.Gen == p.gen && len(l.Lines) >= p.lines:
		if len(l.Lines) > p.lines {
			m.view.SetSections(stepSections(l.Lines, m.job.Steps, m.opts.now()))
			m.view.Append(viewLines(l.Lines[p.lines:])...)
		}
	default:
		m.setLines(logLines(l.Log, m.job.Steps, m.opts.now()))
	}
	m.state, m.truncated = Partial, l.Truncated
	m.part = shownPart{gen: l.Gen, lines: len(l.Lines), at: l.At}
	m.view.SetTitle("partial, as of " + ui.Clock(l.At.Local(), m.opts.now()))
}
