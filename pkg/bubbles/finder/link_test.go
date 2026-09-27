package finder

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// linkRE matches the opening or the closing of a link, and the address
// it opens.
var linkRE = regexp.MustCompile("\x1b\\]8;;([^\x1b]*)\x1b\\\\")

// Each row links its path once, whatever is marked in it and however
// narrow the row, and draws the same cells as with no link.
func TestLinks(t *testing.T) {
	hostile := "evil\x1b]8;;https://evil.test\x1b\\name\x1b]8;;\x1b\\.go"
	paths := append([]string{hostile}, sample...)
	links := func(it Item) string { return "https://x.test/" + url.PathEscape(it.Path) }
	for _, width := range []int{60, 26, 12, 5} {
		for _, query := range []string{"", "rend"} {
			m := typed(t, open(t, width, 12, paths, WithLinks(links), WithIcons(extIcons)), query)
			plain := typed(t, open(t, width, 12, paths, WithIcons(extIcons)), query)
			v := m.View()
			if ansi.Strip(v) != ansi.Strip(plain.View()) {
				t.Errorf("at %d, %q: the links change the cells:\n%s\n%s", width, query, ansi.Strip(v), ansi.Strip(plain.View()))
			}
			lines := strings.Split(v, "\n")
			seen := map[string]bool{}
			for i, l := range lines[1 : len(lines)-1] {
				got := linkRE.FindAllStringSubmatch(l, -1)
				if strings.TrimSpace(ansi.Strip(l)) == "" {
					continue
				}
				if len(got) != 2 || got[1][1] != "" || !strings.HasPrefix(got[0][1], "https://x.test/") || seen[got[0][1]] {
					t.Errorf("at %d, %q: row %d opens and closes %q: %q", width, query, i, got, l)
					continue
				}
				seen[got[0][1]] = true
				if strings.Contains(l, "\x1b]8;;https://evil") {
					t.Errorf("at %d, %q: row %d carries the path's link: %q", width, query, i, l)
				}
				if w := ansi.StringWidth(l); w != width {
					t.Errorf("at %d, %q: row %d is %d cells", width, query, i, w)
				}
			}
		}
	}
}
