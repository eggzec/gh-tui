package search

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// The filters of repositories, issues and pull requests have no Sort tab,
// since the sort has a modal of its own, and a sort in the query survives a
// filter edit. Code search takes no sort either.
func TestFilterHasNoSortTab(t *testing.T) {
	tests := []struct {
		kind  core.SearchKind
		query string
		want  string
	}{
		{core.SearchRepos, "tea language:go sort:stars-asc", "tea language:go sort:stars-asc"},
		{core.SearchIssues, "is:open sort:comments-desc crash", "crash is:open sort:comments-desc"},
		{core.SearchPulls, "is:merged tea", "tea is:merged"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			s := newSection(t, newFake(), 120, 30)
			f := ui.Filter{Spec: spec(tt.kind, tt.query), Query: tt.query}
			msg, names, _ := applyOn(t, s, f)
			if names != nil {
				t.Errorf("the modal shows tabs %q, want none", names)
			}
			if msg.Query != tt.want {
				t.Errorf("applying gave %q, want %q", msg.Query, tt.want)
			}
		})
	}
	s := newSection(t, newFake(), 120, 30)
	f := ui.Filter{Spec: spec(core.SearchCode, "tea"), Query: "tea"}
	msg, names, _ := applyOn(t, s, f)
	if names != nil || msg.Query != "tea" {
		t.Errorf("code search shows tabs %q and applies %q; want no tabs, the query", names, msg.Query)
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
