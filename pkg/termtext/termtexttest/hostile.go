// Package termtexttest checks that text drawn in a terminal is safe to draw.
package termtexttest

import (
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// Hostile is text from GitHub, such as a title or a path, that tries to
// clear the screen, move the cursor, set the terminal's title, link
// somewhere else, ring the bell, send C1 controls, both as UTF-8 and as
// lone bytes and after an unfinished lead byte of UTF-8, send other
// invalid UTF-8, and reorder the text after it.
const Hostile = "Fix \x1b[2J\x1b[H\x1b[5;10Hmoved \x1b]0;pwned\a\x1b]2;pwned\x1b\\ " +
	"\x1b]8;;https://evil.test\x1b\\link\x1b]8;;\x1b\\ bell\a " +
	"\u009b2J \u009d0;t\u009c \x9b2J \x9d0;t\x9c " +
	"\xe2\x9b2J \u00e9\xe2\x9d8;;https://evil.test\x9cX a\x80b a\xe2b x\xe2\x80 " +
	"\u202eexe.txt\u202c \u2066isolated\u2069 end"

// styles matches what a view may send the terminal: the SGR sequences
// that color text, and the opening or closing of a link (OSC 8).
var styles = regexp.MustCompile("\x1b\\[[0-9;:]*m|\x1b\\]8;[^;\x1b\a]*;([^\x1b\a]*)(?:\x1b\\\\|\a)")

// AssertClean fails tb if view sends the terminal anything but text in
// colors and links, such as an escape sequence, a control character, a
// lone byte of C1 or a bidi control, or if a line of it is wider than
// width cells.
func AssertClean(tb testing.TB, view string, width int) {
	tb.Helper()
	for _, m := range styles.FindAllStringSubmatch(view, -1) {
		if strings.Contains(m[1], "evil") {
			tb.Errorf("view links to %q: %q", m[1], view)
		}
	}
	plain := styles.ReplaceAllString(view, "")
	if !utf8.ValidString(view) {
		tb.Errorf("view is not valid UTF-8, so a byte of it may read as C1: %q", view)
	}
	for _, r := range plain {
		if r != '\n' && (r < 0x20 || r >= 0x7f && r < 0xa0 || r != '\u200c' && r != '\u200d' && unicode.Is(unicode.Cf, r)) {
			tb.Errorf("view holds %U: %q", r, view)
			break
		}
	}
	for i, l := range strings.Split(plain, "\n") {
		if w := ansi.StringWidth(l); w > width {
			tb.Errorf("line %d is %d cells, wider than %d: %q", i, w, width, l)
		}
	}
}
