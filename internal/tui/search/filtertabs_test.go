package search

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// The filters of repositories, issues and pull requests show their sort on
// its own tab, and either tab applies the query the combined form did.
// Code search takes no sort, so its filter has no tabs.
func TestFilterTabs(t *testing.T) {
	tests := []struct {
		kind  core.SearchKind
		query string
		want  string
	}{
		{core.SearchRepos, "tea language:go sort:stars-asc", "language:go sort:stars-asc tea"},
		{core.SearchIssues, "is:open sort:comments-desc crash", "is:open sort:comments-desc crash"},
		{core.SearchPulls, "is:merged tea", "is:merged tea"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			s := newSection(t, newFake(), 120, 30)
			f := ui.Filter{Spec: spec(tt.kind, tt.query), Query: tt.query}
			want := tt.want
			for i, tab := range []filterform.Tab{filterform.FiltersTab, filterform.SortTab} {
				msg, names, active := applyOn(t, s, f, tab)
				if !slices.Equal(names, []string{"Filters", "Sort"}) || active != i {
					t.Errorf("the modal shows tabs %q on %d, want Filters and Sort on %d", names, active, i)
				}
				if msg.Query != want {
					t.Errorf("applying from %v gave %q, want %q", tab, msg.Query, want)
				}
			}
		})
	}
	s := newSection(t, newFake(), 120, 30)
	f := ui.Filter{Spec: spec(core.SearchCode, "tea"), Query: "tea"}
	msg, names, _ := applyOn(t, s, f, filterform.SortTab)
	if names != nil || msg.Query != "tea" {
		t.Errorf("code search shows tabs %q and applies %q; want no tabs, the query", names, msg.Query)
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
