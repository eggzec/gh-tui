package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// topModal returns the modal opened last, or nil if none is open.
func (m *Model) topModal() ui.Modal {
	if len(m.modals) == 0 {
		return nil
	}
	return m.modals[len(m.modals)-1]
}

func (m *Model) openModal(mod ui.Modal) {
	if mod == nil {
		return
	}
	mod.SetTheme(m.theme)
	mod.SetSize(m.modalSize())
	m.modals = append(m.modals, mod)
}

// isOpen reports whether mod is open.
func (m *Model) isOpen(mod ui.Modal) bool {
	return slices.Contains(m.modals, mod)
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

// closeModal closes mod and the modals opened over it. A modal that isn't
// open is ignored, so closing twice is harmless.
func (m *Model) closeModal(mod ui.Modal) {
	for i, open := range m.modals {
		if open == mod {
			clear(m.modals[i:])
			m.modals = m.modals[:i]
			return
		}
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
