// Package termtext makes untrusted text, such as a file or a comment read
// from GitHub, safe to draw in a terminal.
package termtext

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Clean expands tabs to spaces and replaces control characters, the
// characters that reorder text and invalid UTF-8, so that the text can
// neither break the layout, nor send escape sequences to the terminal, nor
// show in an order other than the one it is read in. It drops the CR of
// CRLF line endings.
func Clean(src string, tabWidth int) string {
	var b strings.Builder
	b.Grow(len(src))
	// col is the column at byte mark of the output, which is where the
	// last tab or line ended; the columns after it are measured lazily.
	mark, col := 0, 0
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		switch {
		case r == '\n':
			b.WriteByte('\n')
			mark, col = b.Len(), 0
		case r == '\t':
			col += ansi.StringWidth(b.String()[mark:])
			n := tabWidth - col%tabWidth
			for range n {
				b.WriteByte(' ')
			}
			mark, col = b.Len(), col+n
		case r == '\r' && strings.HasPrefix(src[i+size:], "\n"):
		case r == utf8.RuneError && size == 1, Control(r):
			b.WriteRune(utf8.RuneError)
		default:
			b.WriteString(src[i : i+size])
		}
		i += size
	}
	return b.String()
}

// Control reports whether r is a control character, C0, DEL or C1, or one
// of the characters that change the order text shows in, which can make
// code read differently from how it runs.
func Control(r rune) bool {
	switch {
	case r < 0x20, r >= 0x7f && r < 0xa0:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		// Embeddings, overrides and isolates.
		return true
	case r == 0x200e, r == 0x200f, r == 0x61c:
		// Directional marks.
		return true
	}
	return false
}
