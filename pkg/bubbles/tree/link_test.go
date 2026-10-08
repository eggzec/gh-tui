package tree

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// linkRE matches the opening of a link and the address it opens.
var linkRE = regexp.MustCompile("\x1b\\]8;;([^\x1b]*)\x1b\\\\")

// A node with a link links its name, once per row, however narrow the row
// and whatever its name holds, and a row cut before its name leaves the
// link out whole.
func TestLinks(t *testing.T) {
	hostile := "evil\x1b]8;;https://evil.test\x1b\\name\x1b]8;;\x1b\\\u202e.go"
	kids := []Node{
		{ID: "cmd", Name: "cmd", Branch: true, Link: "https://ghe.test:8443/o/r/tree/main/cmd"},
		{ID: "main.go", Name: "a-long-file-name-for-a-narrow-row.go", Detail: "1.2 kB", Link: "https://ghe.test:8443/o/r/blob/main/main.go"},
		{ID: "evil.go", Name: hostile, Link: "https://ghe.test:8443/o/r/blob/main/evil.go"},
		{ID: "plain", Name: "plain"},
	}
	children := func(_ context.Context, parent Node) ([]Node, error) {
		if parent.ID != "" {
			return nil, nil
		}
		return kids, nil
	}
	for _, width := range []int{60, 20, 8, 3} {
		m := run(t, newModel(children, WithSize(width, len(kids)), WithFocused(true)), nil)
		m = run(t, m, m.Init())
		lines := strings.Split(m.View(), "\n")
		for i, n := range kids {
			l := lines[i]
			got := linkRE.FindAllStringSubmatch(l, -1)
			switch {
			case n.Link == "" && len(got) != 0:
				t.Errorf("at %d: %s links to %q", width, n.ID, got)
			case n.Link != "" && len(got) == 0 && width < 8:
				// The row is cut before the name.
			case n.Link != "" && (len(got) != 2 || got[0][1] != n.Link || got[1][1] != ""):
				t.Errorf("at %d: %s opens and closes %q, want %q once: %q", width, n.ID, got, n.Link, l)
			}
			if w := ansi.StringWidth(l); w != width {
				t.Errorf("at %d: %s is %d cells", width, n.ID, w)
			}
			if strings.Contains(l, "evil.test") || strings.Contains(l, "\u202e") {
				t.Errorf("at %d: %s carries the name's escapes: %q", width, n.ID, l)
			}
		}
	}
}
