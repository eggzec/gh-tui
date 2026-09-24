package config

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
)

// Dashboard configures the dashboard, the screen the app opens on.
type Dashboard struct {
	// CalendarGlyph is the cell of a day in the contribution calendar. It
	// must be one cell wide; some terminals draw the default ■ two cells
	// wide, and "▪" or "#" suit them better.
	CalendarGlyph string `yaml:"calendar_glyph"`
}

// DefaultCalendarGlyph is the default Dashboard.CalendarGlyph.
const DefaultCalendarGlyph = "■"

func defaultDashboard() Dashboard {
	return Dashboard{CalendarGlyph: DefaultCalendarGlyph}
}

func (d Dashboard) validate() error {
	if ansi.StringWidth(d.CalendarGlyph) != 1 || len([]rune(d.CalendarGlyph)) != 1 {
		return fmt.Errorf(`dashboard.calendar_glyph: must be one character one cell wide, such as "■" or "#", got %q`, d.CalendarGlyph)
	}
	return nil
}
