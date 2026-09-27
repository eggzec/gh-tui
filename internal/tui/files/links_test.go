package files

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// Each row of the tree links to the page of its file or directory at the
// base, on the user's host, however narrow the pane.
func TestTreeLinks(t *testing.T) {
	want := []string{
		"tree/HEAD/cmd", "tree/HEAD/internal", "tree/HEAD/vendor-lib", "blob/HEAD/.gitignore",
		"blob/HEAD/AGENTS.md", "blob/HEAD/CLAUDE.md", "blob/HEAD/go.mod", "blob/HEAD/README%20with%20spaces.md",
	}
	for _, host := range uitest.Hosts {
		for _, width := range []int{60, 20} {
			s := loaded(t, sampleFake(), width, 12, WithHost(host))
			v := s.View()
			links := uitest.Links(t, v)
			var got []string
			for _, l := range links {
				got = append(got, strings.TrimPrefix(l, ui.WebURL(host, "eggzec/gh-tui/")))
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s at %d: rows link to %q, want %q under %s", host, width, links, want, ui.WebURL(host, "eggzec/gh-tui"))
			}
			for l := range strings.SplitSeq(v, "\n") {
				if w := ansi.StringWidth(l); w != width {
					t.Errorf("%s at %d: row is %d cells: %q", host, width, w, l)
				}
			}
		}
	}
}

// The title of a preview links to the page of the file.
func TestPreviewLink(t *testing.T) {
	h := openRow(t, sampleFake(), rowReadme)
	l, ok := h.top().(ui.Linked)
	if want := "https://github.com/eggzec/gh-tui/blob/HEAD/README%20with%20spaces.md"; !ok || l.Link() != want {
		t.Errorf("the preview links to %v, want %q", h.top(), want)
	}
}
