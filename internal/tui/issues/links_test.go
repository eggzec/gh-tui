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

// The header holds the labels only, cleaned of a hostile name's escapes.
func TestHeaderCleansLabels(t *testing.T) {
	_, m := opened(t, newFakeService(sampleIssues(3)), 24)
	it := m.issue
	it.Labels = []core.Label{{Name: uitest.HostileTitle}}
	h := m.header(it)
	if strings.Contains(h, "evil.test") || strings.Contains(h, "\x1b[2J") {
		t.Errorf("header carries the label's escapes: %q", h)
	}
	it.Labels = nil
	if h := m.header(it); h != "" {
		t.Errorf("header without labels = %q, want empty", h)
	}
}
