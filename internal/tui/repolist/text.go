package repolist

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// truncate cuts plain text s to width cells, ending in tail, an ellipsis,
// where it cuts.
func truncate(s string, width int, tail string) string {
	if width <= 0 {
		return ""
	}
	return termtext.Truncate(s, width, tail)
}

// wrap breaks plain text s into at most n lines of width cells at spaces,
// and cuts the last from the middle, joined by tail, an ellipsis, if s
// doesn't fit.
func wrap(s string, width, n int, tail string) []string {
	if s == "" || width <= 0 || n <= 0 {
		return nil
	}
	lines := strings.Split(ansi.Wordwrap(s, width, ""), "\n")
	if len(lines) > n {
		last := strings.Join(lines[n-1:], " ")
		lines = append(lines[:n-1], termtext.TruncateMiddle(last, width, tail))
	}
	for i := range lines {
		lines[i] = truncate(lines[i], width, tail)
	}
	return lines
}
