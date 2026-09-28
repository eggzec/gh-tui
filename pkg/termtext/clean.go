// Package termtext makes untrusted text, such as a file or a comment read
// from GitHub, safe to draw in a terminal.
package termtext

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Clean expands tabs to spaces and replaces control characters, the
// characters that reorder text, the kitty image placeholder and invalid
// UTF-8, so that the text can neither break the layout, nor send escape
// sequences to the terminal, nor show in an order other than the one it
// is read in, nor draw an image. It drops the CR of
// CRLF line endings. Text that needs none of this is returned as it is.
func Clean(src string, tabWidth int) string {
	s, _ := clean(src, tabWidth, false)
	return s
}

// CleanStyled cleans src as Clean does, but for its escape sequences: it
// keeps the SGR sequences, which only color text, as the styles of the
// text it returns, and drops the others whole, such as those that move
// the cursor or set the title. It is for text with colors of its own,
// such as a program's output kept in a file ([HasSGR]).
func CleanStyled(src string, tabWidth int) (string, []Style) {
	return clean(src, tabWidth, true)
}

func clean(src string, tabWidth int, styled bool) (string, []Style) {
	var (
		b      strings.Builder
		styles Styler
	)
	start := firstChange(src)
	if start == len(src) {
		// Most text needs nothing changed, and costs no copy.
		return src, nil
	}
	b.Grow(len(src))
	b.WriteString(src[:start])
	if styled {
		// At most one style for each escape, and no growing to get there.
		styles.Grow(strings.Count(src[start:], "\x1b"))
	}
	// col is the column at byte mark of the output, which is where the
	// last tab or line ended; the columns after it are measured lazily.
	mark, col := strings.LastIndexByte(src[:start], '\n')+1, 0
	for i := start; i < len(src); {
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
		case styled && r == ansi.ESC:
			n, sgr := Escape(src[i:])
			if sgr {
				styles.Add(b.Len(), src[i:i+n])
			}
			size = n
		case r == utf8.RuneError && size == 1, Control(r):
			b.WriteRune(utf8.RuneError)
		default:
			b.WriteString(src[i : i+size])
		}
		i += size
	}
	return b.String(), styles.Styles()
}

// firstChange returns the index of the first byte of src that cleaning
// changes, or len(src) if it changes none.
func firstChange(src string) int {
	for i := 0; i < len(src); {
		c := src[i]
		if c < utf8.RuneSelf {
			if c < 0x20 && c != '\n' || c == 0x7f {
				return i
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(src[i:])
		if r == utf8.RuneError && size == 1 || Control(r) {
			return i
		}
		i += size
	}
	return len(src)
}

// Placeholder is the kitty graphics protocol's Unicode placeholder. A
// terminal that knows it draws a part of an image in its cell, the image
// its foreground color names, so text from outside must not hold it.
const Placeholder = '\U0010EEEE'

// Control reports whether r is a control character, C0, DEL or C1, one of
// the characters that change the order text shows in, which can make code
// read differently from how it runs, or the kitty image [Placeholder].
func Control(r rune) bool {
	switch {
	case r == Placeholder:
		return true
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
