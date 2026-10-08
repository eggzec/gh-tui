package tui

import (
	"cmp"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// topModal returns the open modal, or nil if none is open.
func (m *Model) topModal() ui.Modal {
	return m.modal
}

// openModal opens mod in place of the open modal, if any, which hides:
// modals never stack, so the screen shows at most one frame over it. It
// closes the command line, whose command was typed for what showed before,
// such as when a goto run before ends while the user types another.
func (m *Model) openModal(mod ui.Modal) {
	if mod == nil {
		return
	}
	m.cancelGoto()
	if m.line.Focused() {
		m.line.Blur()
		m.lineDone()
	}
	if h, ok := m.modal.(ui.Hider); ok && m.modal != mod {
		h.Hide()
	}
	// The help lists the keys of what had them before.
	m.keyhelp.Blur()
	if m.modal != mod {
		m.maximized = m.opensMaximized(mod)
	}
	m.modal = mod
	mod.SetTheme(m.theme)
	m.sizeModal(m.modalSize())
}

// sizeModal gives the open modal its size.
func (m *Model) sizeModal(width, height int) {
	m.modalWidth, m.modalHeight = width, height
	m.modal.SetSize(width, height)
}

// modalContext returns the name of the key context of mod, the modal that
// its first layer of keys is, or "" if none is. A modal context that is
// within another, such as the filter of the runs, is that other one.
func modalContext(mod ui.Modal) string {
	for _, l := range mod.KeyLayers() {
		if c, ok := config.LookupContext(l.Context); ok && c.Modal {
			return cmp.Or(c.Within, c.Name)
		}
	}
	return ""
}

// opensMaximized reports whether ui.maximized lists mod.
func (m *Model) opensMaximized(mod ui.Modal) bool {
	name := modalContext(mod)
	return name != "" && slices.Contains(m.maximizedModals, name)
}

// modalTakesKeys reports whether the open modal types the keys into an
// input, or takes them for a question or a prompt, so that the app leaves
// it every key but ctrl+c and the help key.
func (m *Model) modalTakesKeys() bool {
	inner, _ := m.innerLayers()
	return slices.ContainsFunc(inner, takesKeys)
}

// overModal is what an intent of the global keys that reaches the open
// modal is called, and what its refusal says.
type overModal struct {
	// binding is the key of the intent.
	binding key.Binding
	// action is the name the modal knows the intent by, and what the
	// refusal names, such as "notifications".
	action, use string
}

// overModals returns the intents of the global keys that the open modal
// may take, and that the app refuses over it otherwise: leaving for
// another screen. The quit key is another that a modal may take; the
// modal that doesn't gets the key.
func (m *Model) overModals() []overModal {
	k := m.keys
	var out []overModal
	if m.dash != nil {
		out = append(out, overModal{k.Dashboard, "dashboard", "the dashboard"})
	}
	if m.notif != nil {
		out = append(out, overModal{k.Notifications, "notifications", "notifications"})
	}
	if m.srch != nil {
		out = append(out, overModal{k.Search, "search", "search"})
	}
	return append(out, overModal{k.Repo, "repo", "the repository"}, overModal{k.Owner, "owner", "the owner page"})
}

// actOverModal offers the intent of the global key msg to the open modal,
// which takes it, or else refuses it: the keys that show another screen
// work only once the modal is closed. It reports whether it did, and
// otherwise msg goes to the modal as a key.
func (m *Model) actOverModal(mod ui.Modal, msg tea.KeyPressMsg) (tea.Cmd, bool) {
	actor, _ := mod.(ui.Actor)
	act := func(action string) (tea.Cmd, bool) {
		if actor == nil {
			return nil, false
		}
		return actor.Act(action)
	}
	if key.Matches(msg, m.keys.Quit) {
		return act(ui.ActQuit)
	}
	for _, o := range m.overModals() {
		if !key.Matches(msg, o.binding) {
			continue
		}
		if cmd, ok := act(o.action); ok {
			return cmd, true
		}
		return m.toast.Push(toast.Warning, "Close "+m.modalName(mod)+" first to use "+o.use+"."), true
	}
	return nil, false
}

// modalName names mod for the refusal that asks to close it, with its
// article, such as "the pull request": the key context's title, but for
// the views that have a name of their own.
func (m *Model) modalName(mod ui.Modal) string {
	if _, ok := mod.(*authModal); ok {
		return "the token prompt"
	}
	switch name := modalContext(mod); name {
	case "history":
		return "History"
	case "actions":
		return "Actions"
	case "text":
		return "the pager"
	case "filter":
		return "the filter"
	case "preview":
		return "the file"
	default:
		c, ok := config.LookupContext(name)
		if !ok {
			return "the modal"
		}
		return "the " + strings.ToLower(c.Title)
	}
}

// takesKeys reports whether l is the keys of what takes every key it can
// type, or all of them, such as an input or a question.
func takesKeys(l keyhelp.Layer) bool { return l.Typing || capturing(l) }

// toggleMaximized switches the open modal between its size and the whole
// screen, and gives it the room, which it waits out as after a resize.
func (m *Model) toggleMaximized() {
	m.maximized = !m.maximized
	m.resized = true
	m.sizeModal(m.modalSize())
}

// settleModal asks the open modal, if it waits out resizes, to end the
// wait that the layout's resize began.
func (m *Model) settleModal() tea.Cmd {
	if s, ok := m.modal.(ui.Settler); ok {
		return s.Settle()
	}
	return nil
}

// isOpen reports whether mod is open.
func (m *Model) isOpen(mod ui.Modal) bool {
	return mod != nil && m.modal == mod
}

// openHistory opens the history of the selected repository, on the
// repository screen, if the app has one.
func (m *Model) openHistory() tea.Cmd {
	if m.history == nil || m.screen != repoScreen || m.repo == (core.RepoRef{}) {
		return nil
	}
	mod, load := m.history(m.ctx, m.repo, m.branch, m.base)
	if mod == nil {
		return nil
	}
	m.openModal(mod)
	return load
}

// canOpenHistory reports whether the history key does something.
func (m *Model) canOpenHistory() bool {
	return m.history != nil && m.screen == repoScreen && m.repo != (core.RepoRef{})
}

// openActions opens the workflow runs of the selected repository, on the
// repository screen, if the app has them.
func (m *Model) openActions() tea.Cmd {
	if !m.canOpenActions() {
		return nil
	}
	return m.openActionsOn(m.repo, core.RunFilter{})
}

// openActionsOn opens the runs of repo that f selects, if the app has
// them, on any screen.
func (m *Model) openActionsOn(repo core.RepoRef, f core.RunFilter) tea.Cmd {
	if m.actions == nil || repo == (core.RepoRef{}) {
		return nil
	}
	mod, load := m.actions(m.ctx, repo, f)
	if mod == nil {
		return nil
	}
	m.openModal(mod)
	return load
}

// openCommit opens the history of the repository of msg on its commit, if
// the app has a history, on any screen.
func (m *Model) openCommit(msg ui.OpenCommitMsg) tea.Cmd {
	if m.commit == nil || msg.Repo == (core.RepoRef{}) || msg.SHA == "" {
		return nil
	}
	var branch string
	if msg.Repo.Same(m.repo) {
		branch = m.branch
	}
	mod, load := m.commit(m.ctx, msg.Repo, msg.SHA, branch)
	if mod == nil {
		return nil
	}
	m.openModal(mod)
	return load
}

// openRelease opens the release of msg, if the app has a release modal,
// on any screen.
func (m *Model) openRelease(msg ui.OpenReleaseMsg) tea.Cmd {
	if m.release == nil || msg.Repo == (core.RepoRef{}) {
		return nil
	}
	mod, load := m.release(m.ctx, msg.Repo, msg.ID, msg.URL)
	if mod == nil {
		return nil
	}
	m.openModal(mod)
	return load
}

// fileFinder returns the section of the repository screen that finds its
// files, if the screen is on view with a repository, or nil.
func (m *Model) fileFinder() ui.FileFinder {
	if m.screen != repoScreen || m.repo == (core.RepoRef{}) {
		return nil
	}
	for _, p := range m.panes {
		if f, ok := p.section.(ui.FileFinder); ok {
			return f
		}
	}
	return nil
}

// findFile opens the file finder of the repository screen.
func (m *Model) findFile() tea.Cmd {
	f := m.fileFinder()
	if f == nil {
		return nil
	}
	mod, load := f.FindFile()
	if mod == nil {
		return nil
	}
	m.openModal(mod)
	return load
}

// canOpenActions reports whether the actions key does something.
func (m *Model) canOpenActions() bool {
	return m.actions != nil && m.screen == repoScreen && m.repo != (core.RepoRef{})
}

// closeModal closes mod. A modal that isn't open is ignored, so closing
// twice, or closing one that another has replaced, is harmless.
func (m *Model) closeModal(mod ui.Modal) {
	if m.isOpen(mod) {
		m.modal = nil
	}
}

// frameSize is the size of the open modal with its frame: most of the
// screen, so that the edges of the screen behind it stay in view, or less
// if the modal fits in less.
func (m *Model) frameSize() (width, height int) {
	// A maximized modal ignores what it fits in, and fills the screen but
	// for the footer.
	if m.maximized {
		return m.width, max(m.height-m.footerHeight(), 0)
	}
	// The frame is centred, and leaves the footer below it, such as the
	// command line with its candidates.
	width, height = max(m.width-2*max(m.width/10, 2), 0), max(m.height-2*max(m.height/10, m.footerHeight()), 0)
	if f, ok := m.modal.(ui.Fitter); ok {
		w, h := f.Fit(max(width-4, 0), max(height-2, 0))
		width, height = min(width, w+4), min(height, h+2)
	}
	return width, height
}

// modalSize is the size inside the frame: the frame takes a row at the top
// and bottom and two columns on each side, the edge and its padding.
func (m *Model) modalSize() (width, height int) {
	w, h := m.frameSize()
	return max(w-4, 0), max(h-2, 0)
}

// fitModal sizes the open modal again if what it asks for has changed,
// such as a filter form that opens a picker and needs the room.
func (m *Model) fitModal() {
	if _, ok := m.modal.(ui.Fitter); !ok {
		return
	}
	if w, h := m.modalSize(); w != m.modalWidth || h != m.modalHeight {
		m.resized = true
		m.sizeModal(w, h)
	}
}

// minTitleWidth is the least of a long title that the tabs of a modal
// leave in the top edge of its frame.
const minTitleWidth = 16

// frame draws mod in its frame, with its title in the top edge, and its
// tabs, if it has them and they fit, at the right end of it. The tabs
// shorten a long title, down to minTitleWidth, so they show at 80 columns.
func (m *Model) frame(mod ui.Modal) string {
	w, _ := m.frameSize()
	if w < 4 {
		return ""
	}
	b := m.icons.Border
	var tabs string
	if t, ok := mod.(ui.Tabbed); ok {
		tabs = m.tabs(t)
	}
	tw := lipgloss.Width(tabs)
	titleWidth := max(w-4, 0)
	// The tabs keep a stretch of the edge before them, or give way.
	if room := w - 6 - tw; tabs != "" && room >= minTitleWidth {
		titleWidth = min(titleWidth, room)
	}
	// A title may hold text from GitHub, such as a pull request's.
	title := termtext.Truncate(" "+ui.OneLine(mod.Title())+" ", titleWidth, m.icons.Ellipsis+" ")
	rest := max(w-3-lipgloss.Width(title), 0)
	switch {
	case tabs == "":
	case tw+3 <= rest:
		rest -= tw + 1
	default:
		tabs = ""
	}
	title = m.theme.Title.Render(title)
	if l, ok := mod.(ui.Linked); ok {
		title = m.links.Link(l.Link(), title)
	}
	top := m.theme.Accent.Render(b.TopLeft+b.Top) + title +
		m.theme.Accent.Render(strings.Repeat(b.Top, rest))
	if tabs != "" {
		top += tabs + m.theme.Accent.Render(b.Top)
	}
	top += m.theme.Accent.Render(b.TopRight)
	return top + "\n" + m.frameStyle().Render(mod.View())
}

// frameStyle draws the sides and bottom of a frame like a modal's, in the
// theme's frame with the border of the icon set.
func (m *Model) frameStyle() lipgloss.Style {
	return m.theme.Frame().Border(m.icons.Border, false, true, true)
}

// tabs renders the tabs of t for the top edge of a frame: the one shown in
// the title's style, the others muted.
func (m *Model) tabs(t ui.Tabbed) string {
	names, active := t.Tabs()
	if len(names) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(" ")
	for i, name := range names {
		if i > 0 {
			b.WriteString(m.theme.Subtle.Render(m.icons.Separator))
		}
		st := m.theme.Muted
		if i == active {
			st = m.theme.Title
		}
		b.WriteString(st.Render(name))
	}
	b.WriteString(" ")
	return b.String()
}
