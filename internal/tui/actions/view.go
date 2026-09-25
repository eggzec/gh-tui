package actions

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/jobview"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// View renders the panes side by side, or the focused one alone with a
// breadcrumb on a narrow terminal or zoomed, in exactly the size of the last
// SetSize. The filter step takes the whole modal, and a confirmation the
// last line.
func (m *Modal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	w, h := m.width, m.bodyHeight()
	var lines []string
	switch {
	case m.filterStep != nil:
		lines = append(lines, ui.Fit(m.st.lastCrumb.Render("Filter runs"), w))
		lines = append(lines, m.filterLines(w, h)...)
	case m.narrow():
		lines = append(lines, ui.Fit(m.breadcrumb(w), w))
		lines = append(lines, m.paneLines(m.focus, w, h)...)
	case m.zoom:
		lines = append(lines, m.paneTitle(m.focus, w))
		lines = append(lines, m.paneLines(m.focus, w, h)...)
	default:
		lines = m.columns(h)
	}
	if p := m.prompt(w); p != "" {
		lines[len(lines)-1] = p
	}
	return strings.Join(lines, "\n")
}

// columns renders the three panes side by side, with their titles.
func (m *Modal) columns(h int) []string {
	rw, jw, lw := m.widths()
	cols := [numPanes][]string{
		append([]string{m.paneTitle(runsPane, rw)}, m.paneLines(runsPane, rw, h)...),
		append([]string{m.paneTitle(jobsPane, jw)}, m.paneLines(jobsPane, jw, h)...),
		append([]string{m.paneTitle(logPane, lw)}, m.paneLines(logPane, lw, h)...),
	}
	lines := make([]string, m.height)
	var b strings.Builder
	for i := range lines {
		b.Reset()
		b.Grow(m.width + 128)
		b.WriteString(cols[0][i])
		b.WriteString(m.st.sep)
		b.WriteString(cols[1][i])
		b.WriteString(m.st.sep)
		b.WriteString(cols[2][i])
		lines[i] = b.String()
	}
	return lines
}

// bodyHeight is the height of a pane below its title, or its breadcrumb.
func (m *Modal) bodyHeight() int {
	return max(m.height-1, 0)
}

// widths returns the widths of the runs, jobs and log panes. On a narrow
// terminal, or zoomed, each takes the whole width, one at a time.
func (m *Modal) widths() (runs, jobs, log int) {
	if m.narrow() || m.zoom {
		return m.width, m.width, m.width
	}
	runs = min(max(m.width*3/10, 34), 52)
	jobs = min(max(m.width*22/100, 24), 36)
	return runs, jobs, max(m.width-runs-jobs-2*sepWidth, 0)
}

func (m *Modal) paneWidth(p pane) int {
	r, j, l := m.widths()
	return [numPanes]int{r, j, l}[p]
}

// layout sizes the bubbles to their panes.
func (m *Modal) layout() {
	h := m.bodyHeight()
	m.runs.SetSize(m.paneWidth(runsPane), h)
	m.log.SetSize(m.paneWidth(logPane), h)
	if f := m.filterStep; f != nil && f.form != nil {
		f.form.SetSize(m.width, h)
	}
	m.scrollJobs()
}

// paneLines renders the body of pane p, h lines of w cells.
func (m *Modal) paneLines(p pane, w, h int) []string {
	switch p {
	case runsPane:
		// The feed renders its size exactly.
		return ui.PadLines(strings.Split(m.runs.View(), "\n"), w, h)
	case jobsPane:
		return m.jobLines(w, h)
	case logPane:
	}
	return m.logBody(w, h)
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
			detail = strings.TrimSpace(detail + " · " + f)
		}
	case jobsPane:
		text, detail = "Jobs", m.jobsTitle()
	case logPane:
		text, detail = "Log", m.logTitle()
	}
	line := st.Render(text)
	if room := w - ansi.StringWidth(text) - 1; detail != "" && room > 1 {
		line += " " + m.st.Subtle.Render(ansi.Truncate(detail, room, "…"))
	}
	return ui.Fit(line, w)
}

// breadcrumb shows where the narrow modal is: the runs, the run and the
// job, as far as the focus went. The runs say which tab they are, as the
// frame is too narrow to show the tabs.
func (m *Modal) breadcrumb(w int) string {
	crumbs := []string{m.runsCrumb()}
	if m.focus >= jobsPane && m.hasRun {
		crumbs = append(crumbs, jobview.RunName(m.run))
	}
	if j, ok := m.jobs.selected(); m.focus == logPane && ok {
		crumbs = append(crumbs, ui.OneLine(j.Name))
	}
	const sep = " › "
	// Drop the first crumbs until the rest fit.
	for len(crumbs) > 1 && ansi.StringWidth(strings.Join(crumbs, sep)) > w {
		crumbs = crumbs[1:]
		if crumbs[0] != "…" {
			crumbs = append([]string{"…"}, crumbs[1:]...)
		}
	}
	var b strings.Builder
	for i, c := range crumbs {
		if i > 0 {
			b.WriteString(m.st.Subtle.Render(sep))
		}
		st := m.st.crumb
		if i == len(crumbs)-1 {
			st = m.st.lastCrumb
		}
		b.WriteString(st.Render(c))
	}
	return ansi.Truncate(b.String(), w, "…")
}

// runsCrumb names the runs shown: their tab, unless it is All, or that a
// filter narrows them.
func (m *Modal) runsCrumb() string {
	t, ok := tabOf(m.filter)
	switch {
	case m.filterText() != "":
		return "Runs · filtered"
	case ok && t != tabAll:
		return "Runs · " + tabNames[t]
	}
	return "Runs"
}

// prompt renders the confirmation or the notice on the last line, if
// there is one.
func (m *Modal) prompt(w int) string {
	switch {
	case m.ask != nil:
		yes, no := m.keys.Yes.Help().Key, m.keys.No.Help().Key
		return ui.Spread(m.st.question.Render(m.ask.question), m.st.Muted.Render(yes+"/"+no), w)
	case m.notice != "":
		return ui.Fit(m.st.Warning.Render(ansi.Truncate(m.notice, w, "…")), w)
	}
	return ""
}
