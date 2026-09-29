package overlay

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// compose is how Place drew before: through the cell-based compositor of
// lipgloss, which parses the covered rows into cells. Place must draw the
// same cells.
func compose(bg, fg string, x, y int) string {
	x, y = max(x, 0), max(y, 0)
	rows := strings.Split(bg, "\n")
	if y >= len(rows) {
		return bg
	}
	end := min(y+lipgloss.Height(fg), len(rows))
	covered := strings.Join(rows[y:end], "\n")
	canvas := lipgloss.NewCanvas(lipgloss.Width(covered), end-y)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(covered),
		lipgloss.NewLayer(fg).X(x).Z(1),
	))
	var b strings.Builder
	for _, r := range rows[:y] {
		b.WriteString(r)
		b.WriteByte('\n')
	}
	b.WriteString(canvas.Render())
	for _, r := range rows[end:] {
		b.WriteByte('\n')
		b.WriteString(r)
	}
	return b.String()
}

// cells draws s into a buffer of width by height cells.
func cells(s string, width, height int) uv.ScreenBuffer {
	buf := uv.NewScreenBuffer(width, height)
	buf.Method = ansi.GraphemeWidth
	uv.NewStyledString(s).Draw(buf, buf.Bounds())
	return buf
}

// sameCells reports the first cell where a and b, drawn, differ.
func sameCells(a, b string) error {
	w := max(lipgloss.Width(a), lipgloss.Width(b))
	h := max(lipgloss.Height(a), lipgloss.Height(b))
	ca, cb := cells(a, w, h), cells(b, w, h)
	for y := range h {
		for x := range w {
			pa, pb := ca.CellAt(x, y), cb.CellAt(x, y)
			if pa == nil || pb == nil {
				if pa != pb {
					return fmt.Errorf("cell %d,%d: %v, want %v", x, y, pa, pb)
				}
				continue
			}
			if !pa.Equal(pb) {
				return fmt.Errorf("cell %d,%d: %+v, want %+v", x, y, *pa, *pb)
			}
		}
	}
	return nil
}

// rows of frames with styles, wide runes, and runs of styled and plain
// spaces, as the screens of the app have.
var pieces = []string{
	"abc", "你好", "  ", "·", "→ x",
	lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Render("blue"),
	lipgloss.NewStyle().Background(lipgloss.Color("#333333")).Render(" bg  "),
	lipgloss.NewStyle().Bold(true).Render("好bold"),
	ansi.SetHyperlink("https://github.com/cli/cli") + "link" + ansi.ResetHyperlink(),
	lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("#e0af68")).Render("u 好"),
}

func randomRows(r *rand.Rand, n, maxParts int) string {
	rows := make([]string, n)
	for i := range rows {
		var b strings.Builder
		for range r.IntN(maxParts) {
			b.WriteString(pieces[r.IntN(len(pieces))])
		}
		rows[i] = b.String()
	}
	return strings.Join(rows, "\n")
}

// Place draws what the compositor drew, cell for cell, over frames of all
// shapes: styles on both sides, wide runes cut by either edge, rows
// shorter than the box's column, and boxes cut at the frame's edges.
func TestPlaceDrawsWhatTheCompositorDrew(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	named := []struct{ bg, fg string }{
		{frame(30, 8), box("popup")},
		{strings.Repeat("你好", 10) + "\n" + strings.Repeat("你好", 10), "XYZ"},
		{frame(20, 3), "[你好]"},
	}
	for i := range 2000 {
		bg, fg := randomRows(r, 1+r.IntN(6), 8), box(randomRows(r, 1+r.IntN(3), 3))
		x, y := r.IntN(30)-2, r.IntN(8)-2
		if i < len(named) {
			bg, fg, x, y = named[i].bg, named[i].fg, 4, 1
		}
		if err := sameCells(Place(bg, fg, x, y), compose(bg, fg, x, y)); err != nil {
			t.Fatalf("Place(%q, %q, %d, %d): %v", bg, fg, x, y, err)
		}
	}
}
