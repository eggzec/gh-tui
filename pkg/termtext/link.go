package termtext

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// MaxLink is how long the address of a [Link] is at most, in bytes.
// Terminals drop longer ones: VTE's limit is about 2 KiB.
const MaxLink = 2 << 10

// Link returns text as a hyperlink to addr (OSC 8), which a terminal that
// knows them opens on a click, and others show as text: ESC ]8;;addr ESC \
// text ESC ]8;; ESC \. Text keeps only its printable characters and the
// style sequences that color it, so it can't open, close or carry a link
// of its own. When addr is empty or not a plain https address of at most
// [MaxLink] bytes of printable ASCII, with a host of ASCII names and no
// user or port out of range, Link returns text as it is.
//
// These hold of the rest of the way to the terminal, as of charm.land
// lipgloss v2.0.6, bubbletea v2.0.9, ultraviolet 2026-08-11 and
// x/ansi v0.11.8:
//   - ansi.StringWidth and lipgloss.Width count the link as no cells.
//   - ansi.Truncate, TruncateLeft and Cut keep every escape sequence,
//     those of the cut part too, so a cut link still ends where it did,
//     around less text or none, and never stays open.
//   - bubbletea's renderer and lipgloss's compositor read a view with
//     ultraviolet, which keeps the link on each cell it covers (OSC 8 into
//     Cell.Link) and writes it again around those cells, ended with BEL.
func Link(addr, text string) string {
	if !linkable(addr) {
		return text
	}
	return "\x1b]8;;" + addr + "\x1b\\" + plain(text) + "\x1b]8;;\x1b\\"
}

// linkable reports whether addr may be the address of a link. It takes
// no user, which could make the address read as another host, nor a host
// in punycode, whose name may look like another's.
func linkable(addr string) bool {
	if addr == "" || len(addr) > MaxLink {
		return false
	}
	for i := range len(addr) {
		if c := addr[i]; c <= ' ' || c >= 0x7f {
			return false
		}
	}
	u, err := url.Parse(addr)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return false
	}
	for label := range strings.SplitSeq(strings.ToLower(u.Hostname()), ".") {
		if strings.HasPrefix(label, "xn--") {
			return false
		}
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		return err == nil && n >= 1 && n <= 65535
	}
	// A colon with no port after it.
	return !strings.HasSuffix(u.Host, ":")
}

// plain returns s with only its printable characters and its style
// sequences, ESC [ … m.
func plain(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	var state byte
	for s != "" {
		seq, width, n, next := ansi.DecodeSequence(s, state, nil)
		if n == 0 {
			break
		}
		if width > 0 || strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
			b.WriteString(seq)
		}
		s, state = s[n:], next
	}
	return b.String()
}
