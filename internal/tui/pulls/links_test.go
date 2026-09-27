package pulls

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// A row links its number and title to the pull request's page, whatever
// its width, and with the selection's style too.
func TestRowLinks(t *testing.T) {
	s := started(t, newFakeService(), 100, 10).Section
	for _, selected := range []bool{false, true} {
		uitest.CheckLinks(t, func(host, title string, width int) (string, string) {
			pr := core.PullRequest{}
			pr.Number, pr.Title, pr.State = 7, title, core.StateOpen
			pr.URL = ui.WebURL(host, "o/r/pull/7")
			return s.renderRow(pr, selected, width), pr.URL
		}, 120, 80, 40, 20, 12, 8, 3, 1)
	}
}

// The title and the number in the header link to the pull request's page.
func TestDetailHeaderLinks(t *testing.T) {
	s := started(t, newFakeService(), 120, 30)
	press(t, s, "enter")
	m := s.modal()
	for _, host := range uitest.Hosts {
		m.detail.URL = ui.WebURL(host, "o/r/pull/142")
		m.detail.Title = uitest.HostileTitle
		links := uitest.Links(t, m.detailHeader(80))
		if len(links) != 2 || links[0] != m.detail.URL || links[1] != m.detail.URL {
			t.Errorf("%s: header links to %q, want the title and the number to %q", host, links, m.detail.URL)
		}
		if h := m.detailHeader(80); strings.Contains(h, "evil.test") || strings.Contains(h, "\x1b[2J") {
			t.Errorf("%s: header carries the title's escapes: %q", host, h)
		}
	}
}
