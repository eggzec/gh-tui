package issues

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// The filter shows the sort on its own tab, and either tab applies the
// query the combined form did.
func TestFilterTabs(t *testing.T) {
	s := started(t, newFakeService(sampleIssues(12)), 80, 20)
	apply(t, s, `is:open label:bug assignee:@me sort:comments-desc fix`)
	f, _ := s.Filter()
	const want = "is:open label:bug assignee:@me sort:comments-desc fix"
	for i, tab := range []filterform.Tab{filterform.FiltersTab, filterform.SortTab} {
		msg, names, active := applyOn(t, s, f, tab)
		if !slices.Equal(names, []string{"Filters", "Sort"}) || active != i {
			t.Errorf("the modal shows tabs %q on %d, want Filters and Sort on %d", names, active, i)
		}
		if msg.Query != want {
			t.Errorf("applying from %v gave %q, want %q", tab, msg.Query, want)
		}
	}
}

// applyOn opens the filter modal of target on tab, as the app does, and
// applies it as it opened, from the query line, which every tab has. It
// returns what the form sent, and the tabs of the modal.
func applyOn(t *testing.T, target ui.Filterable, f ui.Filter, tab filterform.Tab) (msg filterform.AppliedMsg, names []string, active int) {
	t.Helper()
	m := ui.NewFilterModal(t.Context(), "List", target, f, ui.OnTab(tab))
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
