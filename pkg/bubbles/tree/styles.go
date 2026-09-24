package tree

import "charm.land/lipgloss/v2"

// cursorGlyph marks the selected row in the gutter.
const cursorGlyph = "▌"

// guideGlyph draws one level of indentation.
const guideGlyph = "│"

// Styles holds the styles of a tree.
type Styles struct {
	// Cursor marks the selected row while the tree is focused.
	Cursor lipgloss.Style
	// BlurredCursor marks the selected row while the tree is blurred.
	BlurredCursor lipgloss.Style
	// Guide styles the indentation guides.
	Guide lipgloss.Style
	// Marker styles the ▸ and ▾ in front of branches.
	Marker lipgloss.Style
	// Branch styles the names of branches.
	Branch lipgloss.Style
	// Leaf styles the names of leaves.
	Leaf lipgloss.Style
	// Detail styles the detail at the right of a row.
	Detail lipgloss.Style
	// Spinner styles the spinner of a branch that is loading.
	Spinner lipgloss.Style
	// Loading styles the text shown while the top-level nodes load.
	Loading lipgloss.Style
	// Empty styles the text shown when there are no nodes.
	Empty lipgloss.Style
	// Error styles the message of a failed load.
	Error lipgloss.Style
	// Hint styles secondary text such as the retry key.
	Hint lipgloss.Style
}

// DefaultStyles returns the default styles for a light or dark terminal.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))
	errColor := ld(lipgloss.Color("#c0392b"), lipgloss.Color("#ef7d7d"))

	return Styles{
		Cursor:        lipgloss.NewStyle().Foreground(accent),
		BlurredCursor: lipgloss.NewStyle().Foreground(subtle),
		Guide:         lipgloss.NewStyle().Foreground(subtle),
		Marker:        lipgloss.NewStyle().Foreground(muted),
		Branch:        lipgloss.NewStyle().Bold(true),
		Leaf:          lipgloss.NewStyle(),
		Detail:        lipgloss.NewStyle().Foreground(subtle),
		Spinner:       lipgloss.NewStyle().Foreground(accent),
		Loading:       lipgloss.NewStyle().Foreground(muted),
		Empty:         lipgloss.NewStyle().Foreground(muted),
		Error:         lipgloss.NewStyle().Foreground(errColor),
		Hint:          lipgloss.NewStyle().Foreground(subtle),
	}
}
