package tabrow

import (
	"charm.land/lipgloss/v2"
)

// Styles holds the styles of a tab row.
type Styles struct {
	// Current styles the tab on view.
	Current lipgloss.Style
	// Other styles the tabs not on view.
	Other lipgloss.Style
	// Marker styles the separators and the scroll markers.
	Marker lipgloss.Style
}

// DefaultStyles returns the styles for a light or dark background: the tab
// on view bold in the accent, the others muted.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	return Styles{
		Current: lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#5A56E0"), lipgloss.Color("#7D79FF"))),
		Other:   lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#6B6B6B"), lipgloss.Color("#9B9B9B"))),
		Marker:  lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#C4C4C4"), lipgloss.Color("#4A4A4A"))),
	}
}
