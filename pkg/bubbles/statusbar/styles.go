package statusbar

import (
	"cmp"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Styles holds the styles of a status bar. The items come styled.
type Styles struct {
	// Separator styles the SeparatorText between two items on the right.
	Separator lipgloss.Style
	// SeparatorText goes between two items on the right. The default,
	// which an empty text keeps, is " · ".
	SeparatorText string
}

// DefaultStyles returns the styles for a light or dark background: a
// faint separator.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	return Styles{
		Separator:     lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#C4C4C4"), lipgloss.Color("#4A4A4A"))),
		SeparatorText: " · ",
	}
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.sep = s.Separator.Render(cmp.Or(s.SeparatorText, " · "))
	m.sepWidth = ansi.StringWidth(m.sep)
	m.layout()
}
