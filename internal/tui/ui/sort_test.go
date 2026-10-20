package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

// sortFixture opens the sort modal of a list that sorts by update time or
// comments, with query typed in its filter.
func sortFixture(t *testing.T, query string) (*SortModal, *fakeFilterable) {
	t.Helper()
	d := config.Default()
	target := &fakeFilterable{}
	spec := filterform.Spec{Sort: &filterform.SortField{
		Options: []filterform.SortOption{SortByTime("Updated", "updated"), SortByCount("Comments", "comments")},
		Default: filterform.Sort{By: "updated", Desc: true},
	}}
	m := NewSortModal("Pull requests", target, Filter{Spec: spec, Query: query}, OrderPickKeys(d.Keys), NewIcons(d.UI.Icons))
	m.SetSize(120, 30)
	return m, target
}

// sendSort gives the modal the keys, and what the commands they start send.
func sendSort(m *SortModal, keys ...string) {
	for _, k := range keys {
		for _, msg := range drain(m.Update(keyOf(k))) {
			m.Update(msg)
		}
	}
}

func TestSortModalApplies(t *testing.T) {
	tests := []struct {
		name  string
		query string
		keys  []string
		want  filterform.AppliedMsg
	}{
		{
			name:  "the sort in force is chosen and applies as it was",
			query: "is:closed sort:comments-asc",
			keys:  []string{"enter"},
			want:  filterform.AppliedMsg{Query: "is:closed sort:comments-asc", Sort: filterform.Sort{By: "comments"}},
		},
		{
			name:  "no sort in the query chooses the default",
			query: "is:closed",
			keys:  []string{"enter"},
			want:  filterform.AppliedMsg{Query: "is:closed sort:updated-desc", Sort: filterform.Sort{By: "updated", Desc: true}},
		},
		{
			name:  "ticking another sort replaces it",
			query: "is:closed sort:comments-asc",
			keys:  []string{"space", "enter"},
			want:  filterform.AppliedMsg{Query: "is:closed sort:updated-desc", Sort: filterform.Sort{By: "updated", Desc: true}},
		},
		{
			name:  "reversing the chosen one flips its order",
			query: "is:closed sort:comments-asc",
			keys:  []string{"tab", "space", "enter"},
			want:  filterform.AppliedMsg{Query: "is:closed sort:comments-desc", Sort: filterform.Sort{By: "comments", Desc: true}},
		},
		{
			name:  "nothing chosen drops the sort",
			query: "is:closed sort:comments-asc",
			keys:  []string{"tab", "x", "enter"},
			want:  filterform.AppliedMsg{Query: "is:closed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, target := sortFixture(t, tt.query)
			sendSort(m, tt.keys...)
			if len(target.applied) != 1 || target.applied[0].Query != tt.want.Query || target.applied[0].Sort != tt.want.Sort {
				t.Errorf("applied %+v, want %+v", target.applied, tt.want)
			}
		})
	}
}

func TestSortModalTakesNewIcons(t *testing.T) {
	t.Parallel()
	m, _ := sortFixture(t, "is:closed")
	d := config.Default()
	d.UI.Icons = config.IconsASCII
	ic := NewIcons(config.IconsASCII)
	m.Update(SettingsMsg{Config: d})
	if want := "Sort" + ic.Separator + "Pull requests"; m.Title() != want {
		t.Errorf("title %q after the icons changed, want %q", m.Title(), want)
	}
	if m.icons.Set() != config.IconsASCII {
		t.Errorf("icons %q, want %q", m.icons.Set(), config.IconsASCII)
	}
}

func TestSortModalEscapeApplies(t *testing.T) {
	m, target := sortFixture(t, "is:closed sort:comments-asc")
	msgs := drain(m.Update(keyOf("esc")))
	cmds := make([]tea.Msg, 0, len(msgs))
	for _, msg := range msgs {
		cmds = append(cmds, drain(m.Update(msg))...)
	}
	if len(target.applied) != 0 {
		t.Errorf("esc applied %+v, want nothing", target.applied)
	}
	if len(cmds) != 1 {
		t.Fatalf("esc sent %v, want the modal closed", cmds)
	}
	if c, ok := cmds[0].(CloseModalMsg); !ok || c.Modal != m {
		t.Errorf("esc sent %#v, want the modal closed", cmds[0])
	}
}
