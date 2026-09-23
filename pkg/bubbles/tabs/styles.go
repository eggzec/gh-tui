package tabs

import "charm.land/lipgloss/v2"

// Styles holds the styles of the tab bar.
type Styles struct {
	// Tab and Active style the titles of inactive tabs and the active tab.
	Tab    lipgloss.Style
	Active lipgloss.Style
	// Badge and ActiveBadge style the small text next to a title, such as a
	// count of unread notifications.
	Badge       lipgloss.Style
	ActiveBadge lipgloss.Style
	// Rule styles the thin line under the bar, and Indicator the part of it
	// under the active tab.
	Rule      lipgloss.Style
	Indicator lipgloss.Style
	// RuleChar and IndicatorChar are the characters the rule is drawn with.
	RuleChar      string
	IndicatorChar string
	// Ellipsis ends titles that were shortened to fit.
	Ellipsis string
}

// DefaultStyles returns the default styles for a dark or light background.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#5A4FCF"), lipgloss.Color("#A89BFF"))
	muted := ld(lipgloss.Color("#6B6B6B"), lipgloss.Color("#9C9C9C"))
	subtle := ld(lipgloss.Color("#8E8E8E"), lipgloss.Color("#767676"))
	rule := ld(lipgloss.Color("#D4D4D4"), lipgloss.Color("#3C3C3C"))
	return Styles{
		Tab:           lipgloss.NewStyle().Foreground(muted),
		Active:        lipgloss.NewStyle().Foreground(accent).Bold(true),
		Badge:         lipgloss.NewStyle().Foreground(subtle),
		ActiveBadge:   lipgloss.NewStyle().Foreground(accent),
		Rule:          lipgloss.NewStyle().Foreground(rule),
		Indicator:     lipgloss.NewStyle().Foreground(accent),
		RuleChar:      "─",
		IndicatorChar: "━",
		Ellipsis:      "…",
	}
}
