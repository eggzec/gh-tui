package notifications

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// The notifications can't be sorted, so their filter has no tabs, and
// applies the query the form did.
func TestFilterHasNoTabs(t *testing.T) {
	s := newSection(t, newFake(inbox()...), 80, 12)
	f, _ := s.Filter()
	if f.Spec.Sort != nil {
		t.Fatal("the notifications have a sort")
	}
	msg, names, _ := applyOn(t, s, f, filterform.SortTab)
	if names != nil || msg.Query != "is:unread" {
		t.Errorf("the filter shows tabs %q and applies %q; want no tabs, is:unread", names, msg.Query)
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
