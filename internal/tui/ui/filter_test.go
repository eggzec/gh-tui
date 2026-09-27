package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
)

type fakeFilterable struct{ applied []filterform.AppliedMsg }

func (f *fakeFilterable) Filter() (Filter, bool) { return Filter{}, true }
func (f *fakeFilterable) ApplyFilter(msg filterform.AppliedMsg) tea.Cmd {
	f.applied = append(f.applied, msg)
	return nil
}

func TestFilterModalIgnoresOtherForms(t *testing.T) {
	target := &fakeFilterable{}
	spec := filterform.Spec{Fields: []filterform.Field{{Key: "base", Label: "Base", Kind: filterform.Text, Qualifier: "base"}}}
	m := NewFilterModal(t.Context(), "Issues", target, Filter{Spec: spec, Query: "base:main"})
	other := NewFilterModal(t.Context(), "Issues", target, Filter{Spec: spec})
	if m.Title() != "Filter · Issues" {
		t.Errorf("title = %q, want no subject", m.Title())
	}
	if cmd := m.Update(filterform.AppliedMsg{ID: other.form.ID()}); cmd != nil || len(target.applied) != 0 {
		t.Error("the modal applied another form's message")
	}
	if cmd := m.Update(filterform.CancelMsg{ID: other.form.ID()}); cmd != nil {
		t.Error("the modal closed on another form's cancel")
	}
	cmd := m.Update(filterform.CancelMsg{ID: m.form.ID()})
	if msg, ok := cmd().(CloseModalMsg); !ok || msg.Modal != m {
		t.Errorf("cancel = %#v, want the modal closed", cmd())
	}
	if w, h := m.Fit(300, 300); w != filterWidth || h != 1+filterSpare {
		t.Errorf("Fit = %dx%d, want %dx%d", w, h, filterWidth, 1+filterSpare)
	}
	if w, h := m.Fit(40, 5); w != 40 || h != 5 {
		t.Errorf("Fit in a small screen = %dx%d, want all of it", w, h)
	}
}

func TestFilterModalTabs(t *testing.T) {
	fields := []filterform.Field{{
		Key: "state", Label: "State", Kind: filterform.Choice, Qualifier: "is",
		Options: []filterform.Item{{Label: "Open", Value: "open"}, {Label: "Closed", Value: "closed"}},
	}}
	sort := &filterform.SortField{
		Options: []filterform.SortOption{SortByTime("Updated", "updated"), SortByCount("Comments", "comments")},
		Default: filterform.Sort{By: "updated", Desc: true},
	}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	tests := []struct {
		name       string
		sort       *filterform.SortField
		opts       []FilterOption
		keys       []tea.KeyPressMsg
		wantNames  []string
		wantActive int
	}{
		{name: "opens on the filters", sort: sort, wantNames: []string{"Filters", "Sort"}, wantActive: 0},
		{name: "opens on the sort", sort: sort, opts: []FilterOption{OnTab(filterform.SortTab)}, wantNames: []string{"Filters", "Sort"}, wantActive: 1},
		{
			name: "switches with the keys of next_filter", sort: sort,
			opts:      []FilterOption{WithFormKeys(FilterFormKeys(map[string][]string{config.ActionNextFilter: {"}"}}))},
			keys:      []tea.KeyPressMsg{{Code: ']', Text: "]"}, {Code: '}', Text: "}"}},
			wantNames: []string{"Filters", "Sort"}, wantActive: 1,
		},
		{name: "a list without a sort has no tabs", opts: []FilterOption{OnTab(filterform.SortTab)}, wantActive: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &fakeFilterable{}
			m := NewFilterModal(t.Context(), "Pull requests", target, Filter{Spec: filterform.Spec{Fields: fields, Sort: tt.sort}, Query: "is:closed sort:comments-asc"}, tt.opts...)
			for _, k := range tt.keys {
				m.Update(k)
			}
			names, active := m.Tabs()
			if !slices.Equal(names, tt.wantNames) || active != tt.wantActive {
				t.Errorf("Tabs = %q, %d; want %q, %d", names, active, tt.wantNames, tt.wantActive)
			}
			// Either tab applies the whole query.
			msg := m.Update(enter)()
			if a, ok := msg.(filterform.AppliedMsg); !ok || !strings.HasPrefix(a.Query, "is:closed") {
				t.Errorf("enter sent %#v, want the query applied", msg)
			}
			// The modal keeps its height on either tab: the rows of the
			// longer one.
			rows := len(fields)
			if tt.sort != nil {
				rows = sortRows
			}
			if _, h := m.Fit(300, 300); h != rows+filterSpare {
				t.Errorf("Fit height = %d, want %d", h, rows+filterSpare)
			}
		})
	}
}

func TestSortOptionsReadAlike(t *testing.T) {
	for _, o := range []filterform.SortOption{SortByTime("Updated", "updated"), SortByCount("Stars", "stars"), SortByName("Name", "name")} {
		if o.Desc == "" || o.Asc == "" || strings.ToUpper(o.Desc[:1]) != o.Desc[:1] {
			t.Errorf("%s orders %q and %q, want both named, capitalized", o.Label, o.Desc, o.Asc)
		}
	}
	if BestMatch.Value != "" {
		t.Errorf("best match writes %q, want nothing", BestMatch.Value)
	}
}

func TestChips(t *testing.T) {
	tests := []struct{ query, want string }{
		{"", ""},
		{"author:@me label:bug", "@me · bug"},
		{`author:octocat label:"good first issue",ui -is:draft crash`, `@octocat · good first issue,ui · -is:draft · crash`},
		{"review:approved sort:created-asc", "review:approved · sort:created-asc"},
	}
	for _, tt := range tests {
		if got := Chips(tt.query); got != tt.want {
			t.Errorf("Chips(%q) = %q, want %q", tt.query, got, tt.want)
		}
	}
}

func TestWithout(t *testing.T) {
	drop := func(tok filterform.Token) bool { return tok.Qualifier == "is" && tok.Value == "open" }
	if got := Without(`is:open label:"good first issue" is:draft`, drop); got != `label:"good first issue" is:draft` {
		t.Errorf("Without = %q, want the rest as written", got)
	}
}
