package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/calendar"
)

func TestCalendarGlyph(t *testing.T) {
	t.Parallel()
	ic := Icons{Cell: "#"}
	def := config.Default().Dashboard.CalendarGlyph
	tests := []struct {
		name, glyph, want string
	}{
		{"default takes the cell of the icons", def, "#"},
		{"custom is kept", "*", "*"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CalendarGlyph(tc.glyph, ic); got != tc.want {
				t.Errorf("CalendarGlyph(%q) = %q, want %q", tc.glyph, got, tc.want)
			}
		})
	}
}

func TestSetContributions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		days int
		want string
	}{
		{"year", 0, "No contributions in the last year."},
		{"30 days", 30, "No contributions in the last 30 days."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cal := calendar.New()
			cal.SetSize(80, 12)
			SetContributions(&cal, core.Contributions{}, tc.days)
			if got := cal.View(); !strings.Contains(got, tc.want) {
				t.Errorf("view lacks %q:\n%s", tc.want, got)
			}
		})
	}

	t.Run("total and days", func(t *testing.T) {
		t.Parallel()
		date := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
		c := core.Contributions{Total: 42, Weeks: [][]core.ContributionDay{{{Date: date, Count: 3, Level: 2}}}}
		cal := calendar.New()
		SetContributions(&cal, c, 0)
		if got := cal.Total(); got != 42 {
			t.Errorf("Total() = %d, want 42", got)
		}
		w := cal.Weeks()
		if len(w) != 1 || len(w[0]) != 1 || w[0][0] != (calendar.Day{Date: date, Count: 3, Level: 2}) {
			t.Errorf("Weeks() = %v", w)
		}
	})
}
