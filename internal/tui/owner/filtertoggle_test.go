package owner

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// toasted reports whether msgs hold a toast.
func toasted(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(ui.NotifyMsg); ok {
			return true
		}
	}
	return false
}

// Alt+f lists the repositories without the filter and brings it back, f
// edits the filter that is off, applying or F ends the switch, and a list
// with no filter refuses.
func TestFilterToggle(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 80, 24)
	l := s.repoTab()

	if msgs := press(t, s, "alt+f"); !toasted(msgs) || l.toggle.Off() {
		t.Errorf("alt+f with no filter: toast %v, off %v; want a toast and no switch", toasted(msgs), l.toggle.Off())
	}

	run(t, s, s.ApplyFilter(filterApplied("language:rust")))
	query := l.Filter().Query()
	if query == "" || s.page.repos().Feed.Len() != 10 {
		t.Fatalf("the filter lists %d repositories with query %q", s.page.repos().Feed.Len(), query)
	}
	press(t, s, "alt+f")
	if l.Filter().Active() || !l.toggle.Off() || s.page.repos().Feed.Len() != 30 {
		t.Errorf("alt+f left active %v, off %v, %d rows; want it off with the whole list", l.Filter().Active(), l.toggle.Off(), s.page.repos().Feed.Len())
	}
	if f, _ := s.Filter(); f.Query != query {
		t.Errorf("the form opens on %q while off, want %q", f.Query, query)
	}

	press(t, s, "alt+f")
	if got := l.Filter().Query(); got != query || l.toggle.Off() || s.page.repos().Feed.Len() != 10 {
		t.Errorf("alt+f again left the filter %q, off %v; want %q on", got, l.toggle.Off(), query)
	}

	press(t, s, "alt+f", "F")
	if l.Filter().Active() || l.toggle.Off() || s.page.repos().Feed.Len() != 30 {
		t.Errorf("F while off left active %v, off %v; want it cleared and on", l.Filter().Active(), l.toggle.Off())
	}
	if msgs := press(t, s, "alt+f"); !toasted(msgs) {
		t.Error("alt+f after F said nothing")
	}

	run(t, s, s.ApplyFilter(filterApplied("language:rust")))
	press(t, s, "alt+f")
	run(t, s, s.ApplyFilter(filterApplied("language:go")))
	if l.toggle.Off() || !l.Filter().Active() {
		t.Errorf("applying while off left off %v, active %v; want the filter on", l.toggle.Off(), l.Filter().Active())
	}
}

// Alt+s lists the repositories without the sort token and brings it back,
// keeping the filter, and a list with no sort refuses.
func TestSortToggle(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 80, 24)
	l := s.repoTab()

	if msgs := press(t, s, "alt+s"); !toasted(msgs) || l.sortToggle.Off() {
		t.Errorf("alt+s with no sort: toast %v, off %v; want a toast and no switch", toasted(msgs), l.sortToggle.Off())
	}

	run(t, s, s.ApplyFilter(filterApplied("language:rust sort:stars-desc")))
	query := l.Filter().Query()
	if !strings.Contains(query, "sort:") {
		t.Fatalf("the filter has no sort: %q", query)
	}
	press(t, s, "alt+s")
	if got := l.Filter().Query(); strings.Contains(got, "sort:") || !strings.Contains(got, "language:rust") || !l.sortToggle.Off() {
		t.Errorf("alt+s left the filter %q, off %v; want the filter without the sort, off", got, l.sortToggle.Off())
	}
	if f, _ := s.Filter(); f.Query != query {
		t.Errorf("the form opens on %q while off, want %q", f.Query, query)
	}

	press(t, s, "alt+s")
	if got := l.Filter().Query(); got != query || l.sortToggle.Off() {
		t.Errorf("alt+s again left the filter %q, off %v; want %q on", got, l.sortToggle.Off(), query)
	}

	run(t, s, s.ApplyFilter(filterApplied("language:rust")))
	if msgs := press(t, s, "alt+s"); !toasted(msgs) || l.sortToggle.Off() {
		t.Errorf("alt+s with a filter and no sort: toast %v, off %v; want a toast and no switch", toasted(msgs), l.sortToggle.Off())
	}
}
