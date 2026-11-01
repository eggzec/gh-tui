package confirm

import "charm.land/lipgloss/v2"

// Styles style the line of an [Ask].
type Styles struct {
	Question, Keys lipgloss.Style
	// DefaultKey styles the key that gives the default answer.
	DefaultKey lipgloss.Style
	// Ellipsis ends a question cut to its room; empty, it is "…".
	Ellipsis string
	// Separator joins the answers to the default; empty, it is " · ".
	Separator string
	// Key names the default key in the words of an icon set, such as
	// "enter" for the arrow in ASCII; nil leaves it as help shows it.
	Key func(help string) string
}

// DefaultStyles returns calm styles for a light or dark terminal: the
// question in the accent, and the keys that answer it muted.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	accent := ld(lipgloss.Color("#3b63c4"), lipgloss.Color("#7aa2f7"))
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	return Styles{
		Question:   lipgloss.NewStyle().Foreground(accent).Bold(true),
		Keys:       lipgloss.NewStyle().Foreground(muted),
		DefaultKey: lipgloss.NewStyle().Foreground(accent),
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
