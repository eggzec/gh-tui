package calendar

import "charm.land/lipgloss/v2"

// Levels is the number of intensity levels a day can have, from 0 for no
// contributions to Levels-1 for the most.
const Levels = 5

// DefaultGlyph is the glyph of a day. Terminals that draw it two cells wide,
// as some do with East Asian fonts, can use [WithGlyph] to pick another.
const DefaultGlyph = "■"

// Styles holds the styles of a calendar.
type Styles struct {
	// Levels color the days by their level, from none to the most.
	Levels [Levels]lipgloss.Style
	// Cursor is laid over the level style of the day under the cursor while
	// the calendar is focused.
	Cursor lipgloss.Style
	// Total styles the line with the total above the grid.
	Total lipgloss.Style
	// Month styles the month labels above the grid.
	Month lipgloss.Style
	// Weekday styles the weekday labels left of the grid.
	Weekday lipgloss.Style
	// Legend styles the words of the legend below the grid.
	Legend lipgloss.Style
	// Status styles the line about the day under the cursor.
	Status lipgloss.Style
	// Empty styles the text shown when there are no days.
	Empty lipgloss.Style
}

// DefaultStyles returns the default styles for a light or dark terminal: a
// green scale like the one on GitHub profiles.
func DefaultStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)
	muted := ld(lipgloss.Color("#545b6e"), lipgloss.Color("#a0a7b8"))
	subtle := ld(lipgloss.Color("#8a90a0"), lipgloss.Color("#6b7285"))

	level := func(light, dark string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(ld(lipgloss.Color(light), lipgloss.Color(dark)))
	}
	return Styles{
		Levels: [Levels]lipgloss.Style{
			// GitHub's empty dark cell is almost the page color, so it is
			// lighter here to stay visible on terminal backgrounds.
			level("#d8dce2", "#2d333b"),
			level("#9be9a8", "#0e4429"),
			level("#40c463", "#006d32"),
			level("#30a14e", "#26a641"),
			level("#216e39", "#39d353"),
		},
		Cursor:  lipgloss.NewStyle().Reverse(true),
		Total:   lipgloss.NewStyle(),
		Month:   lipgloss.NewStyle().Foreground(muted),
		Weekday: lipgloss.NewStyle().Foreground(muted),
		Legend:  lipgloss.NewStyle().Foreground(subtle),
		Status:  lipgloss.NewStyle().Foreground(muted),
		Empty:   lipgloss.NewStyle().Foreground(muted),
	}
}
