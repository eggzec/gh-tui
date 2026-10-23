package actions

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// View renders the panes in their frames, in exactly the size of the last
// SetSize: the boxes, the strip of the pane names while one pane is zoomed,
// and the keys of the modal on the last line. The filter step takes the
// whole modal, and a confirmation the last line.
func (m *Modal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	w := m.width
	var lines []string
	if m.filterStep != nil {
		lines = append(lines, ui.Fit(m.st.lastCrumb.Render("Filter"+m.st.ic.Separator+"Runs"), w))
		lines = append(lines, m.filterLines(w, m.bodyHeight())...)
	} else {
		lines = m.framed()
	}
	if p := m.prompt(w); p != "" {
		lines[len(lines)-1] = p
	}
	return strings.Join(lines, "\n")
}

// framed renders the boxes, the strip and the footer: m.height lines.
func (m *Modal) framed() []string {
	aw, ah := m.area()
	l := paneLayout(aw, ah, m.box(), m.zoom)
	out := m.frames.Draw(l, func(b panes.Box) panes.Content {
		c := panes.Content{Focus: b.Pane == l.Focus}
		title := l.Title(b.Pane)
		if l.Zoomed {
			// The digits are in the strip under the boxes.
			title = paneName(b.Pane)
		}
		c.TopLeft = m.titleForms(title, b)
		inner := b.Rect.Inner()
		if b.Pane == paneNotes {
			c.Body = strings.Join(m.notesLines(inner.W, inner.H), "\n")
		} else {
			c.Body = strings.Join(m.runsBox(inner.W, inner.H), "\n")
		}
		return c
	})
	lines := ui.PadLines(strings.Split(out, "\n"), aw, ah)
	if m.hasStrip() {
		lines = append(lines, m.strip())
	}
	return append(lines, m.footer(l))
}

// titleForms are the forms of a title for box b: the whole title, then the
// title cut to the room the border leaves it. The bubble never cuts a form
// itself.
func (m *Modal) titleForms(title string, b panes.Box) []string {
	if title == "" {
		return nil
	}
	room := b.Rect.W - 5
	if room <= 0 || ansi.StringWidth(title) <= room {
		return []string{title}
	}
	return []string{title, ansi.Truncate(title, room, m.st.ic.Ellipsis)}
}

// zoomed reports whether one pane fills the area.
func (m *Modal) zoomed() bool { return showsOne(m.width, m.zoom) }

// canZoom reports whether the zoom key changes anything: under wideMin the
// focused pane is alone anyway.
func (m *Modal) canZoom() bool { return m.width >= wideMin }

// hasStrip reports whether the row of pane names shows under the boxes.
func (m *Modal) hasStrip() bool { return m.zoomed() && m.height >= 4 }

// box is the pane of the focus: the runs box, or the annotations.
func (m *Modal) box() panes.Pane {
	if m.notes {
		return paneNotes
	}
	return paneRuns
}

// area is the size of the boxes: the modal less the footer and, while one
// pane is zoomed, the strip.
func (m *Modal) area() (w, h int) {
	h = max(m.height-1, 0)
	if m.hasStrip() {
		h--
	}
	return m.width, h
}

// strip is the row of pane names under the zoomed boxes, in the longest form
// that fits the width, and blank when none does.
func (m *Modal) strip() string {
	dot := strings.TrimSpace(m.st.ic.Separator)
	for _, f := range m.frames.StripForms(paneDefs, m.box()) {
		f = strings.ReplaceAll(f, "·", dot)
		if ansi.StringWidth(f) <= m.width {
			return ui.Fit(f, m.width)
		}
	}
	return ui.Fit("", m.width)
}

// footer is the last line: the keys that move between the panes.
func (m *Modal) footer(l panes.Layout) string {
	k := m.keys
	hint := func(b key.Binding, what string) string {
		if !b.Enabled() {
			return ""
		}
		return m.theme.Muted.Render(b.Help().Key) + " " + m.theme.Subtle.Render(what)
	}
	parts := []string{hint(k.Next, "focus"), hint(k.Jump, "pane")}
	if m.canZoom() {
		what := "zoom"
		if l.Zoomed {
			what = "unzoom"
		}
		parts = append(parts, hint(k.Zoom, what))
	}
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return ui.Fit(strings.Join(kept, m.theme.Subtle.Render(m.st.ic.Separator)), m.width)
}

// bodyHeight is the height of the filter form, below its breadcrumb.
func (m *Modal) bodyHeight() int {
	return max(m.height-1, 0)
}

// paneWidth is the width of the runs and the jobs: the inside of the runs
// box.
func (m *Modal) paneWidth(pane) int { return m.inner }

// rowsOf is the height of pane p below its title, or -1 while it is not
// shown.
func (m *Modal) rowsOf(p pane) int { return m.rows[p] }

// layout sizes the bubbles to their panes, and keeps the row in focus of
// the jobs in view.
func (m *Modal) layout() {
	if f := m.filterStep; f != nil && f.form != nil {
		// The form is all that shows, in less room than the panes keep
		// their size for.
		f.form.SetSize(m.width, m.bodyHeight())
		return
	}
	aw, ah := m.area()
	if b, ok := paneLayout(aw, ah, paneRuns, m.zoom).Box(paneRuns, panes.Whole); ok {
		in := b.Rect.Inner()
		m.inner, m.rows = in.W, m.split(in.H)
	}
	m.sizePanes()
	m.scrollJobs()
}

// split divides h lines between the runs and the jobs, each with a title
// line of its own: the focused one gets the larger part. Where there is no
// room for both, only the focused one shows.
func (m *Modal) split(h int) [numPanes]int {
	const both = 8
	rows := [numPanes]int{-1, -1}
	if h < both {
		rows[m.focus] = max(h-1, 0)
		return rows
	}
	jobs := h / 3
	if m.focus == jobsPane {
		jobs = h - h/3
	}
	rows[jobsPane], rows[runsPane] = jobs-1, h-jobs-1
	return rows
}

// sizePanes sizes the runs to their pane.
func (m *Modal) sizePanes() {
	m.runs.SetSize(m.inner, max(m.rowsOf(runsPane)-1, 0))
}

// runsBox renders the inside of the runs box, h lines of w cells: the runs
// and the jobs one above the other, each under its title.
func (m *Modal) runsBox(w, h int) []string {
	var lines []string
	for _, p := range [numPanes]pane{runsPane, jobsPane} {
		rows := m.rowsOf(p)
		if rows < 0 {
			continue
		}
		lines = append(lines, m.paneTitle(p, w))
		lines = append(lines, m.paneLines(p, w, rows)...)
	}
	return ui.PadLines(lines, w, h)
}

// paneLines renders the body of pane p, h lines of w cells.
func (m *Modal) paneLines(p pane, w, h int) []string {
	if p == runsPane {
		// The feed renders its size exactly, under the header's line.
		lines := append([]string{m.runsHeader(w)}, strings.Split(m.runs.View(), "\n")...)
		return ui.PadLines(lines, w, h)
	}
	return m.jobLines(w, h)
}

// notesLines renders the annotations box: the facts of the run, and what
// to do to see annotations.
func (m *Modal) notesLines(w, h int) []string {
	st := &m.st
	if !m.hasRun {
		return ui.FitLines([]string{st.Muted.Render("Pick a run to see its details.")}, w, h)
	}
	r := m.run
	var lines []string
	add := func(name, value string) {
		if value = ui.OneLine(value); value != "" {
			lines = append(lines, st.Subtle.Render(name+" ")+value)
		}
	}
	add("Workflow", r.Name)
	add("Title", r.DisplayTitle)
	add("Status", ui.StatusText(r.Status))
	if r.Done() {
		add("Result", string(r.Conclusion))
	}
	add("Event", r.Event)
	add("Branch", r.Branch)
	add("Commit", r.HeadSHA[:min(len(r.HeadSHA), 7)])
	add("Actor", r.Actor)
	if r.Attempt > 1 {
		add("Attempt", strconv.Itoa(r.Attempt))
	}
	lines = append(lines, "", st.Muted.Render("No annotations here. Those of a failed job show with its log."))
	return ui.FitLines(lines, w, h)
}

// paneTitle renders the first line of section p of the runs box: what it
// shows, in the accent while it has the focus.
func (m *Modal) paneTitle(p pane, w int) string {
	st := m.st.title
	if p == m.focus && !m.notes {
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
