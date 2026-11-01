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

// A commit, a user and a topic each link their row to a page, since there
// is no key that opens them.
func TestOtherKindLinks(t *testing.T) {
	s := newSection(t, newFake(), 140, 38, WithHost("github.com"))
	hits := map[string]core.SearchHit{
		"commit": {Kind: core.SearchCommits, Commit: core.SearchCommit{Repo: core.RepoRef{Owner: "o", Name: "r"}, SHA: "abcdef1234567", Subject: "Fix it"}},
		"user":   {Kind: core.SearchUsers, User: core.SearchUser{Login: "octocat"}},
		"topic":  {Kind: core.SearchTopics, Topic: core.SearchTopic{Name: "tea"}},
	}
	for name, hit := range hits {
		if links := uitest.Links(t, s.renderHit(hit, false, 140)); len(links) == 0 {
			t.Errorf("%s row has no link", name)
		}
	}
}

// A link cut short by the width keeps no open link or colour: the cut text
// is closed, at every width.
func TestTruncatedLinkClosed(t *testing.T) {
	s := newSection(t, newFake(), 140, 38)
	hit := core.CodeHit{Repo: core.RepoRef{Owner: "o", Name: "r"}, Path: "some/long/path/to/main.go", URL: ui.WebURL("github.com", "o/r/blob/abc/main.go")}
	for _, width := range []int{30, 20, 12} {
		row := s.renderCode(hit, false, width)
		if opens, closes := strings.Count(row, "\x1b]8;;http"), strings.Count(row, "\x1b]8;;\x1b\\"); opens != closes {
			t.Errorf("at %d: %d links opened, %d closed: %q", width, opens, closes, row)
		}
		if w := ansi.StringWidth(row); w != width {
			t.Errorf("at %d: row is %d cells", width, w)
		}
	}
}

// A code hit links its file to the address GitHub gave, whatever the host,
// as long as it is a plain https one, and its repository to the session's
// host. A hit of another host is not rewritten to the session's.
func TestCodeLinksOtherHost(t *testing.T) {
	s := newSection(t, newFake(), 140, 38, WithHost("github.com"))
	hit := core.CodeHit{Repo: core.RepoRef{Owner: "o", Name: "r"}, Path: "main.go", URL: "https://ghe.example/o/r/blob/abc/main.go"}
	links := uitest.Links(t, s.renderCode(hit, true, 140))
	if len(links) != 2 || links[0] != hit.URL || links[1] != ui.WebURL("github.com", "o/r") {
		t.Errorf("row links to %q, want the hit's own address and the session host's repository", links)
	}
}

// A file links its repository and itself to their pages.
func TestCodeLinks(t *testing.T) {
	for _, host := range uitest.Hosts {
		s := newSection(t, newFake(), 140, 38, WithHost(host))
		hit := core.CodeHit{Repo: core.RepoRef{Owner: "o", Name: "r"}, Path: uitest.HostileTitle, URL: ui.WebURL(host, "o/r/blob/abc/main.go")}
		for _, width := range []int{140, 40, 12, 3} {
			row := s.renderCode(hit, true, width)
			if links := uitest.Links(t, row); len(links) != 2 || links[0] != hit.URL || links[1] != ui.WebURL(host, "o/r") {
				t.Errorf("%s at %d: row links to %q, want the file and the repository", host, width, links)
			}
			if strings.Contains(row, "evil.test") || strings.Contains(row, "\x1b[2J") {
				t.Errorf("%s at %d: row carries the path's escapes: %q", host, width, row)
			}
		}
	}
}
