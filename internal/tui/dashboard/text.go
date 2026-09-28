package dashboard

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fit pads or cuts s to width cells.
func fit(s string, width int) string {
	w := ansi.StringWidth(s)
	switch {
	case w == width:
		return s
	case w < width:
		return s + strings.Repeat(" ", width-w)
	}
	return ansi.Truncate(s, width, "")
}

// spread puts left and right at the edges of width cells, and drops right
// when both don't fit.
func spread(left, right string, width int) string {
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	if right == "" || lw+rw+1 > width {
		return fit(ansi.Truncate(left, width, "…"), width)
	}
	return left + strings.Repeat(" ", width-lw-rw) + right
}

// truncate cuts plain text s to width cells, with an ellipsis.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

// wrap breaks plain text s into at most n lines of width cells at spaces,
// and ends the last with an ellipsis if s doesn't fit.
func wrap(s string, width, n int) []string {
	if s == "" || width <= 0 || n <= 0 {
		return nil
	}
	lines := strings.Split(ansi.Wordwrap(s, width, ""), "\n")
	if len(lines) > n {
		last := strings.Join(lines[n-1:], " ")
		lines = append(lines[:n-1], truncate(last, width))
	}
	for i := range lines {
		lines[i] = truncate(lines[i], width)
	}
	return lines
}

// cleanLine puts text from GitHub on one line without escape sequences or
// controls, so it can't break the layout.
func cleanLine(s string) string {
	return strings.Join(strings.Fields(ui.OneLine(s)), " ")
}

// count formats n the way GitHub shows counts: 999, 1.2k, 12k.
func count(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 10_000:
		s := strconv.FormatFloat(float64(n)/1000, 'f', 1, 64)
		return strings.TrimSuffix(s, ".0") + "k"
	case n < 1_000_000:
		return strconv.Itoa(n/1000) + "k"
	}
	return strconv.Itoa(n/1_000_000) + "m"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func itoa(n int) string { return strconv.Itoa(n) }
