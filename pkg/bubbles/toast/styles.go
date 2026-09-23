package toast

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// LevelStyle is how one level looks: a glyph one cell wide and the status
// color of the glyph and the toast's edge.
type LevelStyle struct {
	Glyph string
	Color color.Color
}

// Styles holds the styles of a toast stack.
type Styles struct {
	// Toast frames each toast. Its background fills the whole toast and its
	// left border takes the level's color.
	Toast lipgloss.Style
	// Text is the message.
	Text lipgloss.Style
	// Count is the repeat count, such as "×3".
	Count lipgloss.Style

	Info, Success, Warning, Error LevelStyle
}

// DefaultStyles returns calm styles for a light or dark background: a
// subtle panel, and color only on the glyph and the edge.
func DefaultStyles(isDark bool) Styles {
	c := lipgloss.LightDark(isDark)
	panel := c(lipgloss.Color("#EEF1F4"), lipgloss.Color("#1F242C"))
	return Styles{
		Toast: lipgloss.NewStyle().
			Background(panel).
			Padding(0, 1).
			Border(lipgloss.Border{Left: "▌"}, false, false, false, true).
			BorderBackground(panel),
		Text:    lipgloss.NewStyle().Foreground(c(lipgloss.Color("#1F2328"), lipgloss.Color("#E6EDF3"))),
		Count:   lipgloss.NewStyle().Foreground(c(lipgloss.Color("#59636E"), lipgloss.Color("#9198A1"))),
		Info:    LevelStyle{Glyph: "•", Color: c(lipgloss.Color("#0969DA"), lipgloss.Color("#58A6FF"))},
		Success: LevelStyle{Glyph: "✓", Color: c(lipgloss.Color("#1A7F37"), lipgloss.Color("#3FB950"))},
		Warning: LevelStyle{Glyph: "!", Color: c(lipgloss.Color("#9A6700"), lipgloss.Color("#D29922"))},
		Error:   LevelStyle{Glyph: "✗", Color: c(lipgloss.Color("#CF222E"), lipgloss.Color("#F85149"))},
	}
}

func (s Styles) level(l Level) LevelStyle {
	switch l {
	case Success:
		return s.Success
	case Warning:
		return s.Warning
	case Error:
		return s.Error
	default:
		return s.Info
	}
}

// derivedStyles are computed once in SetStyles so that rendering never
// builds a style.
type derivedStyles struct {
	// frame and glyph are indexed by Level.
	frame [Error + 1]lipgloss.Style
	glyph [Error + 1]lipgloss.Style
	// text and count carry the toast's background, because the reset after
	// each styled span would otherwise clear it.
	text, count lipgloss.Style
	frameWidth  int
	glyphWidth  int
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles sets the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	bg := s.Toast.GetBackground()
	d := derivedStyles{
		text:       s.Text.Background(bg),
		count:      s.Count.Background(bg),
		frameWidth: s.Toast.GetHorizontalFrameSize(),
	}
	for l := Info; l <= Error; l++ {
		ls := s.level(l)
		d.frame[l] = s.Toast.BorderForeground(ls.Color)
		d.glyph[l] = lipgloss.NewStyle().Foreground(ls.Color).Background(bg).Bold(true)
		d.glyphWidth = max(d.glyphWidth, lipgloss.Width(ls.Glyph))
	}
	m.derived = d
	m.changed()
}
