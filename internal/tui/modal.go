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
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// modalContext returns the name of the key context of mod, the modal that
// its first layer of keys is, or "" if none is. A modal context that is
// within another, such as the filter of the runs, is that other one.
func modalContext(mod ui.Modal) string {
	// A modal that shows steps with contexts of their own says which it is.
	if h, ok := mod.(ui.Hosted); ok {
		return h.KeyContext()
	}
	for _, l := range mod.KeyLayers() {
		if c, ok := config.LookupContext(l.Context); ok && c.Modal && !c.Step {
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
	// refusal names, such as "search".
	action, use string
}

// overModals returns the intents of the global keys that the open modal
// may take, and that the app refuses over it otherwise: leaving for
// another screen. The quit, back and dismiss keys are others that a modal
// may take; the modal that doesn't gets the key.
func (m *Model) overModals() []overModal {
	k := m.keys
	var out []overModal
	if m.dash != nil {
		out = append(out, overModal{k.Dashboard, "dashboard", "the dashboard"})
	}
	if m.notif != nil {
		out = append(out, overModal{k.Notifications, "notifications", "notifications"})
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
	switch {
	case keymap.Matches(msg, m.keys.Quit):
		return act(ui.ActQuit)
	case keymap.Matches(msg, m.keys.Back):
		// The modal steps back inside itself first; otherwise the key
		// returns to the modal it replaced, if there is one, and else
		// reaches the modal as a key.
		if cmd, ok := act(ui.ActBack); ok {
			return cmd, true
		}
		if m.modals.CanBack() {
			return m.apply(m.modals.Back(mod)), true
		}
		return nil, false
	case keymap.Matches(msg, m.keys.Dismiss):
		return act(ui.ActDismiss)
	}
	if m.srch != nil && keymap.Matches(msg, m.keys.Search) {
		if cmd, ok := act("search"); ok {
			return cmd, true
		}
		return m.openSearch(), true
	}
	for _, o := range m.overModals() {
		if !keymap.Matches(msg, o.binding) {
			continue
		}
		if cmd, ok := act(o.action); ok {
			return cmd, true
		}
		if o.action == "notifications" {
			return m.openNotifications(), true
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
	case "notifications":
		return "the notifications"
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
	if m.modals.ToggleMaximized() {
		m.resized = true
	}
}

// canMaximize reports whether the maximize key changes what the open modal
// shows; a modal that doesn't say is taken to change it.
func (m *Model) canMaximize() bool {
	if mx, ok := m.modals.Top().(ui.Maximizer); ok {
		return mx.CanMaximize()
	}
	return true
}

// settleModal asks the open modal, if it waits out resizes, to end the
// wait that the layout's resize began.
func (m *Model) settleModal() tea.Cmd {
	if s, ok := m.modals.Top().(ui.Settler); ok {
		return s.Settle()
	}
	return nil
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
	return m.openLoading(mod, nil, load)
}

// openLoading opens mod over the modal over, if any, and runs load only if
// mod is on top afterwards: a refused open reads nothing.
func (m *Model) openLoading(mod, over ui.Modal, load tea.Cmd) tea.Cmd {
	cmd := m.open(mod, over)
	if m.modals.Top() != mod {
		return cmd
	}
	return tea.Batch(cmd, load)
}

// openNotifications shows the notifications in a modal, over what is open.
func (m *Model) openNotifications() tea.Cmd {
	if m.notif == nil {
		return nil
	}
	sec, ok := m.notif.section.(*notifications.Section)
	if !ok {
		return nil
	}
	return m.open(notifications.NewModal(sec), nil)
}

// notifSection returns the notifications section.
func (m *Model) notifSection() ui.Section {
	if m.notif == nil {
		return nil
	}
	return m.notif.section
}

// notifHost returns the notifications modal if it is the open one.
func (m *Model) notifHost() ui.Modal {
	if h, ok := m.modals.Top().(*notifications.Modal); ok {
		return h
	}
	return nil
}

// openSearch shows the search in a modal, with the focus in an empty query,
// if the app has one. The search keeps its recent searches, but not the last
// query, so typing starts a new one.
func (m *Model) openSearch() tea.Cmd {
	if m.srch == nil {
		return nil
	}
	sec, ok := m.srch.section.(*search.Section)
	if !ok {
		return nil
	}
	mod := search.NewModal(sec)
	cmd := m.open(mod, nil)
	switch {
	case m.modals.Top() == mod:
		if f, ok := m.srch.section.(Fresher); ok {
			f.Fresh()
		}
		return tea.Batch(cmd, m.srch.start())
	case m.searchHost() != nil:
		sec.Focus()
	}
	return cmd
}

// leaveSearch closes the search modal if it is the open one.
func (m *Model) leaveSearch() tea.Cmd {
	if h := m.searchHost(); h != nil {
		return m.apply(m.modals.Close(h))
	}
	return nil
}

// hosted returns the list a filter or sort message acts on when a modal is
// open, with the modal, or nil if there is none: the section a host modal
// shows, else the top modal itself if it is a [ui.Filterable].
func (m *Model) hosted() (titled, ui.Modal) {
	switch {
	case m.notifHost() != nil:
		return m.notifSection(), m.notifHost()
	case m.searchHost() != nil && m.srch != nil:
		return m.srch.section, m.searchHost()
	}
	if top := m.modals.Top(); top != nil {
		if _, ok := top.(ui.Filterable); ok {
			return top, top
		}
	}
	return nil, nil
}

// searchHost returns the search modal if it is the open one.
func (m *Model) searchHost() ui.Modal {
	if h, ok := m.modals.Top().(*search.Modal); ok {
		return h
	}
	return nil
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
	return m.openActionsOn(ui.OpenActionsMsg{Repo: m.repo})
}

// openActionsOn opens the runs of the repository of msg that its filter
// selects, where it says, if the app has them, on any screen.
func (m *Model) openActionsOn(msg ui.OpenActionsMsg) tea.Cmd {
	if m.actions == nil || msg.Repo == (core.RepoRef{}) {
		return nil
	}
	mod, load := m.actions(m.ctx, msg.Repo, msg.Filter, msg.At)
	if mod == nil {
		return nil
	}
	return m.openLoading(mod, msg.Over, load)
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
	return m.openLoading(mod, m.modals.Top(), load)
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
	return m.openLoading(mod, m.modals.Top(), load)
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
	return m.openLoading(mod, nil, load)
}

// canOpenActions reports whether the actions key does something.
func (m *Model) canOpenActions() bool {
	return m.actions != nil && m.screen == repoScreen && m.repo != (core.RepoRef{})
}

// minTitleWidth is the least of a long title that the tabs of a modal
// leave in the top edge of its frame.
const minTitleWidth = 16

// frame draws mod in its frame, with its title in the top edge, and its
// tabs, if it has them and they fit, at the right end of it. The tabs
// shorten a long title, down to minTitleWidth, so they show at 80 columns.
func (m *Model) frame(mod ui.Modal) string {
	w, _ := m.modals.FrameSize()
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
