package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Seal cuts or pads line to width cells, and ends its styles, so that the
// next line starts plain.
func Seal(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line + "\x1b[m"
}

// ChipRow is a line of chips, as many as fit in width cells, and the count
// of the others, which more styles. widths holds the cell width of each chip.
// The line is sealed at width.
func ChipRow(chips []string, widths []int, width int, more func(text string) string) string {
	n := len(chips)
	for k := n; k >= 0; k-- {
		need := max(k-1, 0)
		for _, w := range widths[:k] {
			need += w
		}
		rest := "+" + strconv.Itoa(n-k)
		if k < n {
			need += len(rest)
			if k > 0 {
				need++
			}
		}
		if need > width && k > 0 {
			continue
		}
		line := strings.Join(chips[:k], " ")
		if k < n {
			if k > 0 {
				line += " "
			}
			line += more(rest)
		}
		return Seal(line, width)
	}
	return Seal("", width)
}
