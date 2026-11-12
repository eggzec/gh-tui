package tui

import (
	"cmp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// screen is what fills the space between the header and the status bar.
type screen int

const (
	// repoScreen shows the panes of the selected repository.
	repoScreen screen = iota
	// dashScreen shows the dashboard.
	dashScreen
	// ownerScreen shows the page of a user or an organization.
	ownerScreen
)

// Sizes of the repository screen. Below narrowWidth columns the panes
// don't fit side by side, so the screen shows the focused pane alone, at
// full size, and the pane keys switch between them. Above it, the files
// take two fifths of the width, within the bounds.
const maxFilesWidth = 60

// pane is a section in a frame whose top edge carries its label, or a
// bare one, which fills its place and draws its own frames.
type pane struct {
	section ui.Section
	// preview, when set, makes the pane the preview box beside a zoomed
	// section: it has no section, and shows what the splitter gives.
	preview ui.Splitter
	// label is the name shown in the top edge, such as "Files", with the
	// pane's key before it, "[1] Files", unless the pane is zoomed: then
	// its key is in the bottom edge, among those of the other panes.
	label, key string
	// previewTitle is the title the preview's top edge was last drawn with.
	previewTitle string
	// count is the forms of the count the frame was last drawn with, in its
	// top-right.
	count   []string
	bare    bool
	started bool
	focused bool
	// width and height are the outer size, frame included.
	width, height int
	// The edges of the frame, drawn for the size, focus and theme.
	top, bottom, side string
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
	if p.preview != nil {
		p.preview.SetPreviewSize(max(p.width-2, 0), max(p.height-2, 0))
		return
	}
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
	var body string
	if p.preview != nil {
		body = p.preview.PreviewView()
	} else {
		body = p.section.View()
	}
	inner := p.width - 2
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
	edge         lipgloss.Style
	repo, branch lipgloss.Style
	// base marks a base other than the head of the default branch.
	base       lipgloss.Style
	dot, badge lipgloss.Style
	// frame draws the edges of a pane and the strip of panes in the bottom
	// edge of a zoomed one.
	frame panes.Styles
}

func newStyles(t ui.Theme) styles {
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(t.Palette.Border))
	frame := panes.DefaultStyles(t.Dark).WithAccent(t.Accent)
	frame.Edge, frame.Title = border, t.Muted
	// The strip's forms carry their own styles.
	frame.Info = lipgloss.NewStyle()
	return styles{
		edge:   border,
		repo:   t.Title,
		branch: t.Muted,
		base:   t.Accent,
		dot:    t.Accent,
		badge:  t.Muted,
		frame:  frame,
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
	frames := m.st.frame
	frames.Border = m.icons.Border
	frames.Ellipsis = m.icons.Ellipsis
	frames.Separator = m.icons.Separator
	zoomed := m.zoom.Zoomed() && p.key != ""
	text := p.label
	if p.preview != nil {
		text = p.preview.PreviewTitle()
	}
	if p.preview == nil {
		text = m.linkTitle(text)
	}
	if p.key != "" && !zoomed {
		text = "[" + p.key + "] " + text
	}
	c := panes.Content{Focus: m.focusMarked(p)}
	if label := termtext.Truncate(text, max(w-5, 0), m.icons.Ellipsis); label != "" {
		c.TopLeft = []string{label}
	}
	if p.preview == nil && len(p.count) > 0 {
		c.TopRight = p.count
		c.TopLeft = ui.TitleBeside([]string{text}, p.count, max(w-5, 0), m.icons.Ellipsis)
	}
	if zoomed {
		c.BottomLeft = m.foot(p, frames)
	}
	p.top, p.bottom = frames.Edges(c, w)
	p.side, _ = frames.Sides(m.focusMarked(p))
}

// linkTitle links the name that starts a pane's label, on the repository
// screen, to the page on GitHub that lists what the pane shows: the pull
// requests and the issues of the repository.
func (m *Model) linkTitle(label string) string {
	if m.screen != repoScreen || m.repo.Owner == "" {
		return label
	}
	for name, page := range map[string]string{ui.PullsTitle: "pulls", ui.IssuesTitle: "issues"} {
		if rest, ok := strings.CutPrefix(label, name); ok && (rest == "" || strings.HasPrefix(rest, m.icons.Separator)) {
			return termtext.Link(ui.WebURL(m.host, m.repo.String()+"/"+page), name) + rest
		}
	}
	return label
}

// foot is the forms of the strip of the keys of the panes of the repository
// screen, with the one of p in brackets, for the bottom edge of the zoomed
// pane.
func (m *Model) foot(p *pane, frames panes.Styles) []string {
	defs := make([]panes.Def, len(m.panes))
	cur := panes.Pane(0)
	for i, q := range m.panes {
		defs[i] = panes.Def{Name: q.label, Short: q.label}
		if q == p {
			cur = panes.Pane(i + 1)
		}
	}
	return frames.StripForms(defs, cur)
}

func (m *Model) drawFrames() {
	for _, p := range m.all {
		m.drawFrame(p)
	}
	if m.prev != nil {
		m.drawFrame(m.prev)
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
	rule := func(n int) string { return m.st.edge.Render(ui.Rule(m.icons, n)) }
	var name string
	switch {
	case m.screen == dashScreen:
		name = ui.DashboardTitle
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
		// The count links to the inbox on GitHub.
		right = m.st.dot.Render(m.icons.Dot) + " " + termtext.Link(ui.WebURL(m.host, "notifications"), m.st.badge.Render(m.badge+noun))
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
func (m *Model) onePane() bool { return m.zoom.Zoomed() }

// narrow reports whether the terminal is too small for the panes of the
// dashboard and the owner page to show side by side, by ui.wide. It is
// decided here, from the whole terminal, once for every screen.
func (m *Model) narrow() bool {
	return m.width < m.wide.Width || m.height < m.wide.Height
}

// canZoom reports whether the zoom key does something: on the repository
// screen, with more than one pane, unless a narrow terminal forces the zoom.
func (m *Model) canZoom() bool {
	return m.screen == repoScreen && len(m.panes) > 1 && m.zoom.CanZoom()
}

// zoomed reports whether the user zoomed the repository screen, which the
// zoom key undoes. A narrow terminal shows one pane anyway, so there the keys
// keep their other uses.
func (m *Model) zoomed() bool { return m.zoom.User() && m.zoom.CanZoom() }

// filesWidth is the files column's width when the user has not moved its
// divider.
func filesWidth(width int) int {
	return min(max(width*2/5, panes.MinWidth), maxFilesWidth)
}

// bounds returns the narrowest and widest a divider's first pane may be: the
// other keeps at least panes.MinWidth cells.
func (m *Model) bounds() (lo, hi int) { return panes.MinWidth, m.width - panes.MinWidth }

// filesDivider is the files column's width with the user's move applied.
func (m *Model) filesDivider() int {
	lo, hi := m.bounds()
	return m.dividers.Files.Of(filesWidth(m.width), lo, hi)
}

// listWidth is the list's width beside its preview, with the user's move
// applied.
func (m *Model) listWidth() int {
	lo, hi := m.bounds()
	return m.dividers.List.Of(panes.DefaultListWidth, lo, hi)
}

// pageWidth is the width of the first column of the dashboard and of the owner
// page when the user has not moved their divider.
func pageWidth(width int) int { return width * 11 / 20 }

// pageDivider is the first column's width of the dashboard and the owner page
// with the user's move applied.
func (m *Model) pageDivider() int {
	lo, hi := m.bounds()
	return m.dividers.Page.Of(pageWidth(m.width), lo, hi)
}

// pageSection is what the root needs of the dashboard and the owner page to
// move their divider.
type pageSection interface {
	SetSplit(width int)
	OnePane() bool
}

// pageOf returns the pane of the dashboard or the owner page, or nil for any
// other screen.
func (m *Model) pageOf(s screen) *pane {
	switch s {
	case dashScreen:
		return m.dash
	case ownerScreen:
		return m.own
	case repoScreen:
	}
	return nil
}

// pageShowsTwo reports whether the page shows its two columns side by side, so
// that there is a divider between them.
func (m *Model) pageShowsTwo(p *pane) bool {
	s, ok := p.section.(pageSection)
	lo, hi := m.bounds()
	return ok && !s.OnePane() && !m.narrow() && hi >= lo
}

// divider returns the size of the divider that the divider keys move, with
// its default, or nil when the screen shows none. The repository screen shows
// the files column beside the right one, or, zoomed on a section with a
// preview, that section's list beside it.
func (m *Model) divider() (s *panes.Size, def int) {
	if m.screen != repoScreen {
		if p := m.pageOf(m.screen); p != nil && m.pageShowsTwo(p) {
			return &m.dividers.Page, pageWidth(m.width)
		}
		return nil, 0
	}
	if m.onePane() {
		if m.splitOf == nil {
			return nil, 0
		}
		return &m.dividers.List, panes.DefaultListWidth
	}
	if len(m.leftC) == 0 || len(m.rightC) == 0 {
		return nil, 0
	}
	return &m.dividers.Files, filesWidth(m.width)
}

// canDivide reports whether the divider keys have a divider to move.
func (m *Model) canDivide() bool {
	s, _ := m.divider()
	return s != nil
}

// moveDivider moves the divider by steps presses and lays the panes out again.
func (m *Model) moveDivider(steps int) {
	s, def := m.divider()
	if s == nil {
		return
	}
	lo, hi := m.bounds()
	m.dividers.Move(s, steps, def, lo, hi)
	m.layout()
}

// arrange sizes the panes for the terminal.
func (m *Model) arrange(height int) {
	if m.notif != nil {
		m.notif.resize(m.width, height)
	}
	narrow := m.narrow()
	m.zoom.SetNarrow(narrow)
	for _, p := range []*pane{m.dash, m.own} {
		if p == nil {
			continue
		}
		if s, ok := p.section.(interface{ SetNarrow(bool) }); ok {
			s.SetNarrow(narrow)
		}
		if s, ok := p.section.(pageSection); ok {
			s.SetSplit(m.pageDivider())
		}
		p.resize(m.width, height)
	}
	if len(m.panes) == 0 {
		return
	}
	if m.onePane() {
		for _, p := range m.panes {
			p.resize(m.width, height)
		}
		m.split(height)
		return
	}
	m.endSplit()
	left, right := m.leftC, m.rightC
	lw := 0
	switch {
	case len(right) == 0:
		lw = m.width
	case len(left) > 0:
		lw = m.filesDivider()
	}
	if len(left) == 2 {
		// Files over the releases, which have a third of the height.
		top := height - height/3
		left[0].resize(lw, top)
		left[1].resize(lw, height-top)
	} else {
		stack(left, lw, height)
	}
	if len(right) == 2 {
		top := m.stackTop(height)
		right[0].resize(m.width-lw, top)
		right[1].resize(m.width-lw, height-top)
		return
	}
	stack(right, m.width-lw, height)
}

// stackBounds returns the height the pull requests have unmoved, over the
// issues, and the least and most the user may give them.
func stackBounds(height int) (def, lo, hi int) {
	return (height + 1) / 2, panes.MinHeight, height - panes.MinHeight
}

// stackTop is the height of the pull requests, the top pane of the stack on
// the repository screen, with the user's move applied.
func (m *Model) stackTop(height int) int {
	def, lo, hi := stackBounds(height)
	return m.dividers.Stack.Of(def, lo, hi)
}

// canStack reports whether the stack keys have a divider to move: the repository
// screen shows the pull requests over the issues, and one of them has the focus.
func (m *Model) canStack() bool {
	if m.screen != repoScreen || m.onePane() || len(m.rightC) != 2 || !slices.Contains(m.rightC, m.panes[m.focus]) {
		return false
	}
	_, lo, hi := stackBounds(m.contentHeight())
	return hi >= lo
}

// growPane grows the focused pane of the stack by dir presses, or shrinks it
// when dir is negative, and lays the panes out again.
func (m *Model) growPane(dir int) {
	if !m.canStack() {
		return
	}
	_, sign := panes.Grow(slices.Index(m.rightC, m.panes[m.focus]), 2)
	def, lo, hi := stackBounds(m.contentHeight())
	m.dividers.Move(&m.dividers.Stack, dir*sign, def, lo, hi)
	m.layout()
}

// split lays the zoomed pane out beside a preview box when its section is a
// ui.Splitter and the terminal is not narrow, and otherwise tells the section
// there is no preview box.
func (m *Model) split(height int) {
	p := m.panes[m.focus]
	s, ok := p.section.(ui.Splitter)
	if !ok || m.narrow() {
		m.endSplit()
		return
	}
	if m.splitOf != nil && m.splitOf != p {
		m.endSplit()
	}
	defs := make([]panes.Def, len(m.panes))
	for i, q := range m.panes {
		defs[i] = panes.Def{Name: q.label, Short: q.label, Split: q == p}
	}
	focus := panes.Pane(m.focus + 1)
	lay := panes.Compute(panes.Spec{
		Width: m.width, Height: height, Panes: defs, Focus: focus, Zoom: true,
		SplitMin: m.wide.Width, ListWidth: m.listWidth(),
	})
	list, hasList := lay.Box(focus, panes.List)
	box, hasPreview := lay.Box(focus, panes.Preview)
	if !hasList || !hasPreview {
		m.endSplit()
		return
	}
	p.resize(list.Rect.W, list.Rect.H)
	if m.prev == nil {
		m.prev = &pane{}
	}
	m.splitOf = p
	m.prev.preview = s
	m.prev.resize(box.Rect.W, box.Rect.H)
}

// endSplit drops the preview box, and tells the section that had one.
func (m *Model) endSplit() {
	if m.prev == nil {
		return
	}
	m.prev.preview.SetPreviewFocus(false)
	m.prev.preview.SetPreviewSize(0, 0)
	m.prev, m.splitOf = nil, nil
	m.prevFocus = false
}

// splitShown reports whether the zoomed pane is on view beside its preview.
func (m *Model) splitShown() bool {
	return m.screen == repoScreen && m.prev != nil && m.splitOf != nil
}

// switchSplit moves the focus between the list and the preview.
func (m *Model) switchSplit() {
	m.prevFocus = !m.prevFocus
	m.prev.preview.SetPreviewFocus(m.prevFocus)
	m.drawFrame(m.splitOf)
	m.drawFrame(m.prev)
}

// focusMarked reports whether p's frame is drawn in the focus color.
func (m *Model) focusMarked(p *pane) bool {
	switch {
	case p == m.prev && p != nil:
		return m.prevFocus
	case p == m.splitOf && p != nil:
		return p.focused && !m.prevFocus
	}
	return p.focused
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
	case dashScreen:
		return m.dash.appendLines(nil)
	case ownerScreen:
		return m.own.appendLines(nil)
	case repoScreen:
	}
	if len(m.panes) == 0 {
		return nil
	}
	if m.onePane() {
		lines := m.panes[m.focus].appendLines(nil)
		if m.prev == nil {
			return lines
		}
		right := m.prev.appendLines(nil)
		for i := range min(len(lines), len(right)) {
			lines[i] += right[i]
		}
		return lines
	}
	h := m.contentHeight()
	left := make([]string, 0, h)
	for _, p := range m.leftC {
		left = p.appendLines(left)
	}
	right := make([]string, 0, h)
	for _, p := range m.rightC {
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
	case dashScreen:
		return m.dash
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

// maxBackModals is how many of those places may be modals. A modal kept
// for the back key holds what it has read, such as its rendered thread, so
// this bounds memory; the oldest is dropped past it.
const maxBackModals = 10

// place is where the back key returns to: a screen, with the pane that was
// focused, the repository on view, and the login of the owner page, or a
// modal that another opened in its place, if modal is set.
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
	if m.modals.Top() != nil {
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
// places before them, newest first. It does nothing at the bottom. An open
// modal takes the back key before it gets here.
func (m *Model) goBack() tea.Cmd {
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
	case repoScreen:
		return len(m.panes) > 0
	case dashScreen:
		return m.dash != nil
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
