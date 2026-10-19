package dashboard

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
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
// edits the filter that is off, applying or F ends the switch, and a pane
// with no filter refuses.
func TestFilterToggle(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, nil, 140, 38)

	if msgs := press(t, s, "alt+f"); !toasted(msgs) || s.repos.toggle.Off() {
		t.Errorf("alt+f with no filter: toast %v, off %v; want a toast and no switch", toasted(msgs), s.repos.toggle.Off())
	}

	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "is:private language:go"}))
	query := s.repos.filter().Query()
	if query == "" {
		t.Fatal("the filter was not applied")
	}
	press(t, s, "alt+f")
	if s.repos.filter().Active() || !s.repos.toggle.Off() {
		t.Errorf("alt+f left the filter active %v, off %v; want it off", s.repos.filter().Active(), s.repos.toggle.Off())
	}
	if view := screen(s); !strings.Contains(view, "off") || !strings.Contains(view, "private") {
		t.Errorf("the title should say the filter is off:\n%s", view)
	}
	if f, _ := s.Filter(); f.Query != query {
		t.Errorf("the form opens on %q while off, want %q", f.Query, query)
	}

	press(t, s, "alt+f")
	if got := s.repos.filter().Query(); got != query || s.repos.toggle.Off() {
		t.Errorf("alt+f again left the filter %q, off %v; want %q on", got, s.repos.toggle.Off(), query)
	}

	press(t, s, "alt+f", "F")
	if s.repos.filter().Active() || s.repos.toggle.Off() {
		t.Errorf("F while off left active %v, off %v; want it cleared and on", s.repos.filter().Active(), s.repos.toggle.Off())
	}
	if msgs := press(t, s, "alt+f"); !toasted(msgs) {
		t.Error("alt+f after F said nothing")
	}

	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "is:private"}))
	press(t, s, "alt+f")
	run(t, s, s.ApplyFilter(filterform.AppliedMsg{Query: "is:private"}))
	if s.repos.toggle.Off() || !s.repos.filter().Active() {
		t.Errorf("applying while off left off %v, active %v; want the filter on", s.repos.toggle.Off(), s.repos.filter().Active())
	}
}
