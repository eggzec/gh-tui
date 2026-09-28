package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// topModal returns the open modal, or nil if none is open.
func (m *Model) topModal() ui.Modal {
	return m.modal
}

// openModal opens mod in place of the open modal, if any, which hides:
// modals never stack, so the screen shows at most one frame over it. It
// closes the command line, which runs no command over a modal, such as when
// a goto run before ends while the user types another.
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
	m.modal = mod
	mod.SetTheme(m.theme)
	mod.SetSize(m.modalSize())
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
	if sameRepo(msg.Repo, m.repo) {
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
	width, height = max(m.width-2*max(m.width/10, 2), 0), max(m.height-2*max(m.height/10, 1), 0)
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
	b := lipgloss.RoundedBorder()
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
	title := ansi.Truncate(" "+ui.OneLine(mod.Title())+" ", titleWidth, "… ")
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
	return top + "\n" + m.theme.Frame().Render(mod.View())
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
			b.WriteString(m.theme.Subtle.Render(" · "))
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
