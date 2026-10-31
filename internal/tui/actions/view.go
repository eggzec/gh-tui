package actions

import (
	"net/url"
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
			c.BottomLeft = m.notesCount()
		} else {
			c.Body = strings.Join(m.runsBox(inner.W, inner.H), "\n")
			c.BottomLeft = m.runsCount()
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
	for _, f := range m.frames.StripForms(paneDefs, m.box()) {
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

// layout sizes the bubbles to their panes.
func (m *Modal) layout() {
	aw, ah := m.area()
	if b, ok := paneLayout(aw, ah, paneRuns, m.zoom).Box(paneRuns, panes.Whole); ok {
		in := b.Rect.Inner()
		m.inner = in.W
		m.rows[runsPane] = max(in.H-1, 0)
	}
	m.runs.SetSize(m.inner, m.rowsOf(runsPane))
}

// runsBox renders the inside of the runs box, h lines of w cells: its title,
// then the runs and the jobs under them in one tree.
func (m *Modal) runsBox(w, h int) []string {
	lines := []string{m.paneTitle(runsPane, w)}
	if m.runs.Height() > 0 {
		lines = append(lines, strings.Split(m.runs.View(), "\n")...)
	}
	return ui.PadLines(lines, w, h)
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
	add := func(glyph, name, value string, addr ...string) {
		if value = ui.OneLine(value); value != "" {
			if len(addr) > 0 {
				value = termtext.Link(addr[0], value)
			}
			lines = append(lines, glyph+" "+st.Subtle.Render(name+" ")+value)
		}
	}
	// The run's page names the host and the repository, which the branch,
	// the commit and the actor are pages of.
	var branchURL, commitURL string
	if repoPage, _, ok := strings.Cut(r.URL, "/actions/"); ok {
		segs := strings.Split(r.Branch, "/")
		for i, s := range segs {
			segs[i] = url.PathEscape(s)
		}
		branchURL = repoPage + "/tree/" + strings.Join(segs, "/")
		commitURL = repoPage + "/commit/" + url.PathEscape(r.HeadSHA)
	}
	sub := func(g string) string { return st.Subtle.Render(g) }
	add(sub(ic.Workflow), "Workflow", r.Name)
	add(sub(ic.Comment), "Title", r.DisplayTitle)
	add(st.Glyphs[ui.RunStateOf(r.Status, "")], "Status", ui.StatusText(r.Status))
	if r.Done() {
		add(st.Glyphs[ui.RunStateOf(r.Status, r.Conclusion)], "Result", string(r.Conclusion))
	}
	add(sub(ic.Event), "Event", r.Event)
	add(sub(ic.Glyph(ui.GlyphBranchEvent)), "Branch", r.Branch, branchURL)
	add(sub(ic.Glyph(ui.GlyphCommit)), "Commit", r.HeadSHA[:min(len(r.HeadSHA), 7)], commitURL)
	add(sub(ic.Person), "Actor", r.Actor, ui.PageProfileURL(r.URL, r.Actor))
	if !r.CreatedAt.IsZero() {
		add(sub(ic.Clock), "Started", m.opts.dates.Short(r.CreatedAt, now))
	}
	add(sub(ic.Clock), "Duration", m.jobsSpan(now))
	if r.Attempt > 1 {
		add(sub(ic.Attempt), "Attempt", strconv.Itoa(r.Attempt))
	}
	if m.cur().err != nil && !m.cur().loaded {
		// The tree has room for only a cut of the error; it reads whole here.
		lines = append(lines, "")
		lines = append(lines, m.errorLines("load the jobs", jobview.RunName(r), m.cur().err, w)...)
	}
	return lines
}

// jobsSpan is how long the run took, from the earliest start of its jobs to
// the latest end, or to now while one still runs: "-" until the jobs are
// loaded and have times.
func (m *Modal) jobsSpan(now time.Time) string {
	var start, end time.Time
	running := false
	items := m.cur().items
	for i := range items {
		j := &items[i]
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
	spec := ui.TitleSpec{
		Width: w, Sep: m.st.ic.Separator, Ellipsis: m.st.ic.Ellipsis,
		NameStyle: st.Render, ChipStyle: m.st.Subtle.Render, SepStyle: m.st.Subtle.Render}
	if p == runsPane {
		spec.Name = "Runs"
		if f := m.filterText(); f != "" {
			spec.Chips = append(spec.Chips, f)
		}
		if kept, off := m.toggle.Kept(); off {
			spec.Chips = append(spec.Chips, ui.QueryChips(m.st.ic, ui.ChipSpec{Filter: ansi.Strip(queryOf(kept, m.workflows.items)), SortOn: true})...)
		}
	}
	return ui.Title(spec)
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
