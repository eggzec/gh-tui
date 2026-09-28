package logview

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// sanitize returns the text of s without escape sequences and control
// characters, so that a log can neither break the layout nor send commands
// to the terminal, and the SGR sequences it held, which only color text.
// Tabs expand to tabWidth; invalid UTF-8, C1 controls, the characters that
// reorder text and the kitty image placeholder turn into U+FFFD
// (termtext.Control); other invisible format characters are dropped
// (termtext.Hidden); and a carriage return drops what came before it, the
// way a terminal overwrites a progress line.
func sanitize(s string, tabWidth int) (string, []termtext.Style) {
	s = strings.TrimRight(s, "\r\n")
	if i := strings.LastIndexByte(s, '\r'); i >= 0 {
		s = s[i+1:]
	}
	if printableASCII(s) {
		return s, nil
	}
	var (
		b     strings.Builder
		marks termtext.Styler
	)
	b.Grow(len(s))
	// col is the column at byte tab of the output, where the last tab
	// ended; the columns after it are measured when the next tab needs them.
	tab, col := 0, 0
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ansi.ESC:
			n, sgr := termtext.Escape(s[i:])
			if sgr {
				marks.Add(b.Len(), s[i:i+n])
			}
			i += n
			continue
		case c == '\t':
			col += ansi.StringWidth(b.String()[tab:])
			n := tabWidth - col%tabWidth
			for range n {
				b.WriteByte(' ')
			}
			tab, col = b.Len(), col+n
		case c < 0x20 || c == 0x7f:
		case c < utf8.RuneSelf:
			b.WriteByte(c)
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			switch {
			case r == utf8.RuneError && size == 1, termtext.Control(r):
				// A C1 control, such as the one-byte CSI, a character that
				// reorders the text, or the kitty graphics protocol's image
				// placeholder, which a workflow may echo into its log.
				b.WriteRune(utf8.RuneError)
			case termtext.Hidden(r):
				// An invisible format character, dropped as OneLine drops
				// it.
			default:
				b.WriteString(s[i : i+size])
			}
			i += size
			continue
		}
		i++
	}
	return b.String(), marks.Styles()
}

// printableASCII reports whether s is printable ASCII, which is what most
// lines of a log are, and needs no sanitizing.
func printableASCII(s string) bool {
	for i := range len(s) {
		if c := s[i]; c < 0x20 || c >= 0x7f {
			return false
		}
	}
	return true
}
