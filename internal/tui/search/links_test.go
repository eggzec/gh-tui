package search

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// An issue or a pull request links where it is and its title to its page.
func TestHitLinks(t *testing.T) {
	s := newSection(t, newFake(), 140, 38)
	for _, selected := range []bool{false, true} {
		uitest.CheckLinks(t, func(host, title string, width int) (string, string) {
			hit := issue(core.SearchPulls, "o/r", 7, title, core.StateOpen, false)
			hit.Issue.URL = ui.WebURL(host, "o/r/pull/7")
			return s.renderHit(hit, selected, width), hit.Issue.URL
		}, 140, 80, 40, 12, 3)
	}
}

// A repository links its name to its page on the user's host.
func TestRepoLinks(t *testing.T) {
	for _, host := range uitest.Hosts {
		s := newSection(t, newFake(), 140, 38, WithHost(host))
		r := core.Repo{Ref: core.RepoRef{Owner: "o", Name: "r"}, Description: "A repository"}
		for _, width := range []int{140, 40, 12, 3} {
			row := s.renderRepo(r, true, width)
			if links := uitest.Links(t, row); len(links) != 1 || links[0] != ui.WebURL(host, "o/r") {
				t.Errorf("%s at %d: row links to %q, want the repository once", host, width, links)
			}
			for l := range strings.SplitSeq(row, "\n") {
				if w := ansi.StringWidth(l); w != width {
					t.Errorf("%s at %d: line is %d cells: %q", host, width, w, l)
				}
			}
		}
	}
}

// A file links its repository and itself to their pages.
func TestCodeLinks(t *testing.T) {
	for _, host := range uitest.Hosts {
		s := newSection(t, newFake(), 140, 38, WithHost(host))
		hit := core.CodeHit{Repo: core.RepoRef{Owner: "o", Name: "r"}, Path: uitest.HostileTitle, URL: ui.WebURL(host, "o/r/blob/abc/main.go")}
		for _, width := range []int{140, 40, 12, 3} {
			row := s.renderCode(hit, true, width)
			if links := uitest.Links(t, row); len(links) != 2 || links[0] != ui.WebURL(host, "o/r") || links[1] != hit.URL {
				t.Errorf("%s at %d: row links to %q, want the repository and the file", host, width, links)
			}
			if strings.Contains(row, "evil.test") || strings.Contains(row, "\x1b[2J") {
				t.Errorf("%s at %d: row carries the path's escapes: %q", host, width, row)
			}
		}
	}
}
