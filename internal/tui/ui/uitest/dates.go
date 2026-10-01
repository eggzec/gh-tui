package uitest

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// DateLayout is a date format wider than any age, so a view that keeps
// room for ages alone overflows with it.
const DateLayout = "Jan _2 2006"

// DateFormat returns the defaults with ui.date_format set to DateLayout,
// as the set command leaves the config.
func DateFormat(tb testing.TB) config.Config {
	tb.Helper()
	c, err := config.Default().Set("ui.date_format", DateLayout)
	if err != nil {
		tb.Fatal(err)
	}
	return c
}

// Dated checks that view, drawn after the date format was set to
// DateLayout, tells each of times in it, and that each of its lines still
// fits in width cells.
func Dated(tb testing.TB, view string, width int, times ...time.Time) {
	tb.Helper()
	plain := ansi.Strip(view)
	d := ui.NewDates(DateLayout)
	for _, t := range times {
		if want := d.Date(t, ""); !strings.Contains(plain, want) {
			tb.Errorf("the view doesn't tell %v as %q:\n%s", t, want, plain)
		}
	}
	for i, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > width {
			tb.Errorf("line %d takes %d cells, more than %d:\n%s", i, w, width, ansi.Strip(line))
		}
	}
}
