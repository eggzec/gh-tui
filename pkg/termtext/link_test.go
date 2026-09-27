package termtext

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	linkOpen  = "\x1b]8;;"
	linkClose = "\x1b]8;;\x1b\\"
)

func TestLink(t *testing.T) {
	styled := "\x1b[4mView\x1b[m ↗"
	tests := []struct{ name, addr, text, want string }{
		{"plain", "https://github.com/o/r/pull/1", "#1", linkOpen + "https://github.com/o/r/pull/1\x1b\\#1" + linkClose},
		{"styled", "https://x.test/a?b=c#d", styled, linkOpen + "https://x.test/a?b=c#d\x1b\\" + styled + linkClose},
		{"empty", "", "#1", "#1"},
		{"http", "http://x.test", "x", "x"},
		{"javascript", "javascript:alert(1)", "x", "x"},
		{"no host", "https:///path", "x", "x"},
		{"space", "https://x.test/a b", "x", "x"},
		{"escape", "https://x.test/\x1b\\\x1b]8;;https://evil.test", "x", "x"},
		{"bel", "https://x.test/\a", "x", "x"},
		{"non-ascii", "https://x.test/é", "x", "x"},
		{"user", "https://github.com@evil.test/", "x", "x"},
		{"user and password", "https://u:p@x.test/", "x", "x"},
		{"empty host with port", "https://:443/", "x", "x"},
		{"punycode", "https://xn--gthub-n4a.com/", "x", "x"},
		{"punycode label", "https://a.XN--p1ai/", "x", "x"},
		{"port 0", "https://x.test:0/", "x", "x"},
		{"port too big", "https://x.test:65536/", "x", "x"},
		{"port not a number", "https://x.test:ab/", "x", "x"},
		{"empty port", "https://x.test:/", "x", "x"},
		{"port", "https://x.test:8443/a", "x", linkOpen + "https://x.test:8443/a\x1b\\x" + linkClose},
		{"ipv6", "https://[::1]:443/", "x", linkOpen + "https://[::1]:443/\x1b\\x" + linkClose},
		{"too long", "https://x.test/" + strings.Repeat("a", MaxLink), "x", "x"},
		{"text's own link", "https://x.test", linkOpen + "https://evil.test\x1b\\x" + linkClose, linkOpen + "https://x.test\x1b\\x" + linkClose},
		{"text's controls", "https://x.test", "a\x1b[2J\x1b]0;t\ab\u202ec", linkOpen + "https://x.test\x1b\\abc" + linkClose},
	}
	for _, tt := range tests {
		if got := Link(tt.addr, tt.text); got != tt.want {
			t.Errorf("%s: Link(%q, %q)\n got %q\nwant %q", tt.name, tt.addr, tt.text, got, tt.want)
		}
	}
	if !linkable("https://x.test/" + strings.Repeat("a", MaxLink-len("https://x.test/"))) {
		t.Error("a link of MaxLink bytes isn't linkable")
	}
}

// A link takes no cells, and cutting it anywhere, from either side,
// leaves it closed.
func TestLinkWidthAndCuts(t *testing.T) {
	text := "\x1b[1mView\x1b[m diagram ↗"
	l := "ab " + Link("https://x.test/v", text) + " cd"
	want := 3 + ansi.StringWidth(text) + 3
	if w := ansi.StringWidth(l); w != want {
		t.Errorf("ansi.StringWidth = %d, want %d", w, want)
	}
	if w := lipgloss.Width(l); w != want {
		t.Errorf("lipgloss.Width = %d, want %d", w, want)
	}
	for n := range want + 1 {
		for _, c := range []string{ansi.Truncate(l, n, "…"), ansi.TruncateLeft(l, n, "…"), ansi.Cut(l, n/2, n)} {
			if o, e := strings.Count(c, linkOpen+"https"), strings.Count(c, linkClose); o != e || o > 1 {
				t.Errorf("cut at %d leaves %d opened, %d closed: %q", n, o, e, c)
			}
		}
	}
}
