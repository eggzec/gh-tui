package termtext

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// OneLine joins the lines of s, and drops the escape sequences and the
// control characters that would break a row or command the terminal.
// Invalid UTF-8 turns into U+FFFD, since terminals and the parsers of
// escape sequences on the way to them may read a byte of it, such as
// 0x9b, as a C1 control. OneLine drops the invisible format characters
// too, such as the bidi controls, which can make text read other than it
// is, like a right-to-left override in a name from GitHub; the zero-width
// joiner and non-joiner stay, since emoji and scripts need them. The
// kitty image [Placeholder] turns into U+FFFD, so the text draws no image.
func OneLine(s string) string {
	valid := utf8.ValidString(s)
	if valid && !strings.ContainsFunc(s, func(r rune) bool { return isControl(r) || isMapped(r) }) {
		return s
	}
	if !valid {
		// Before Strip, which keeps the bytes of C1 that follow an
		// unfinished lead byte, such as the 0x9b of "\xe2\x9b2J".
		s = strings.ToValidUTF8(s, "\ufffd")
	}
	s = ansi.Strip(s)
	if strings.ContainsFunc(s, isMapped) {
		s = strings.Map(func(r rune) rune {
			switch {
			case r == Placeholder:
				return utf8.RuneError
			case Hidden(r):
				return -1
			}
			return r
		}, s)
	}
	return strings.Join(strings.FieldsFunc(s, isControl), " ")
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0
}

// isMapped reports whether OneLine drops or replaces r, which is no control.
func isMapped(r rune) bool {
	return r == Placeholder || Hidden(r)
}

// Hidden reports whether r is an invisible format character, which text
// from outside may hold to hide or reorder what it shows, and OneLine
// drops: any of Unicode's category Cf, such as U+202E or U+2066, but the
// zero-width joiner and non-joiner, which join emoji and scripts.
func Hidden(r rune) bool {
	return r >= 0xad && r != '\u200c' && r != '\u200d' && unicode.Is(unicode.Cf, r)
}
