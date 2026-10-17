package diffview

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Mode is what a diff shows: the one file, or every file of the range.
type Mode int

const (
	// ThisFile shows the diff of one file.
	ThisFile Mode = iota
	// WholePatch shows the diff of every file in the range, one after another.
	WholePatch
)

// sep joins the parts of a title and of a status note.
const sep = " · "

// Title is the title of the diff: "Diff · label · path (i/n) · view · P%", or
// "Whole patch · ..." in the whole patch, where path is the file under the
// cursor. i and n are the file's place in tree order, left out when n is 0, and
// pct the cursor's position in percent. The title never gets wider than width: a long path is
// cut in the middle with "…", and the rest is cut at the end only when even
// that doesn't fit.
func Title(m Mode, label, path string, i, n int, view string, pct, width int) string {
	lead := "Diff"
	if m == WholePatch {
		lead = "Whole patch"
	}
	head := lead + sep + clean(label) + sep
	count := ""
	if n > 0 {
		// Without the list of files yet read there is no place to give.
		count = fmt.Sprintf(" (%d/%d)", i, n)
	}
	tail := fmt.Sprintf("%s%s%s%s%d%%", count, sep, clean(view), sep, min(max(pct, 0), 100))
	room := width - ansi.StringWidth(head) - ansi.StringWidth(tail)
	return ansi.Truncate(head+cutMiddle(clean(path), room)+tail, max(width, 0), "…")
}

// Note is the status note of the diff: "L<cur>/<total> · hint · hint".
// Styling is left to the caller.
func Note(cur, total int, hints []string) string {
	parts := make([]string, 0, 1+len(hints))
	parts = append(parts, fmt.Sprintf("L%d/%d", cur, total))
	parts = append(parts, hints...)
	return strings.Join(parts, sep)
}

// clean drops the escape sequences and control characters of text that came
// from a repository, so that a hostile name can't move the cursor or restyle
// the screen, and measures the same wherever it is drawn.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, ansi.Strip(s))
}

// cutMiddle shortens s to at most w cells by replacing its middle with "…",
// since the start of a path says where it is and its end what it is.
func cutMiddle(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := ansi.StringWidth(s)
	if sw <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	keep := w - 1
	right := keep / 2
	left := keep - right
	return ansi.Truncate(s, left, "") + "…" + ansi.TruncateLeft(s, sw-right, "")
}
