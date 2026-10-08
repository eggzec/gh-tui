package tui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// screen is what fills the space between the header and the status bar.
type screen int

const (
	// repoScreen shows the panes of the selected repository.
	repoScreen screen = iota
	// notifScreen shows the notifications.
	notifScreen
	// dashScreen shows the dashboard.
	dashScreen
	// searchScreen shows the search page.
	searchScreen
	// ownerScreen shows the page of a user or an organization.
	ownerScreen
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

// pane is a section in a frame whose top edge carries its label, or a
// bare one, which fills its place and draws its own frames.
type pane struct {
	section ui.Section
	// label is shown in the top edge, such as "[1] Files".
	label   string
	bare    bool
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
	if p.bare {
		p.section.SetSize(p.width, p.height)
		return
	}
	p.section.SetSize(max(p.width-2, 0), max(p.height-2, 0))
}

// appendLines appends the lines of the framed section to dst. A section
// that draws less than its size is padded, so the panes beside it stay in
// place.
func (p *pane) appendLines(dst []string) []string {
	if p.width <= 0 || p.height <= 0 {
		return dst
	}
	if p.bare {
		body := p.section.View()
		for range p.height {
			var line string
			line, body, _ = strings.Cut(body, "\n")
			dst = append(dst, fit(line, p.width))
		}
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
	// base marks a base other than the head of the default branch.
	base       lipgloss.Style
	dot, badge lipgloss.Style
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
		base:       t.Accent,
		dot:        t.Accent,
		badge:      t.Muted,
	}
}

// drawFrame draws the edges of p's frame: the accent while it is focused,
// the border color otherwise.
func (m *Model) drawFrame(p *pane) {
	if p.bare {
		return
	}
	w := p.width
	if w < 2 || p.height < 2 {
		p.top, p.bottom, p.side = "", "", ""
		return
	}
	edge, title := m.st.edge, m.st.title
	if p.focused {
		edge, title = m.st.focusEdge, m.st.focusTitle
	}
	b := m.icons.Border
	label := termtext.Truncate(p.label, max(w-4, 0), m.icons.Ellipsis)
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

// drawHeader draws the header: the title of the screen on the left, which
// on the repository screen is the repository, after its owner's avatar
// where avatars are drawn, and its default branch, or the base its files
// are shown at, and on the owner screen the login of the page, and the
// unread notifications on the right, on a rule.
func (m *Model) drawHeader() {
	w := m.width
	if w <= 0 {
		m.header = ""
		return
	}
	rule := func(n int) string { return m.st.edge.Render(strings.Repeat(m.icons.Border.Top, max(n, 0))) }
	var name string
	switch {
	case m.screen == dashScreen:
		name = ui.DashboardTitle
	case m.screen == searchScreen:
		name = ui.SearchTitle
	case m.screen == notifScreen:
		name = ui.NotificationsTitle
	case m.screen == ownerScreen:
		// The page names its account once it has one.
		name = cmp.Or(m.ownerLogin, ui.OwnerTitle)
	case m.repo.Owner != "":
		name = m.repo.String()
	default:
		name = "No repository"
		if k := m.keys.Search.Help().Key; k != "" && m.keys.Search.Enabled() {
			name += m.icons.Separator + "press " + m.icons.Key(k) + " to search"
		}
	}
	left := m.st.repo.Render(name)
	switch {
	case m.screen == ownerScreen && m.ownerLogin != "":
		// The login links to the profile on GitHub.
		left = termtext.Link(ui.WebURL(m.host, m.ownerLogin), left)
	case name == m.repo.String():
		// The repository links to its page, after its owner's avatar,
		// which stands for its icon, as on GitHub; the avatar stays out
		// of the link's style.
		left = m.pics.Line(m.ownerAvatar) + termtext.Link(ui.WebURL(m.host, m.repo.String()), left)
	}
	switch {
	case m.screen != repoScreen:
	case m.base.Ref != "":
		left += " " + rule(1) + " " + m.st.base.Render(ui.OneLine(cmp.Or(m.base.Label, m.base.Ref)))
	case m.branch != "":
		left += " " + rule(1) + " " + m.st.branch.Render(ui.OneLine(m.branch))
	}
	var right string
	if m.badge != "" {
		noun := " notifications"
		if m.badge == "1" {
			noun = " notification"
		}
		right = m.st.dot.Render(m.icons.Dot) + " " + m.st.badge.Render(m.badge+noun)
	}
	// Two cells of rule and a space on each side frame the text.
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	if right != "" && lw+rw+8 > w {
		right, rw = "", 0
	}
	if right == "" {
		if lw+3 > w {
			m.header = fit(rule(1)+" "+termtext.Truncate(left, max(w-3, 0), m.icons.Ellipsis), w)
			return
		}
		m.header = rule(1) + " " + left + " " + rule(w-lw-3)
		return
	}
	m.header = rule(1) + " " + left + " " + rule(w-lw-rw-6) + " " + right + " " + rule(1)
}

// onePane reports whether the repository screen shows the focused pane
// alone: when the panes don't fit side by side, or while it is zoomed.
func (m *Model) onePane() bool { return m.width < narrowWidth || m.zoom }

// canZoom reports whether the zoom key does something: on the repository
// screen, with more than one pane.
func (m *Model) canZoom() bool {
	return m.screen == repoScreen && len(m.panes) > 1
}

// zoomed reports whether the zoom shows. A narrow terminal shows one pane
// anyway, so there the back key keeps its other uses.
func (m *Model) zoomed() bool { return m.zoom && m.width >= narrowWidth }

// setZoom shows the focused pane of the repository screen alone, or every
// pane again.
func (m *Model) setZoom(zoom bool) {
	m.zoom = zoom
	m.layout()
}

func filesWidth(width int) int {
	return min(max(width*2/5, minFilesWidth), maxFilesWidth)
}

// arrange sizes the panes for the terminal.
func (m *Model) arrange(height int) {
	if m.notif != nil {
		m.notif.resize(m.width, height)
	}
	for _, p := range []*pane{m.dash, m.srch, m.own} {
		if p != nil {
			p.resize(m.width, height)
		}
	}
	if len(m.panes) == 0 {
		return
	}
	if m.onePane() {
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
	switch m.screen {
	case notifScreen:
		return m.notif.appendLines(nil)
	case dashScreen:
		return m.dash.appendLines(nil)
	case searchScreen:
		return m.srch.appendLines(nil)
	case ownerScreen:
		return m.own.appendLines(nil)
	case repoScreen:
	}
	if len(m.panes) == 0 {
		return nil
	}
	if m.onePane() {
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
	switch m.screen {
	case notifScreen:
		return m.notif
	case dashScreen:
		return m.dash
	case searchScreen:
		return m.srch
	case ownerScreen:
		return m.own
	case repoScreen:
	}
	if len(m.panes) == 0 {
		return nil
	}
	return m.panes[m.focus]
}

// maxBack is how many places the back key remembers.
const maxBack = 50

// place is where the back key returns to: a screen, with the pane that was
// focused, the repository on view, and the login of the owner page.
type place struct {
	screen screen
	focus  int
	repo   core.RepoRef
	owner  string
}

// place returns the place on view.
func (m *Model) place() place {
	return place{screen: m.screen, focus: m.focus, repo: m.repo, owner: m.ownerLogin}
}

// pushBack remembers the place on view, for the back key.
func (m *Model) pushBack() {
	m.back = append(m.back, m.place())
	if len(m.back) > maxBack {
		m.back = slices.Clone(m.back[len(m.back)-maxBack:])
	}
}

// canGoBack reports whether the back key has somewhere to go: a page of an
// owner before this one, or a place before the screen.
func (m *Model) canGoBack() bool {
	if m.modal != nil {
		return false
	}
	if b := m.ownerPages(); b != nil && m.screen == ownerScreen && b.CanGoBack() {
		return true
	}
	return len(m.back) > 0
}

// ownerPages returns the owner section's pages to go back through, or nil.
func (m *Model) ownerPages() PageBacker {
	if m.own == nil {
		return nil
	}
	b, _ := m.own.section.(PageBacker)
	return b
}

// goBack goes back as a browser does: through the owner pages, then to the
// places before them, newest first. It does nothing at the bottom, or over
// a modal.
func (m *Model) goBack() tea.Cmd {
	if m.modal != nil {
		return nil
	}
	if b := m.ownerPages(); b != nil && m.screen == ownerScreen && b.CanGoBack() {
		cmd := b.GoBack()
		m.updateOwner()
		return cmd
	}
	if len(m.back) == 0 {
		return nil
	}
	p := m.back[len(m.back)-1]
	m.back = m.back[:len(m.back)-1]
	var cmds []tea.Cmd
	switch {
	// A repository gone back to opens as selecting it does, on its default
	// branch, whatever base or branch it was left on.
	case p.screen == repoScreen && p.repo != (core.RepoRef{}) && !p.repo.Same(m.repo):
		cmds = append(cmds, m.openRepo(ui.RepoMsg{Repo: p.repo}, p.focus))
	case p.screen == ownerScreen && p.owner != "" && !strings.EqualFold(p.owner, m.ownerLogin) && m.own != nil:
		// The page isn't the one on view, which the owner section takes
		// as a new start, since it isn't shown now.
		cmds = append(cmds, m.own.section.Update(ui.OwnerMsg{Login: p.owner}))
		m.updateOwner()
	}
	return tea.Batch(append(cmds, m.reveal(p.screen, p.focus))...)
}

// showScreen shows screen s, with pane i focused on the repository screen,
// and starts the sections that come into view. The screen it leaves is the
// one the back key returns to.
func (m *Model) showScreen(s screen, i int) tea.Cmd {
	if s != m.screen && m.hasScreen(s) {
		m.pushBack()
	}
	return m.reveal(s, i)
}

// hasScreen reports whether the app has screen s to show.
func (m *Model) hasScreen(s screen) bool {
	switch s {
	case notifScreen:
		return m.notif != nil
	case repoScreen:
		return len(m.panes) > 0
	case dashScreen:
		return m.dash != nil
	case searchScreen:
		return m.srch != nil
	case ownerScreen:
		return m.own != nil
	}
	return false
}

// reveal shows screen s as showScreen does, without remembering the place
// it leaves.
func (m *Model) reveal(s screen, i int) tea.Cmd {
	if !m.hasScreen(s) {
		return nil
	}
	before := m.focused()
	if s != m.screen {
		// Going elsewhere drops a goto still waiting.
		m.cancelGoto()
		defer m.drawHeader()
	}
	m.screen = s
	if s == repoScreen {
		m.focus = min(max(i, 0), len(m.panes)-1)
	}
	var revisit tea.Cmd
	if after := m.focused(); after != before {
		if before != nil {
			before.setFocus(false)
			m.drawFrame(before)
		}
		after.setFocus(true)
		m.drawFrame(after)
		// A section shown before may have gone stale meanwhile; one not
		// started yet reads everything as it starts.
		if r, ok := after.section.(ui.Revisiter); ok && after.started {
			revisit = r.Revisit()
		}
	}
	return tea.Batch(m.startScreen(), revisit)
}

// cycle moves the focus by delta panes on the repository screen, the only
// screen whose panes the app cycles through.
func (m *Model) cycle(delta int) tea.Cmd {
	if len(m.panes) == 0 {
		return nil
	}
	n := len(m.panes)
	return m.showScreen(repoScreen, ((m.focus+delta)%n+n)%n)
}
