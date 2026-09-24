package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// topModal returns the open modal, or nil if none is open.
func (m *Model) topModal() ui.Modal {
	return m.modal
}

// openModal opens mod in place of the open modal, if any: modals never
// stack, so the screen shows at most one frame over it.
func (m *Model) openModal(mod ui.Modal) {
	if mod == nil {
		return
	}
	mod.SetTheme(m.theme)
	mod.SetSize(m.modalSize())
	m.modal = mod
}

// isOpen reports whether mod is open.
func (m *Model) isOpen(mod ui.Modal) bool {
	return mod != nil && m.modal == mod
}

// openSearch opens the search modal, if the app has a search.
func (m *Model) openSearch() tea.Cmd {
	if m.search == nil {
		return nil
	}
	first := m.searchBox == nil
	if first {
		m.searchBox = newSearchModal(m.ctx, m.search)
	}
	m.openModal(m.searchBox)
	return m.searchBox.open(first)
}

// closeModal closes mod. A modal that isn't open is ignored, so closing
// twice, or closing one that another has replaced, is harmless.
func (m *Model) closeModal(mod ui.Modal) {
	if m.isOpen(mod) {
		m.modal = nil
	}
}

// frameSize is the size of a modal with its frame: most of the screen, so
// that the edges of the screen behind it stay in view.
func (m *Model) frameSize() (width, height int) {
	return max(m.width-2*max(m.width/10, 2), 0), max(m.height-2*max(m.height/10, 1), 0)
}

// modalSize is the size inside the frame: the frame takes a row at the top
// and bottom and two columns on each side, the edge and its padding.
func (m *Model) modalSize() (width, height int) {
	w, h := m.frameSize()
	return max(w-4, 0), max(h-2, 0)
}

// frame draws mod in its frame, with its title in the top edge.
func (m *Model) frame(mod ui.Modal) string {
	w, _ := m.frameSize()
	if w < 4 {
		return ""
	}
	b := lipgloss.RoundedBorder()
	title := ansi.Truncate(" "+mod.Title()+" ", max(w-4, 0), "… ")
	rest := max(w-3-lipgloss.Width(title), 0)
	top := m.theme.Accent.Render(b.TopLeft+b.Top) +
		m.theme.Title.Render(title) +
		m.theme.Accent.Render(strings.Repeat(b.Top, rest)+b.TopRight)
	return top + "\n" + m.theme.Frame().Render(mod.View())
}
