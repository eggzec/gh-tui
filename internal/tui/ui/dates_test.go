package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
)

func TestDates(t *testing.T) {
	now := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	then := now.Add(-50 * time.Hour)
	tests := []struct {
		format             string
		short, prose, date string
		width              int
	}{
		{"", "2d", "2d ago", "2026-09-26 10:00 UTC", 4},
		{config.DateRelative, "2d", "2d ago", "2026-09-26 10:00 UTC", 4},
		{config.DateAbsolute, "2026-09-26 10:00 UTC", "2026-09-26 10:00 UTC", "2026-09-26 10:00 UTC", 20},
		{"Jan _2 15:04", "Sep 26 10:00", "Sep 26 10:00", "Sep 26 10:00", 12},
		{"Monday 2 January", "Saturday 26 September", "Saturday 26 September", "Saturday 26 September", 22},
	}
	for _, tt := range tests {
		d := NewDates(tt.format).In(time.UTC)
		if got := d.Short(then, now); got != tt.short {
			t.Errorf("%q: Short = %q, want %q", tt.format, got, tt.short)
		}
		if got := d.Prose(then, now); got != tt.prose {
			t.Errorf("%q: Prose = %q, want %q", tt.format, got, tt.prose)
		}
		if got := d.Date(then, AbsoluteLayout); got != tt.date {
			t.Errorf("%q: Date = %q, want %q", tt.format, got, tt.date)
		}
		if got := d.Width(); got != tt.width {
			t.Errorf("%q: Width = %d, want %d", tt.format, got, tt.width)
		}
	}
}

// TestDatesWidth checks that no date is wider than Width says, through a
// year of days and hours, in a zone that changes with the seasons.
func TestDatesWidth(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no zone database:", err)
	}
	now := time.Date(2026, time.December, 31, 0, 0, 0, 0, loc)
	for _, format := range []string{config.DateRelative, config.DateAbsolute, "Mon Jan _2 3:04PM MST", "Monday, 2 January 2006", "1/2/06", "1/2/2006 3:04PM"} {
		d := NewDates(format).In(loc)
		w := d.Width()
		for h := range 366 * 24 {
			at := time.Date(2026, time.January, 1, 0, 0, 0, 0, loc).Add(time.Duration(h) * time.Hour)
			if got := ansi.StringWidth(d.Short(at, now)); got > w {
				t.Fatalf("%q: Short(%v) takes %d cells, more than Width %d", format, at, got, w)
			}
		}
	}
}
