package termimg

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi/kitty"
)

// MaxCells is how many rows or columns an image may span at most: one for
// each diacritic that can name a row or column.
const MaxCells = 297

// Rows returns the lines of placeholder cells that show image id, cols by
// rows, each cols cells wide, for a placement of the same size ([Place]).
// Each cell holds U+10EEEE and the diacritics of its row, its column and
// the high byte of id, all of them, so a line cut short or covered in part
// still shows the right part of the image. Each line sets the foreground
// to id's color index and resets it at its end. The lines must reach the
// terminal as they are: a style that reverses them or sets their
// foreground, or wraps them, loses the image. cols and rows are clamped
// to 1 through [MaxCells].
func Rows(id ID, cols, rows int) []string {
	cols, rows = clamp(cols), clamp(rows)
	fg := "\x1b[38;5;" + strconv.Itoa(int(id.Color())) + "m"
	msb := kitty.Diacritic(int(id.MSB()))
	lines := make([]string, rows)
	var b strings.Builder
	for r := range rows {
		b.Grow(len(fg) + cols*cellBytes + len(fgReset))
		b.WriteString(fg)
		row := kitty.Diacritic(r)
		for c := range cols {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(row)
			b.WriteRune(kitty.Diacritic(c))
			b.WriteRune(msb)
		}
		b.WriteString(fgReset)
		lines[r] = b.String()
		b.Reset()
	}
	return lines
}

// Cell reports what the placeholder cell at the start of s names: its
// row, its column and the high byte of the image's ID, and its length in
// bytes. ok is false if s doesn't start with a placeholder with all three
// diacritics.
func Cell(s string) (row, col int, msb uint8, n int, ok bool) {
	rs := []rune(s[:min(len(s), 4*4)])
	if len(rs) < 4 || rs[0] != kitty.Placeholder {
		return 0, 0, 0, 0, false
	}
	var idx [3]int
	for i := range idx {
		if idx[i] = diacriticIndex(rs[i+1]); idx[i] < 0 {
			return 0, 0, 0, 0, false
		}
	}
	if idx[2] > 255 {
		return 0, 0, 0, 0, false
	}
	return idx[0], idx[1], uint8(idx[2]), len(string(rs[:4])), true
}

// fgReset ends a row's foreground.
const fgReset = "\x1b[39m"

// cellBytes is the most bytes a cell takes: the placeholder and three
// diacritics of four bytes each.
const cellBytes = 4 * 4

// diacriticIndex returns the index of diacritic r, or -1 if it is none.
func diacriticIndex(r rune) int {
	return slices.Index(diacritics, r)
}

// diacritics is kitty's table, as kitty.Diacritic hands it out.
var diacritics = func() []rune {
	d := make([]rune, MaxCells)
	for i := range d {
		d[i] = kitty.Diacritic(i)
	}
	return d
}()

func clamp(n int) int { return max(1, min(n, MaxCells)) }
