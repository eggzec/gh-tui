package ui

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
)

func rowTheme(t *testing.T) Theme {
	t.Helper()
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	return NewTheme(p, true)
}

// Every cell of the selected row is bold, its name takes the accent in a
// focused pane only, and a row that isn't selected is not bold.
func TestRowPaints(t *testing.T) {
	t.Parallel()
	th := rowTheme(t)
	rows := NewRowPaints(th)
	bold := func(p Paint) bool { return strings.HasPrefix(p.Render("x"), "\x1b[1") }
	cells := func(r RowPaints) []Paint { return []Paint{r.Title, r.Text, r.Muted, r.Subtle, r.Accent} }

	for _, look := range []int{LookBlurred, LookFocused} {
		for i, p := range cells(rows[look]) {
			if !bold(p) {
				t.Errorf("look %d cell %d = %q, want bold", look, i, p.Render("x"))
			}
		}
	}
	for i, p := range cells(rows[LookPlain]) {
		if bold(p) && i != 0 {
			t.Errorf("plain cell %d = %q, want not bold", i, p.Render("x"))
		}
	}
	if got, want := rows[LookPlain].Title.Render("x"), NewPaint(th.Text).Render("x"); got != want {
		t.Errorf("plain title = %q, want %q", got, want)
	}
	if got, want := rows[LookFocused].Title.Render("x"), NewPaint(th.Pick).Render("x"); got != want {
		t.Errorf("focused title = %q, want the accent %q", got, want)
	}
	if got, want := rows[LookBlurred].Title.Render("x"), NewPaint(th.Title).Render("x"); got != want {
		t.Errorf("blurred title = %q, want bold only %q", got, want)
	}
}

// Bold keeps the color of a paint.
func TestPaintBold(t *testing.T) {
	t.Parallel()
	th := rowTheme(t)
	p := NewPaint(th.Muted)
	if got, want := p.Bold().Render("x"), NewPaint(th.Cell(th.Muted, true)).Render("x"); got != want {
		t.Errorf("Bold() = %q, want %q", got, want)
	}
}
