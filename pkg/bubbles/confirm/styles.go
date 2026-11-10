package confirm

import "charm.land/lipgloss/v2"

// Styles style the box of an [Ask].
type Styles struct {
	Question, Keys lipgloss.Style
	// Border styles the border, Title the word on it and Selected the choice
	// that enter gives.
	Border, Title, Selected lipgloss.Style
	// Danger, DangerTitle and DangerSelected stand in for Border, Title and
	// Selected on a destructive question.
	Danger, DangerTitle, DangerSelected lipgloss.Style
	// Ellipsis ends a question cut to its room; empty, it is "…".
	Ellipsis string
	// Separator joins the answers to the default; empty, it is " · ".
	Separator string
	// Key names the default key in the words of an icon set, such as
	// "enter" for the arrow in ASCII; nil leaves it as help shows it.
	Key func(help string) string
	// Glyphs draws the border; the zero value is the rounded one.
	Glyphs lipgloss.Border
}

// glyphs is the border of st, or the rounded one while it has none.
func (st Styles) glyphs() lipgloss.Border {
	if st.Glyphs == (lipgloss.Border{}) {
		return lipgloss.RoundedBorder()
	}
	return st.Glyphs
}

// DefaultStyles returns calm styles for a light or dark terminal: the
// question in the accent, and the keys that answer it muted.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	danger := lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#c0392b"), lipgloss.Color("#f7768e")))
	return Styles{
		Question:       lipgloss.NewStyle().Foreground(accent).Bold(true),
		Keys:           lipgloss.NewStyle().Foreground(muted),
		Border:         lipgloss.NewStyle().Foreground(accent),
		Title:          lipgloss.NewStyle().Foreground(accent).Bold(true),
		Selected:       lipgloss.NewStyle().Foreground(accent).Bold(true),
		Danger:         danger,
		DangerTitle:    danger.Bold(true),
		DangerSelected: danger.Bold(true),
	}
}

// ellipsis is the ellipsis of st, or the default while it has none.
func (st Styles) ellipsis() string {
	if st.Ellipsis == "" {
		return "…"
	}
	return st.Ellipsis
}

// separator is the separator of st, or the default while it has none.
func (st Styles) separator() string {
	if st.Separator == "" {
		return " · "
	}
	return st.Separator
}
