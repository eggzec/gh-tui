package config

import (
	"errors"
	"fmt"
	"slices"

	"github.com/charmbracelet/x/ansi"
)

// Dashboard configures the dashboard, the screen the app opens on.
type Dashboard struct {
	// CalendarGlyph is the cell of a day in the contribution calendar. It
	// must be one cell wide; some terminals draw the default ■ two cells
	// wide, and "▪" or "#" suit them better.
	CalendarGlyph string `yaml:"calendar_glyph"`
	// Contributions is how far back the contribution calendar goes:
	// Contributions30d, Contributions90d or ContributionsYear. Its total
	// counts the days it shows.
	Contributions string `yaml:"contributions"`
	// Prefetch reads the pull requests and issues of Waiting on you ahead
	// while its pane has the focus: the first three rows of the list on
	// view, and the row the cursor rests on for details.prefetch.hover_delay,
	// so that they open at once. Each costs two requests, the detail and
	// its first comments; what is cached is skipped. It needs
	// details.prefetch.enabled.
	Prefetch bool `yaml:"prefetch"`
}

// DefaultCalendarGlyph is the default Dashboard.CalendarGlyph.
const DefaultCalendarGlyph = "■"

// Ranges of the contribution calendar.
const (
	Contributions30d  = "30d"
	Contributions90d  = "90d"
	ContributionsYear = "year"
)

func defaultDashboard() Dashboard {
	return Dashboard{CalendarGlyph: DefaultCalendarGlyph, Contributions: Contributions90d, Prefetch: true}
}

// DashboardPrefetch reports whether the dashboard reads the work waiting
// on the viewer ahead: it has its own switch, and reads details ahead as
// the lists do, so it needs theirs too.
func (c Config) DashboardPrefetch() bool {
	return c.Dashboard.Prefetch && c.Details.Prefetch.Enabled
}

// ContributionDays is the number of recent days the calendar shows, or 0
// for the year that GitHub reports.
func (d Dashboard) ContributionDays() int {
	switch d.Contributions {
	case Contributions30d:
		return 30
	case Contributions90d:
		return 90
	default:
		return 0
	}
}

func (d Dashboard) validate() error {
	var errs []error
	if ansi.StringWidth(d.CalendarGlyph) != 1 || len([]rune(d.CalendarGlyph)) != 1 {
		errs = append(errs, fmt.Errorf(`dashboard.calendar_glyph: must be one character one cell wide, such as "■" or "#", got %q`, d.CalendarGlyph))
	}
	if !slices.Contains([]string{Contributions30d, Contributions90d, ContributionsYear}, d.Contributions) {
		errs = append(errs, fmt.Errorf("dashboard.contributions: must be %s, %s or %s, got %q", Contributions30d, Contributions90d, ContributionsYear, d.Contributions))
	}
	return errors.Join(errs...)
}
