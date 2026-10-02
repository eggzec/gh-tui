package jobview

import (
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// View renders the job in exactly the size of the last SetSize: its log
// under a notice when only the end of it was read, or its steps under why
// there is no log to show.
func (m Model) View() string {
	return strings.Join(m.lines(), "\n")
}

func (m Model) lines() []string {
	w, h := m.width, m.height
	if m.state == None {
		return ui.FitLines([]string{m.st.Muted.Render("Pick a job to see its log.")}, w, h)
	}
	var lines []string
	if n := m.notice(); n != "" {
		lines = ui.Wrap(n, w)[:1]
	}
	lines = append(lines, m.noteLines(w)...)
	switch m.state {
	case Pending, Expired:
		lines = append(lines, m.stepLines(w, h-len(lines))...)
	default:
		lines = append(lines, strings.Split(m.view.View(), "\n")...)
	}
	return ui.PadLines(lines, w, h)
}

// notice is the line above the log, if any: that only its end was read.
func (m Model) notice() string {
	if (m.state != Ready && m.state != Partial) || !m.truncated {
		return ""
	}
	text := "Only the end of this log: it is too large to read whole."
	if k := m.keys.Open.Help().Key; k != "" {
		text += " " + k + " opens it on GitHub."
	}
	return m.st.Warning.Render(text)
}

// stepLines renders the steps of the job, for a job whose log isn't there
// to read, after why.
func (m Model) stepLines(w, h int) []string {
	st, j := &m.st, &m.job
	var why string
	switch {
	case m.state == Expired:
		why = "GitHub no longer keeps this log."
	case j.Status == core.RunInProgress:
		why = "Logs appear when the job finishes."
		// Only GitHub shows the lines of a job as it writes them.
		if k := m.keys.Open; k.Enabled() && k.Help().Key != "" {
			ic := m.opts.icons
			why = "Logs appear when the job finishes" + ic.Separator + ic.Key(k.Help().Key) + " to watch live on GitHub"
		}
	default:
		why = "The job hasn't started yet."
	}
	lines := ui.Wrap(st.Muted.Render(why), w)
	if len(j.Steps) == 0 {
		return ui.FitLines(lines, w, h)
	}
	lines = append(lines, strings.Repeat(" ", w))
	now := m.opts.now()
	for _, s := range j.Steps {
		if len(lines) >= h {
			break
		}
		status := s.Status
		if status == core.RunPending {
			// A step is pending until the steps before it end, which is
			// no wait of the kind a pending job's is.
			status = core.RunQueued
		}
		state := ui.RunStateOf(status, s.Conclusion)
		lines = append(lines, ui.Spread(st.Glyphs[state]+" "+st.Text.Render(ui.OneLine(s.Name)), st.Took(status, s.Conclusion, s.StartedAt, s.CompletedAt, now), w, m.opts.icons.Ellipsis))
	}
	return ui.FitLines(lines, w, h)
}
