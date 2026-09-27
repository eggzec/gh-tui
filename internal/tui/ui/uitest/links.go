package uitest

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Hosts are github.com and an Enterprise Server with a port, whose links
// must name the host as it is.
var Hosts = []string{"github.com", "ghe.example.com:8443"}

// HostileTitle is a title that tries to link somewhere else, to clear the
// screen and to reorder the text after it.
const HostileTitle = "Fix \x1b]8;;https://evil.test\x1b\\this\x1b]8;;\x1b\\ \x1b[2J\x1b]8;;https://evil.test\anow\u202e"

// link matches the opening and the closing of a link (OSC 8), ended with
// ST or BEL.
var link = regexp.MustCompile("\x1b\\]8;[^;\x1b\a]*;([^\x1b\a]*)(?:\x1b\\\\|\a)")

// Links returns the addresses of the links s opens, in order. It fails tb
// when a link opens before the one before it closed, or when one is left
// open at the end of a line.
func Links(tb testing.TB, s string) []string {
	tb.Helper()
	var addrs []string
	for l := range strings.SplitSeq(s, "\n") {
		open := false
		for _, m := range link.FindAllStringSubmatch(l, -1) {
			switch {
			case m[1] == "" && !open:
				tb.Errorf("a link closes that isn't open: %q", l)
			case m[1] == "":
				open = false
			case open:
				tb.Errorf("a link opens inside another: %q", l)
			default:
				open = true
				addrs = append(addrs, m[1])
			}
		}
		if open {
			tb.Errorf("a link is left open: %q", l)
		}
	}
	return addrs
}

// CheckLinks checks a row that links to what it shows. draw draws the row
// of an item on host with title in width cells, and returns it with the
// address it should link to. At each width and on each of [Hosts], the
// row must link to that address once and to nothing else, take width
// cells on each line as it would with no link, and keep its link whole
// however narrow it is. A [HostileTitle] must neither add a link nor get
// its escapes and its reordering through.
func CheckLinks(t *testing.T, draw func(host, title string, width int) (row, want string), widths ...int) {
	t.Helper()
	for _, host := range Hosts {
		for _, w := range widths {
			for _, title := range []string{"Add a disk layer to the cache", HostileTitle} {
				row, want := draw(host, title, w)
				if !strings.Contains(want, "://"+host+"/") {
					t.Errorf("%s at %d: want %q, which isn't on the host", host, w, want)
				}
				if got := Links(t, row); len(got) != 1 || got[0] != want {
					t.Errorf("%s at %d: row links to %q, want %q once: %q", host, w, got, want, row)
				}
				plain := link.ReplaceAllString(row, "")
				for i, l := range strings.Split(row, "\n") {
					pl := strings.Split(plain, "\n")[i]
					if lw, pw := ansi.StringWidth(l), ansi.StringWidth(pl); lw != pw {
						t.Errorf("%s at %d: line %d is %d cells, %d with no link", host, w, i, lw, pw)
					}
				}
				for _, bad := range []string{"\x1b]8;;https://evil", "\x1b[2J", "\u202e", "\a"} {
					if strings.Contains(row, bad) {
						t.Errorf("%s at %d: row carries %q of the title: %q", host, w, bad, row)
					}
				}
			}
		}
	}
}
