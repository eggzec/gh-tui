package history

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// The header of a commit links its SHA to the commit and its author to
// their profile, on the user's host.
func TestHeaderLinks(t *testing.T) {
	for _, host := range uitest.Hosts {
		m, h := newModal(t, newFake(), 140, 30, WithHost(host))
		h.keys("enter")
		m.commit.c.URL = ""
		m.commit.c.Author.Login = "octo-cat"
		m.commit.header = nil
		got := uitest.Links(t, strings.Join(m.headerLines(100), "\n"))
		want := []string{ui.WebURL(host, repo.String()+"/commit/"+m.commit.c.SHA), ui.WebURL(host, "octo-cat")}
		if len(got) < len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("%s: header links to %q, want it to start with %q", host, got, want)
		}
	}
}

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
