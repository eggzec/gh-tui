package history

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// The rows of the graph link to the pages of their commits: the one
// GitHub gave, or else the commit's on the user's host.
func TestGraphLinks(t *testing.T) {
	for _, host := range uitest.Hosts {
		f := newFake()
		for i := range f.histories["main"] {
			f.histories["main"][i].URL = ""
		}
		f.histories["main"][0].Subject = uitest.HostileTitle
		m, _ := newModal(t, f, 140, 20, WithHost(host))
		links := uitest.Links(t, m.graph.model.View())
		if len(links) == 0 {
			t.Fatalf("%s: the graph links to nothing", host)
		}
		for i, l := range links {
			if want := ui.WebURL(host, repo.String()+"/commit/"+sha("main", i)); l != want {
				t.Errorf("%s: row %d links to %q, want %q", host, i, l, want)
			}
		}
		if v := m.graph.model.View(); strings.Contains(v, "\x1b]8;;https://evil") {
			t.Errorf("%s: the graph carries the subject's link", host)
		}
	}
	m, _ := newModal(t, newFake(), 140, 20)
	if links := uitest.Links(t, m.graph.model.View()); len(links) == 0 || links[0] != "https://github.com/charmbracelet/bubbletea/commit/"+sha("main", 0) {
		t.Errorf("the graph links to %q, want GitHub's own pages", links)
	}
}
