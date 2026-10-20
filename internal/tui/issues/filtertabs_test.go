package issues

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// The filter has no Sort tab, since the sort has a modal of its own, and a
// sort in the query survives a filter edit.
func TestFilterHasNoSortTab(t *testing.T) {
	s := started(t, newFakeService(sampleIssues(12)), 80, 20)
	apply(t, s, `is:open label:bug assignee:@me sort:comments-desc fix`)
	f, _ := s.Filter()
	if got := f.Spec.Fields[0].Key; got != "search" {
		t.Errorf("the filter opens on the %q row, want the search", got)
	}
	const want = "fix is:open label:bug assignee:@me sort:comments-desc"
	msg, names, _ := applyOn(t, s, f)
	if names != nil {
		t.Errorf("the modal shows tabs %q, want none", names)
	}
	if msg.Query != want {
		t.Errorf("applying gave %q, want %q", msg.Query, want)
	}
}

// applyOn opens the filter modal of target, as the app does, and
// applies it as it opened, from the query line, which every tab has. It
// returns what the form sent, and the tabs of the modal.
func applyOn(t *testing.T, target ui.Filterable, f ui.Filter) (msg filterform.AppliedMsg, names []string, active int) {
	t.Helper()
	m := ui.NewFilterModal(t.Context(), "List", target, f)
	names, active = m.Tabs()
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter sent nothing")
	}
	msg, ok := cmd().(filterform.AppliedMsg)
	if !ok {
		t.Fatalf("enter sent %T, want the form applied", cmd())
	}
	return msg, names, active
}
