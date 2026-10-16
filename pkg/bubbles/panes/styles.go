package panes

import "charm.land/lipgloss/v2"

// Styles holds how the frames look. The styles only colour and weigh text:
// padding, margins or a width on one would break the widths of the frames.
type Styles struct {
	// Border is the glyphs of the frame. The default is rounded; use
	// lipgloss.ASCIIBorder for terminals without line drawing. Each glyph
	// is one cell wide and none is empty, so lipgloss.NormalBorder and its
	// like fit but a hidden or wide-glyph border does not.
	Border lipgloss.Border
	// Edge colours the border of an unfocused box, EdgeFocus the focused one.
	Edge, EdgeFocus lipgloss.Style
	// Title and TitleFocus style the title in the top border.
	Title, TitleFocus lipgloss.Style
	// Info styles the right-hand info of the top border.
	Info lipgloss.Style
	// StripCurrent styles the current pane in the pane strip, StripOther
	// the others and StripSep what separates them.
	StripCurrent, StripOther, StripSep lipgloss.Style
	// Ellipsis ends a line or title that was cut. The default is "…".
	Ellipsis string
}

// DefaultStyles returns calm styles for a light or dark background: a dim
// border that takes the accent colour when focused.
func DefaultStyles(isDark bool) Styles {
	c := lipgloss.LightDark(isDark)
	dim := c(lipgloss.Color("#818B98"), lipgloss.Color("#6E7681"))
	text := c(lipgloss.Color("#1F2328"), lipgloss.Color("#E6EDF3"))
	accent := c(lipgloss.Color("#0969DA"), lipgloss.Color("#58A6FF"))
	return Styles{
		Border:       lipgloss.RoundedBorder(),
		Edge:         lipgloss.NewStyle().Foreground(dim),
		EdgeFocus:    lipgloss.NewStyle().Foreground(accent),
		Title:        lipgloss.NewStyle().Foreground(text),
		TitleFocus:   lipgloss.NewStyle().Foreground(accent).Bold(true),
		Info:         lipgloss.NewStyle().Foreground(dim),
		StripCurrent: lipgloss.NewStyle().Foreground(accent).Bold(true),
		StripOther:   lipgloss.NewStyle().Foreground(dim),
		StripSep:     lipgloss.NewStyle().Foreground(dim),
		Ellipsis:     "…",
	}
}

func (s Styles) edge(focus bool) lipgloss.Style {
	if focus {
		return s.EdgeFocus
	}
	return s.Edge
}
