package logview

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// mark is an SGR sequence that applies from byte pos of a row's text on.
type mark struct {
	pos int
	// seq is the sequence to write. When reset is set, the sequence
	// started with a reset, and seq holds only what follows it.
	seq   string
	reset bool
}

// sanitize returns the text of s without escape sequences and control
// characters, so that a log can neither break the layout nor send commands
// to the terminal, and the SGR sequences it held, which only color text.
// Tabs expand to tabWidth, invalid UTF-8 turns into U+FFFD, and a carriage
// return drops what came before it, the way a terminal overwrites a
// progress line.
func sanitize(s string, tabWidth int) (string, []mark) {
	s = strings.TrimRight(s, "\r\n")
	if i := strings.LastIndexByte(s, '\r'); i >= 0 {
		s = s[i+1:]
	}
	if printableASCII(s) {
		return s, nil
	}
	var (
		b     strings.Builder
		marks []mark
	)
	b.Grow(len(s))
	// col is the column at byte tab of the output, where the last tab
	// ended; the columns after it are measured when the next tab needs them.
	tab, col := 0, 0
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ansi.ESC:
			n, sgr := escape(s[i:])
			if sgr {
				marks = append(marks, newMark(b.Len(), s[i:i+n]))
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
			case r == utf8.RuneError && size == 1:
				b.WriteRune(utf8.RuneError)
			case r < 0xa0:
				// A C1 control, such as the one-byte CSI.
			default:
				b.WriteString(s[i : i+size])
			}
			i += size
			continue
		}
		i++
	}
	return b.String(), marks
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

// escape returns the length of the escape sequence at the start of s, and
// whether it is an SGR sequence. A sequence cut short ends where it stops
// being valid, so what follows is shown as text.
func escape(s string) (n int, sgr bool) {
	if len(s) < 2 {
		return len(s), false
	}
	switch s[1] {
	case '[':
		j := 2
		for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3f {
			j++
		}
		params := s[2:j]
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
			j++
		}
		if j < len(s) && s[j] >= 0x40 && s[j] <= 0x7e {
			return j + 1, s[j] == 'm' && j == 2+len(params) && sgrParams(params)
		}
		return j, false
	case ']', 'P', 'X', '^', '_':
		// OSC, DCS, SOS, PM and APC hold a string up to BEL or ST.
		for j := 2; j < len(s); j++ {
			switch s[j] {
			case ansi.BEL:
				return j + 1, false
			case ansi.ESC:
				if j+1 < len(s) && s[j+1] == '\\' {
					return j + 2, false
				}
				return j, false
			}
		}
		return len(s), false
	}
	j := 1
	for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
		j++
	}
	if j < len(s) && s[j] >= 0x30 && s[j] <= 0x7e {
		return j + 1, false
	}
	return j, false
}

// sgrParams reports whether p holds only the digits and separators of SGR
// parameters, and no private markers.
func sgrParams(p string) bool {
	for i := range len(p) {
		if c := p[i]; (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

// newMark returns the mark of SGR sequence seq at pos. A sequence that
// resets the style, in full or first, is kept as a reset and the rest, so
// the view can put its own style back under what follows.
func newMark(pos int, seq string) mark {
	ps := strings.Split(seq[2:len(seq)-1], ";")
	// The last parameter that resets: empty or all zeros, and not an
	// argument of an extended color.
	cut := -1
	for i := 0; i < len(ps); i++ {
		switch p := strings.TrimLeft(ps[i], "0"); {
		case p == "":
			cut = i
		case (p == "38" || p == "48" || p == "58") && i+1 < len(ps):
			switch strings.TrimLeft(ps[i+1], "0") {
			case "5":
				i += 2
			case "2":
				i += 4
			}
		}
	}
	switch {
	case cut < 0:
		return mark{pos: pos, seq: seq}
	case cut == len(ps)-1:
		return mark{pos: pos, reset: true}
	default:
		return mark{pos: pos, seq: "\x1b[" + strings.Join(ps[cut+1:], ";") + "m", reset: true}
	}
}
