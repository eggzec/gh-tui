package actions

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// showJob shows the log of the job under the cursor of the jobs: from
// memory at once, or read, once the cursor rests if rest is set.
func (m *Modal) showJob(rest bool) tea.Cmd {
	defer m.layout()
	j, ok := m.jobs.selected()
	if !ok {
		m.log.Clear()
		return nil
	}
	return m.log.Show(j, rest, jobview.Hints{SHA: m.run.HeadSHA})
}

// logBody renders the log pane's body, h lines of w cells.
func (m *Modal) logBody(w, h int) []string {
	if m.log.State() == jobview.None {
		text := "Pick a job to see its log."
		if m.hasRun && m.jobs.loaded && len(m.jobs.items) == 0 {
			text = "No jobs, so no log."
		}
		return ui.FitLines([]string{m.st.Muted.Render(text)}, w, h)
	}
	return ui.PadLines(strings.Split(m.log.View(), "\n"), w, h)
}

// logTitle names the job of the log pane.
func (m *Modal) logTitle() string {
	if _, ok := m.jobs.selected(); !ok {
		return ""
	}
	return m.log.Title()
}
