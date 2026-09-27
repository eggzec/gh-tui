package issues

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// A row links its number and title to the issue's page, whatever its
// width, and with the selection's style too.
func TestRowLinks(t *testing.T) {
	s := newSection(t, newFakeService(nil), 100, 10).Section
	for _, selected := range []bool{false, true} {
		uitest.CheckLinks(t, func(host, title string, width int) (string, string) {
			it := core.Issue{Number: 7, Title: title, State: core.StateOpen, URL: ui.WebURL(host, "o/r/issues/7")}
			return s.renderRow(it, selected, width), it.URL
		}, 120, 80, 40, 20, 12, 8, 3, 1)
	}
}

// The title and the number in the header link to the issue's page.
func TestHeaderLinks(t *testing.T) {
	_, m := opened(t, newFakeService(sampleIssues(3)), 24)
	for _, host := range uitest.Hosts {
		it := m.issue
		it.URL, it.Title = ui.WebURL(host, "o/r/issues/3"), uitest.HostileTitle
		h := m.header(it)
		if links := uitest.Links(t, h); len(links) != 1 || links[0] != it.URL {
			t.Errorf("%s: header links to %q, want %q once", host, links, it.URL)
		}
		if strings.Contains(h, "evil.test") || strings.Contains(h, "\x1b[2J") {
			t.Errorf("%s: header carries the title's escapes: %q", host, h)
		}
	}
}
