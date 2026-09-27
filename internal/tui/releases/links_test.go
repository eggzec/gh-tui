package releases

import (
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// The name and the tag in the header link to the release's page, and so
// does the title of the modal once it is read, and the releases before.
func TestLinks(t *testing.T) {
	m := newModal(&fakeService{}, 80, 24)
	if got := m.Link(); got != "https://github.com/charmbracelet/glow/releases" {
		t.Errorf("before the read, the title links to %q", got)
	}
	run(t, m, m.Init())
	if got := m.Link(); got != v3.URL {
		t.Errorf("the title links to %q, want %q", got, v3.URL)
	}
	for _, width := range []int{80, 20} {
		m.rel.Name = uitest.HostileTitle
		h := m.header(width)
		links := uitest.Links(t, h)
		if len(links) < 2 || slices.ContainsFunc(links, func(l string) bool { return l != v3.URL }) {
			t.Errorf("at %d: the header links to %q, want the name and the tag to %q", width, links, v3.URL)
		}
		if strings.Contains(h, "\x1b]8;;https://evil") || strings.Contains(h, "\x1b[2J") {
			t.Errorf("at %d: the header carries the name's escapes: %q", width, h)
		}
	}
}
