package markdown

import (
	"strings"
	"unicode/utf8"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// safe returns a rendered line with nothing left that can do more than
// color text. It keeps the style sequences that set colors and attributes,
// except blink and conceal, drops every other escape, turns a tab into a
// space, and replaces control characters, the characters that reorder
// text and invalid UTF-8 with U+FFFD. It runs on what glamour made, since
// glamour decodes character references such as &#27; after the source
// was cleaned.
func safe(line string) string {
	if plainASCII(line) {
		return line
	}
	var b strings.Builder
	b.Grow(len(line))
	for i := 0; i < len(line); {
		if line[i] == esc {
			n, ok := sgr(line[i:])
			if !ok {
				// Only the ESC goes; what follows shows as text.
				i++
				continue
			}
			if allowed(line[i+2 : i+n-1]) {
				b.WriteString(line[i : i+n])
			}
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case r == utf8.RuneError && size == 1, termtext.Control(r):
			b.WriteRune(utf8.RuneError)
		default:
			b.WriteString(line[i : i+size])
		}
		i += size
	}
	return b.String()
}

// plainASCII reports whether s is printable ASCII, as most lines are.
func plainASCII(s string) bool {
	for i := range len(s) {
		if c := s[i]; c < 0x20 || c >= 0x7f {
			return false
		}
	}
	return true
}

// sgr reports whether s starts with a style sequence, ESC [ followed by
// digits, semicolons and colons and then m, and how long it is.
func sgr(s string) (int, bool) {
	if len(s) < 3 || s[1] != '[' {
		return 0, false
	}
	j := 2
	for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == ';' || s[j] == ':') {
		j++
	}
	if j < len(s) && s[j] == 'm' {
		return j + 1, true
	}
	return 0, false
}

// allowed reports whether p, the parameters of a style sequence, are ones
// glamour could have written: at most 64 bytes and 32 parameters, each at
// most 255, and without blink or conceal, which glamour never sets.
func allowed(p string) bool {
	if len(p) > 64 {
		return false
	}
	ps := strings.Split(p, ";")
	if len(ps) > 32 {
		return false
	}
	for _, f := range ps {
		for v := range strings.SplitSeq(f, ":") {
			if v = strings.TrimLeft(v, "0"); len(v) > 3 || len(v) == 3 && v > "255" {
				return false
			}
		}
	}
	for i := 0; i < len(ps); i++ {
		f, _, sub := strings.Cut(ps[i], ":")
		switch strings.TrimLeft(f, "0") {
		case "5", "6", "8":
			return false
		case "38", "48", "58":
			if !sub {
				i += colorArgs(ps[i+1:])
			}
		}
	}
	return true
}
