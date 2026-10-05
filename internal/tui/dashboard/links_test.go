package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// checkLinks checks that lines link to want, each once, keep to width
// and carry nothing of a hostile title.
func checkLinks(t *testing.T, what string, lines []string, want string, width int) {
	t.Helper()
	got := uitest.Links(t, strings.Join(lines, "\n"))
	if len(got) != len(lines) || slices.ContainsFunc(got, func(a string) bool { return a != want }) {
		t.Errorf("%s at %d: lines link to %q, want %q on each of %d", what, width, got, want, len(lines))
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("%s at %d: line is %d cells: %q", what, width, w, l)
		}
		if strings.Contains(l, "evil.test") || strings.Contains(l, "\x1b[2J") || strings.Contains(l, "\u202e") {
			t.Errorf("%s at %d: line carries the title's escapes: %q", what, width, l)
		}
	}
}

// Repositories link their names to their pages on the user's host, in
// the list and on the cards.
func TestRepoLinks(t *testing.T) {
	for _, host := range uitest.Hosts {
		s := newSection(t, newFake(), nil, 140, 40, WithHost(host))
		r := core.Repo{Ref: core.RepoRef{Owner: "o", Name: "r"}, Description: "A repository"}
		want := ui.WebURL(host, "o/r")
		var m ownerui.Measure
		m.Add(r, s.icons)
		for _, width := range []int{100, 40, 20} {
			c := ownerui.LayoutCols(width, m, "*", 4)
			checkLinks(t, host+" row", []string{s.renderRepo(c, r, true)}, want, width)
			card := s.card(ownerui.Card{Repo: r}, true, width)
			checkLinks(t, host+" card", card[:1], want, width)
		}
	}
}

// Work links where it is and each line of its title to its page.
func TestWorkLinks(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 40)
	for _, host := range uitest.Hosts {
		h := hit(core.SearchPulls, "o/r", 7, uitest.HostileTitle+" and a title long enough to wrap", false, 0)
		h.Issue.URL = ui.WebURL(host, "o/r/pull/7")
		for _, width := range []int{60, 30, 12} {
			r := workRow{hit: &h}
			r.ref, r.lines = wrapWork(h.Issue, width-workIndent, 3, "…")
			lines := s.workItem(nil, &r, true, true, width)
			checkLinks(t, host+" work", lines, h.Issue.URL, width)
		}
	}
}

// A notification links its repository and title to the thread's page.
func TestInboxLinks(t *testing.T) {
	th := thread("7", "o/r", uitest.HostileTitle, true, 0)
	th.Subject.WebURL = ui.WebURL(uitest.Hosts[1], "o/r/pull/7")
	s := newSection(t, newFake(), &fakeInbox{threads: []core.Notification{th}}, 140, 40)
	lines := s.inboxBody(60, 5)
	checkLinks(t, "inbox", lines[1:], th.Subject.WebURL, 60)
}

// However the panes cut the rows, every link they show is closed.
func TestViewLinksClosed(t *testing.T) {
	for _, width := range []int{140, 100, 60, 30} {
		s := newSection(t, newFake(), &fakeInbox{threads: inboxThreads()}, width, 38)
		if links := uitest.Links(t, s.View()); len(links) == 0 {
			t.Errorf("at %d: the dashboard links to nothing", width)
		}
	}
}
