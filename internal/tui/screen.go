package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// screen is what fills the space between the header and the help line.
type screen int

const (
	// repoScreen shows the panes of the selected repository.
	repoScreen screen = iota
	// notifScreen shows the notifications.
	notifScreen
)

// Sizes of the repository screen. Below narrowWidth columns the panes
// don't fit side by side, so the screen shows the focused pane alone, at
// full size, and the pane keys switch between them. Above it, the files
// take two fifths of the width, within the bounds.
const (
	narrowWidth   = 70
	minFilesWidth = 24
	maxFilesWidth = 60
)

// pane is a section in a frame whose top edge carries its label.
type pane struct {
	section ui.Section
	// label is shown in the top edge, such as "[1] Files".
	label   string
	started bool
	focused bool
	// width and height are the outer size, frame included.
	width, height int
	// The edges of the frame, drawn for the size, focus and theme.
	top, bottom, side string
}

func paneLabel(i int, title string) string {
	return "[" + strconv.Itoa(i+1) + "] " + title
}

// start initializes the section the first time it is shown.
func (p *pane) start() tea.Cmd {
	if p == nil || p.started {
		return nil
	}
	p.started = true
	return p.section.Init()
}

func (p *pane) setFocus(focused bool) {
	p.focused = focused
	if focused {
		p.section.Focus()
	} else {
		p.section.Blur()
	}
}

// resize sets the outer size, and gives the section the room inside the
// frame.
func (p *pane) resize(width, height int) {
	p.width, p.height = max(width, 0), max(height, 0)
	p.section.SetSize(max(p.width-2, 0), max(p.height-2, 0))
}

// appendLines appends the lines of the framed section to dst. A section
// that draws less than its size is padded, so the panes beside it stay in
// place.
func (p *pane) appendLines(dst []string) []string {
	if p.width <= 0 || p.height <= 0 {
		return dst
	}
	if p.top == "" {
		blank := strings.Repeat(" ", p.width)
		for range p.height {
			dst = append(dst, blank)
		}
		return dst
	}
	dst = append(dst, p.top)
	body, inner := p.section.View(), p.width-2
	for range p.height - 2 {
		var line string
		line, body, _ = strings.Cut(body, "\n")
		dst = append(dst, p.side+fit(line, inner)+p.side)
	}
	return append(dst, p.bottom)
}

// fit pads or cuts s to width cells.
func fit(s string, width int) string {
	w := ansi.StringWidth(s)
	switch {
	case w == width:
		return s
	case w < width:
		return s + strings.Repeat(" ", width-w)
	}
	return ansi.Truncate(s, width, "")
}

// styles are the root's own, built once per theme.
type styles struct {
	edge, focusEdge   lipgloss.Style
	title, focusTitle lipgloss.Style
	repo, branch      lipgloss.Style
	dot, badge        lipgloss.Style
}

func newStyles(t ui.Theme) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	return styles{
		edge:       border,
		focusEdge:  t.Accent,
		title:      t.Muted,
		focusTitle: t.Accent.Bold(true),
		repo:       t.Title,
		branch:     t.Muted,
		dot:        t.Accent,
		badge:      t.Muted,
	}
}

// drawFrame draws the edges of p's frame: the accent while it is focused,
// the border color otherwise.
func (m *Model) drawFrame(p *pane) {
	w := p.width
	if w < 2 || p.height < 2 {
		p.top, p.bottom, p.side = "", "", ""
		return
	}
	edge, title := m.st.edge, m.st.title
	if p.focused {
		edge, title = m.st.focusEdge, m.st.focusTitle
	}
	b := lipgloss.RoundedBorder()
	label := ansi.Truncate(p.label, max(w-4, 0), "…")
	if label == "" {
		p.top = edge.Render(b.TopLeft + strings.Repeat(b.Top, w-2) + b.TopRight)
	} else {
		rest := max(w-4-ansi.StringWidth(label), 0)
		p.top = edge.Render(b.TopLeft+b.Top) + title.Render(label) +
			edge.Render(" "+strings.Repeat(b.Top, rest)+b.TopRight)
	}
	p.bottom = edge.Render(b.BottomLeft + strings.Repeat(b.Bottom, w-2) + b.BottomRight)
	p.side = edge.Render(b.Left)
}

func (m *Model) drawFrames() {
	for _, p := range m.all {
		m.drawFrame(p)
	}
}

// drawHeader draws the header: the repository and its default branch on
// the left, and the unread notifications on the right, on a rule.
func (m *Model) drawHeader() {
	w := m.width
	if w <= 0 {
		m.header = ""
		return
	}
	rule := func(n int) string { return m.st.edge.Render(strings.Repeat("─", max(n, 0))) }
	name := "No repository · press / to search"
	if m.repo.Owner != "" {
		name = m.repo.String()
	}
	left := m.st.repo.Render(name)
	if m.branch != "" {
		left += " " + rule(1) + " " + m.st.branch.Render(m.branch)
	}
	var right string
	if m.badge != "" {
		noun := " notifications"
		if m.badge == "1" {
			noun = " notification"
		}
		right = m.st.dot.Render("●") + " " + m.st.badge.Render(m.badge+noun)
	}
	// Two cells of rule and a space on each side frame the text.
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	if right != "" && lw+rw+8 > w {
		right, rw = "", 0
	}
	if right == "" {
		if lw+3 > w {
			m.header = fit(rule(1)+" "+ansi.Truncate(left, max(w-3, 0), "…"), w)
			return
		}
		m.header = rule(1) + " " + left + " " + rule(w-lw-3)
		return
	}
	m.header = rule(1) + " " + left + " " + rule(w-lw-rw-6) + " " + right + " " + rule(1)
}

// narrow reports whether the repository screen shows one pane at a time.
func (m *Model) narrow() bool { return m.width < narrowWidth }

func filesWidth(width int) int {
	return min(max(width*2/5, minFilesWidth), maxFilesWidth)
}

// arrange sizes the panes for the terminal.
func (m *Model) arrange(height int) {
	if m.notif != nil {
		m.notif.resize(m.width, height)
	}
	if m.narrow() {
		for _, p := range m.panes {
			p.resize(m.width, height)
		}
		return
	}
	left, right := m.panes[:m.left], m.panes[m.left:]
	lw := 0
	switch {
	case len(right) == 0:
		lw = m.width
	case len(left) > 0:
		lw = filesWidth(m.width)
	}
	stack(left, lw, height)
	stack(right, m.width-lw, height)
}

// stack shares height between ps evenly, from the top.
func stack(ps []*pane, width, height int) {
	n := len(ps)
	for i, p := range ps {
		h := height / n
		if i < height%n {
			h++
		}
		p.resize(width, h)
	}
}

// body returns the lines of the screen on view.
func (m *Model) body() []string {
	if m.screen == notifScreen {
		return m.notif.appendLines(nil)
	}
	if len(m.panes) == 0 {
		return nil
	}
	if m.narrow() {
		return m.panes[m.focus].appendLines(nil)
	}
	h := m.contentHeight()
	left := make([]string, 0, h)
	for _, p := range m.panes[:m.left] {
		left = p.appendLines(left)
	}
	right := make([]string, 0, h)
	for _, p := range m.panes[m.left:] {
		right = p.appendLines(right)
	}
	if len(left) == 0 {
		return right
	}
	for i := range min(len(left), len(right)) {
		left[i] += right[i]
	}
	return left
}

// focused returns the focused pane of the screen on view, or nil.
func (m *Model) focused() *pane {
	if m.screen == notifScreen {
		return m.notif
	}
	if len(m.panes) == 0 {
		return nil
	}
	return m.panes[m.focus]
}

// showScreen shows screen s, with pane i focused on the repository screen,
// and starts the sections that come into view.
func (m *Model) showScreen(s screen, i int) tea.Cmd {
	if s == notifScreen && m.notif == nil || s == repoScreen && len(m.panes) == 0 {
		return nil
	}
	before := m.focused()
	m.screen = s
	if s == repoScreen {
		m.focus = min(max(i, 0), len(m.panes)-1)
	}
	if after := m.focused(); after != before {
		if before != nil {
			before.setFocus(false)
			m.drawFrame(before)
		}
		after.setFocus(true)
		m.drawFrame(after)
	}
	return m.startScreen()
}

// cycle moves the focus by delta panes on the repository screen.
func (m *Model) cycle(delta int) tea.Cmd {
	if m.screen != repoScreen || len(m.panes) == 0 {
		return nil
	}
	n := len(m.panes)
	return m.showScreen(repoScreen, ((m.focus+delta)%n+n)%n)
}

// toggleScreen switches between the repository screen and the
// notifications.
func (m *Model) toggleScreen() tea.Cmd {
	if m.screen == notifScreen {
		return m.showScreen(repoScreen, m.focus)
	}
	return m.showScreen(notifScreen, m.focus)
}
