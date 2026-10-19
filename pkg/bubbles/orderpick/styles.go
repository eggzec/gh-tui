package orderpick

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Styles holds the styles of an ordered picker.
type Styles struct {
	// Frame surrounds the pane that is not in focus, and FocusFrame the one
	// that is. Their border and padding are taken from the size; leave their
	// width and height unset.
	Frame      lipgloss.Style
	FocusFrame lipgloss.Style
	// Picker styles the left pane. Its Frame is replaced by this Frame or
	// FocusFrame, and its CursorGlyph by a blank while the pane is not in
	// focus.
	Picker picker.Styles
	// Heading styles the title of the right pane, and Status the count under
	// it.
	Heading lipgloss.Style
	Status  lipgloss.Style
	// Number styles the place of a chosen item.
	Number lipgloss.Style
	// Title styles the name of a chosen item, and SelectedTitle that of the
	// item under the cursor.
	Title         lipgloss.Style
	SelectedTitle lipgloss.Style
	// Direction styles the direction of a chosen item.
	Direction lipgloss.Style
	// Empty styles the text shown while nothing is chosen.
	Empty lipgloss.Style
	// Gutter styles the mark of the item under the cursor, and CursorGlyph is
	// that mark, cut or padded to one cell.
	Gutter      lipgloss.Style
	CursorGlyph string
	// Ellipsis ends a line cut to the width.
	Ellipsis string
}

// DefaultStyles returns calm styles for a light or dark terminal, with the
// accent color on the pane in focus, the cursor and the matches.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	text := ld(lipgloss.Color("#1f2330"), lipgloss.Color("#c8cedb"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))

	ps := picker.DefaultStyles(isDark)
	return Styles{
		Frame:         ps.Frame,
		FocusFrame:    ps.Frame.BorderForeground(accent),
		Picker:        ps,
		Heading:       lipgloss.NewStyle().Foreground(muted).Bold(true),
		Status:        lipgloss.NewStyle().Foreground(subtle),
		Number:        lipgloss.NewStyle().Foreground(subtle),
		Title:         lipgloss.NewStyle().Foreground(text),
		SelectedTitle: lipgloss.NewStyle().Foreground(text).Bold(true),
		Direction:     lipgloss.NewStyle().Foreground(subtle),
		Empty:         lipgloss.NewStyle().Foreground(muted),
		Gutter:        lipgloss.NewStyle().Foreground(accent),
		CursorGlyph:   "▌",
		Ellipsis:      "…",
	}
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles, and passes them on to the left pane.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.gutter = s.Gutter.Render(termtext.Cells(s.CursorGlyph, 1)) + " "
	m.restyle()
}

// restyle gives the left pane the frame and the cursor its focus calls for.
func (m *Model) restyle() {
	ps := m.styles.Picker
	if m.pane == PaneAvailable {
		ps.Frame = m.styles.FocusFrame
	} else {
		ps.Frame = m.styles.Frame
		ps.CursorGlyph = ""
	}
	m.left.SetStyles(ps)
}
