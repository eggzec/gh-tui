package ownerui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Digits is the strip of pane keys for the bottom edge of a frame, like the
// pane strip of the pull request modal: the keys in order, the one of pane
// cur in brackets and painted on, the others off. A pane with no key ("")
// is left out, and when pane cur has no key, no key is in brackets. It returns the strip and its width, or "" when no pane has a
// key.
func Digits(keys []string, cur int, on, off ui.Paint) (strip string, width int) {
	return strip1(keys, nil, cur, on, off)
}

// Foot is the strip of pane keys with the title of each pane after its key,
// "1 Pinned  [2] Repositories  3 Waiting on you", for the bottom edge of a
// zoomed pane. It tries each of tiers, a list of titles by pane, from the
// first, and keeps the first strip no wider than maxW; when none fits, it is
// the keys alone, as Digits makes them.
func Foot(keys []string, tiers [][]string, cur int, on, off ui.Paint, maxW int) (strip string, width int) {
	for _, titles := range tiers {
		if s, w := strip1(keys, titles, cur, on, off); w <= maxW {
			return s, w
		}
	}
	return Digits(keys, cur, on, off)
}

// strip1 writes one strip: each key, with the title of its pane after it
// when titles has one, the one of pane cur in brackets.
func strip1(keys, titles []string, cur int, on, off ui.Paint) (strip string, width int) {
	var b strings.Builder
	n := 0
	sep := " "
	if titles != nil {
		sep = "  "
	}
	for i, k := range keys {
		if k == "" {
			continue
		}
		if n > 0 {
			b.WriteString(sep)
			n += len(sep)
		}
		if i == cur {
			k = "[" + k + "]"
		}
		if i < len(titles) && titles[i] != "" {
			k += " " + titles[i]
		}
		if i == cur {
			on.Write(&b, k)
		} else {
			off.Write(&b, k)
		}
		n += ansi.StringWidth(k)
	}
	return b.String(), n
}

// Beside appends to lines those of left, each with the line of right
// beside it.
func Beside(lines, left, right []string) []string {
	for i, l := range left {
		if i < len(right) {
			l += right[i]
		}
		lines = append(lines, l)
	}
	return lines
}

// Indent puts a space before each of lines, as the lines of a pane start.
func Indent(lines []string) []string {
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	return lines
}
