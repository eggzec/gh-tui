package ownerui

import (
	"strconv"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
)

// CalendarGlyph returns the glyph of a day in a contribution calendar:
// glyph, as dashboard.calendar_glyph sets it, or the cell of the icons
// while it is the default, so the ASCII set draws one of its own.
func CalendarGlyph(glyph string, ic ui.Icons) string {
	if glyph == config.Default().Dashboard.CalendarGlyph {
		return ic.Cell
	}
	return glyph
}

// SetContributions shows the contributions c in cal, whose range is the
// last days days, or the year for 0.
func SetContributions(cal *calendar.Model, c core.Contributions, days int) {
	weeks := make([][]calendar.Day, len(c.Weeks))
	for i, w := range c.Weeks {
		weeks[i] = make([]calendar.Day, len(w))
		for j, d := range w {
			weeks[i][j] = calendar.Day{Date: d.Date, Count: d.Count, Level: d.Level}
		}
	}
	period := "the last year"
	if days > 0 {
		period = "the last " + strconv.Itoa(days) + " days"
	}
	cal.SetEmptyText("No contributions in " + period + ".")
	cal.SetWeeks(weeks)
	// The total GitHub reports is for the year, and a range counts its
	// own days instead.
	cal.SetTotal(c.Total)
}
