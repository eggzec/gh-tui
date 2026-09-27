package graph

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// linkRE matches the opening or the closing of a link, and the address
// it opens.
var linkRE = regexp.MustCompile("\x1b\\]8;;([^\x1b]*)\x1b\\\\")

// A commit with a link links its short SHA and title, once per row,
// however narrow the row and whatever its title holds.
func TestLinks(t *testing.T) {
	commits := sample()
	commits[0].Title = "evil\x1b]8;;https://evil.test\x1b\\title\x1b]8;;\x1b\\\u202e"
	for i := range commits {
		commits[i].Link = "https://ghe.test:8443/o/r/commit/" + commits[i].ID
	}
	for _, width := range []int{80, 30, 14} {
		m := load(t, newSource(commits, 50), WithSize(width, len(commits)))
		for i, l := range strings.Split(m.View(), "\n") {
			got := linkRE.FindAllStringSubmatch(l, -1)
			if len(got) != 2 || got[0][1] != commits[i].Link || got[1][1] != "" {
				t.Errorf("at %d: row %d opens and closes %q, want %q once: %q", width, i, got, commits[i].Link, l)
			}
			if w := ansi.StringWidth(l); w != width {
				t.Errorf("at %d: row %d is %d cells", width, i, w)
			}
			if strings.Contains(l, "evil.test") || strings.Contains(l, "\u202e") {
				t.Errorf("at %d: row %d carries the title's escapes: %q", width, i, l)
			}
		}
	}
}
