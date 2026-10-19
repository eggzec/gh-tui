package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
)

// The selected row is bold in every cell, which keeps its color, and its
// title takes the accent in a focused pane only.
func TestSelectedRowStyles(t *testing.T) {
	t.Parallel()
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	th := NewTheme(p, true)

	bold := func(s lipgloss.Style) bool { return strings.HasPrefix(s.Render("x"), "\x1b[1") }
	if !bold(th.Cell(th.Muted, true)) || bold(th.Cell(th.Muted, false)) {
		t.Errorf("a cell is bold only in the selected row: selected %q, other %q",
			th.Cell(th.Muted, true).Render("x"), th.Cell(th.Muted, false).Render("x"))
	}

	if got, want := th.RowTitle(false, true).Render("x"), th.Text.Render("x"); got != want {
		t.Errorf("title of another row = %q, want %q", got, want)
	}
	if got, want := th.RowTitle(true, true).Render("x"), th.Pick.Render("x"); got != want {
		t.Errorf("title of the selected row in a focused pane = %q, want %q", got, want)
	}
	blurred := th.RowTitle(true, false).Render("x")
	if want := th.Title.Render("x"); blurred != want {
		t.Errorf("title of the selected row in a blurred pane = %q, want %q", blurred, want)
	}
	if !bold(th.Pick) || th.Pick.Render("x") == th.Title.Render("x") {
		t.Errorf("pick = %q, want bold in the accent, not the title's color", th.Pick.Render("x"))
	}
}
