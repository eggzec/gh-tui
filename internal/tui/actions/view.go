package actions

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// View renders the panes side by side, in exactly the size of the last
// SetSize. The filter step takes the whole modal, and a confirmation the
// last line.
func (m *Modal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	w, h := m.width, m.bodyHeight()
	var lines []string
	if m.filterStep != nil {
		lines = append(lines, ui.Fit(m.st.lastCrumb.Render("Filter"+m.st.ic.Separator+"Runs"), w))
		lines = append(lines, m.filterLines(w, h)...)
	} else {
		lines = m.columns(h)
	}
	if p := m.prompt(w); p != "" {
		lines[len(lines)-1] = p
	}
	return strings.Join(lines, "\n")
}

// columns renders the panes side by side, with their titles.
func (m *Modal) columns(h int) []string {
	rw, jw := m.widths()
	cols := [numPanes][]string{
		append([]string{m.paneTitle(runsPane, rw)}, m.paneLines(runsPane, rw, h)...),
		append([]string{m.paneTitle(jobsPane, jw)}, m.paneLines(jobsPane, jw, h)...),
	}
	lines := make([]string, m.height)
	var b strings.Builder
	for i := range lines {
		b.Reset()
		b.Grow(m.width + 128)
		b.WriteString(cols[0][i])
		b.WriteString(m.st.sep)
		b.WriteString(cols[1][i])
		// Below the width of the runs there is no room for the jobs.
		lines[i] = ui.Fit(b.String(), m.width)
	}
	return lines
}

// bodyHeight is the height of a pane below its title, or its breadcrumb.
func (m *Modal) bodyHeight() int {
	return max(m.height-1, 0)
}

// widths returns the widths of the runs and jobs panes: the runs take
// about half, and the jobs the rest.
func (m *Modal) widths() (runs, jobs int) {
	runs = min(max(m.width/2, 34), m.width)
	return runs, max(m.width-runs-sepWidth, 0)
}

func (m *Modal) paneWidth(p pane) int {
	r, j := m.widths()
	return [numPanes]int{r, j}[p]
}

// layout sizes the bubbles to their panes, and keeps the row in focus of
// the jobs in view.
func (m *Modal) layout() {
	if f := m.filterStep; f != nil && f.form != nil {
		// The form is all that shows, in less room than the panes keep
		// their size for.
		f.form.SetSize(m.width, m.bodyHeight())
		return
	}
	m.sizePanes()
	m.scrollJobs()
}

// sizePanes sizes the runs to their pane.
func (m *Modal) sizePanes() {
	m.runs.SetSize(m.paneWidth(runsPane), max(m.bodyHeight()-1, 0))
}

// paneLines renders the body of pane p, h lines of w cells.
func (m *Modal) paneLines(p pane, w, h int) []string {
	if p == runsPane {
		// The feed renders its size exactly, under the header's line.
		lines := append([]string{m.runsHeader(m.paneWidth(runsPane))}, strings.Split(m.runs.View(), "\n")...)
		return ui.PadLines(lines, w, h)
	}
	return m.jobLines(w, h)
}

// paneTitle renders the first line of pane p: what it shows, in the accent
// while it has the focus.
func (m *Modal) paneTitle(p pane, w int) string {
	st := m.st.title
	if p == m.focus {
		st = m.st.focusTitle
	}
	var text, detail string
	switch p {
	case runsPane:
		text, detail = "Runs", m.runsTitle()
		if f := m.filterText(); f != "" {
			detail = strings.TrimSpace(detail + m.st.ic.Separator + f)
		}
		if kept, off := m.toggle.Kept(); off {
			detail = strings.TrimSpace(detail + m.st.ic.Separator + ui.OffChip(ansi.Strip(queryOf(kept, m.workflows.items)), m.st.ic.Separator))
		}
	case jobsPane:
		text, detail = "Jobs", m.jobsTitle()
	}
	line := st.Render(text)
	if room := w - ansi.StringWidth(text) - 1; detail != "" && room > 1 {
		line += " " + m.st.Subtle.Render(termtext.Truncate(detail, room, m.st.ic.Ellipsis))
	}
	return ui.Fit(line, w)
}

// prompt renders the confirmation or the notice on the last line, if
// there is one.
func (m *Modal) prompt(w int) string {
	switch {
	case m.ask != nil:
		return m.ask.Line(m.st.confirm, m.keys.Confirm, w)
	case m.notice != "":
		return ui.Fit(m.st.Warning.Render(termtext.Truncate(m.notice, w, m.st.ic.Ellipsis)), w)
	}
	return ""
}
