package actions

import (
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/jobview"
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
	lines := m.framed()
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
	aw, ah := m.area()
	if b, ok := paneLayout(aw, ah, paneRuns, m.zoom).Box(paneRuns, panes.Whole); ok {
		in := b.Rect.Inner()
		body := max(in.H-1, 0)
		m.inner = in.W
		m.rows[runsPane] = body
		m.rows[jobsPane] = min(m.jobsWant(), max(body-1, 0))
	}
	m.sizePanes()
	// A footer under the runs, a prompt or a chip, takes a row of the feed;
	// the run the jobs hang from must keep one.
	for m.rowsOf(jobsPane) > 0 && m.runs.SelectedRow() < 0 {
		m.rows[jobsPane]--
		m.sizePanes()
	}
	m.scrollJobs()
}

// fit sizes the tree again when the rows the jobs take changed, as they do
// when the jobs load, a run folds or a group opens. Other moves leave the
// layout alone.
func (m *Modal) fit() {
	if min(m.jobsWant(), max(m.rowsOf(runsPane)-1, 0)) != m.rowsOf(jobsPane) ||
		m.rowsOf(jobsPane) > 0 && m.runs.SelectedRow() < 0 {
		m.layout()
	}
	// Jobs that have no room can't hold the focus.
	if m.focus == jobsPane && m.hasRun && m.rowsOf(jobsPane) == 0 {
		m.setFocus(runsPane)
	}
}

// jobsWant is how many rows the jobs of the run under the cursor would take
// under its row: none while there is no run or its jobs are folded away.
func (m *Modal) jobsWant() int {
	if !m.hasRun || m.shut[m.run.ID] {
		return 0
	}
	if note, ok := m.jobsNote(m.inner); ok {
		return len(note)
	}
	n := m.linesSpan(0, len(m.jobs.lines))
	if m.jobs.more {
		n++
	}
	return n
}

// sizePanes sizes the runs to what the jobs leave of the runs box.
func (m *Modal) sizePanes() {
	m.runs.SetSize(m.inner, max(m.rowsOf(runsPane)-m.rowsOf(jobsPane), 0))
}

// runsBox renders the inside of the runs box, h lines of w cells: its title,
// then the runs in one list, with the jobs of the run under the cursor in
// the lines under its row.
func (m *Modal) runsBox(w, h int) []string {
	lines := []string{m.paneTitle(runsPane, w)}
	var runs []string
	if m.runs.Height() > 0 {
		runs = strings.Split(m.runs.View(), "\n")
	}
	k := m.rowsOf(jobsPane)
	if k <= 0 {
		return ui.PadLines(append(lines, runs...), w, h)
	}
	at := m.runs.SelectedRow()
	if at < 0 {
		at = len(runs) - 1
	}
	lines = append(lines, runs[:at+1]...)
	lines = append(lines, m.jobLines(w, k)...)
	return ui.PadLines(append(lines, runs[at+1:]...), w, h)
}

// notesLines renders the annotations box: the facts of the run, and what
// to do to see annotations.
func (m *Modal) notesLines(w, h int) []string {
	st := &m.st
	if !m.hasRun {
		if err := m.runs.Err(); err != nil && m.runs.Len() == 0 {
			// The runs box has room for only a cut of the error.
			return ui.FitLines(m.errorLines("load the runs", m.repo.String(), err, w), w, h)
		}
		return ui.FitLines([]string{st.Muted.Render("Pick a run to see its details.")}, w, h)
	}
	facts := m.runFacts(w)
	facts, rows := noteSplit(facts, len(m.noteEntries()), len(m.noteNotice(w)), h)
	if len(facts) > 0 {
		facts = append(facts, "")
	}
	return ui.FitLines(append(facts, m.noteLines(w, h-len(facts), rows)...), w, h)
}

// runFacts are the lines that tell of the run shown.
func (m *Modal) runFacts(w int) []string {
	st := &m.st
	r := m.run
	now := m.now()
	var lines []string
	ic := st.ic
	add := func(glyph, name, value string) {
		if value = ui.OneLine(value); value != "" {
			lines = append(lines, glyph+" "+st.Subtle.Render(name+" ")+value)
		}
	}
	sub := func(g string) string { return st.Subtle.Render(g) }
	add(sub(ic.Workflow), "Workflow", r.Name)
	add(sub(ic.Comment), "Title", r.DisplayTitle)
	add(st.Glyphs[ui.RunStateOf(r.Status, "")], "Status", ui.StatusText(r.Status))
	if r.Done() {
		add(st.Glyphs[ui.RunStateOf(r.Status, r.Conclusion)], "Result", string(r.Conclusion))
	}
	add(sub(ic.Event), "Event", r.Event)
	add(sub(ic.Glyph(ui.GlyphBranchEvent)), "Branch", r.Branch)
	add(sub(ic.Glyph(ui.GlyphCommit)), "Commit", r.HeadSHA[:min(len(r.HeadSHA), 7)])
	add(sub(ic.Person), "Actor", r.Actor)
	if !r.CreatedAt.IsZero() {
		add(sub(ic.Clock), "Started", m.opts.dates.Short(r.CreatedAt, now))
	}
	add(sub(ic.Clock), "Duration", m.jobsSpan(now))
	if r.Attempt > 1 {
		add(sub(ic.Changed), "Attempt", strconv.Itoa(r.Attempt))
	}
	if m.jobs.err != nil && !m.jobs.loaded {
		// The tree has room for only a cut of the error; it reads whole here.
		lines = append(lines, "")
		lines = append(lines, m.errorLines("load the jobs", jobview.RunName(r), m.jobs.err, w)...)
	}
	return lines
}

// jobsSpan is how long the run took, from the earliest start of its jobs to
// the latest end, or to now while one still runs: "-" until the jobs are
// loaded and have times.
func (m *Modal) jobsSpan(now time.Time) string {
	var start, end time.Time
	running := false
	for i := range m.jobs.items {
		j := &m.jobs.items[i]
		if j.StartedAt.IsZero() {
			continue
		}
		if start.IsZero() || j.StartedAt.Before(start) {
			start = j.StartedAt
		}
		if j.CompletedAt.IsZero() {
			running = true
		} else if j.CompletedAt.After(end) {
			end = j.CompletedAt
		}
	}
	if running {
		end = time.Time{}
	}
	d, ok := ui.Span(start, end, now)
	if !ok {
		return "-"
	}
	return ui.Duration(d)
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
	case m.workflows.wanted && m.workflows.loading:
		return ui.Fit(m.spin.View()+m.st.Muted.Render("Loading the workflows"+m.st.ic.Ellipsis), w)
	}
	return ""
}
