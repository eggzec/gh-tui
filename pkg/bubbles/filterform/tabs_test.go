package filterform

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTabs(t *testing.T) {
	tests := []struct {
		name     string
		spec     func(Loader) Spec
		opts     []Option
		keys     []tea.Msg
		wantTab  Tab
		wantTabs []string
	}{
		{name: "opens on the filters", spec: prSpec, wantTab: FiltersTab, wantTabs: []string{"Filters", "Sort"}},
		{name: "opens on the sort", spec: prSpec, opts: []Option{WithTab(SortTab)}, wantTab: SortTab, wantTabs: []string{"Filters", "Sort"}},
		{name: "] switches", spec: prSpec, keys: []tea.Msg{nextTab}, wantTab: SortTab, wantTabs: []string{"Filters", "Sort"}},
		{name: "[ switches back", spec: prSpec, opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{prevTab}, wantTab: FiltersTab, wantTabs: []string{"Filters", "Sort"}},
		{name: "without a sort there are no tabs", spec: noSortSpec, opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{nextTab, prevTab}, wantTab: FiltersTab},
		{name: "an unknown tab opens the filters", spec: prSpec, opts: []Option{WithTab(Tab(7))}, wantTab: FiltersTab, wantTabs: []string{"Filters", "Sort"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, tt.spec(nil), tt.opts...)
			m, sent := press(t, m, tt.keys...)
			if len(sent) > 0 {
				t.Errorf("sent %+v, want nothing", sent)
			}
			if m.Tab() != tt.wantTab {
				t.Errorf("Tab = %v, want %v", m.Tab(), tt.wantTab)
			}
			if !slices.Equal(m.Tabs(), tt.wantTabs) {
				t.Errorf("Tabs = %q, want %q", m.Tabs(), tt.wantTabs)
			}
		})
	}
}

// Applying from either tab applies both, and gives the same query.
func TestApplyFromEitherTab(t *testing.T) {
	const q = "is:closed label:docs sort:comments-asc fix"
	got := make([]AppliedMsg, 0, 4)
	for _, tab := range []Tab{FiltersTab, SortTab} {
		for _, keys := range [][]tea.Msg{{enter}, {keyBigG, enter}} {
			m := open(t, prSpec(nil), WithQuery(q), WithTab(tab))
			_, sent := press(t, m, keys...)
			if len(sent) != 1 {
				t.Fatalf("sent %+v, want one AppliedMsg", sent)
			}
			a, ok := sent[0].(AppliedMsg)
			if !ok || a.ID != m.ID() {
				t.Fatalf("sent %+v, want this form's AppliedMsg", sent[0])
			}
			got = append(got, a)
		}
	}
	for _, a := range got {
		if a.Query != q || a.Sort != (Sort{By: "comments"}) || a.Values["state"].Text() != "closed" {
			t.Errorf("applied %q, %+v, state %q; want %q, comments ascending, closed", a.Query, a.Sort, a.Values["state"].Text(), q)
		}
	}
}

// Esc steps back out of a picker or insert mode before it closes the form,
// on either tab.
func TestEscStepsBackThenCloses(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		keys []tea.Msg
	}{
		{name: "from the filters", keys: []tea.Msg{down, space, esc}},
		{name: "from the sort", opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{prevTab, down, space, esc, nextTab}},
		{name: "from the query line", opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{keyBigG, keyI, esc}},
		{name: "from a text", keys: append(keys(down, rowBase), keyI, esc)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, prSpec(nil), tt.opts...)
			m, sent := press(t, m, tt.keys...)
			if len(sent) > 0 || m.mode != rowsMode {
				t.Fatalf("sent %+v, mode %v; want the editor closed and nothing sent", sent, m.mode)
			}
			if m.Query() != prDefaults {
				t.Errorf("Query = %q, want the defaults", m.Query())
			}
			_, sent = press(t, m, esc)
			if len(sent) != 1 || sent[0] != (CancelMsg{ID: m.ID()}) {
				t.Errorf("sent %+v, want a CancelMsg", sent)
			}
		})
	}
}

// Switching tabs leaves an open editor, keeping what it chose, and the
// query line.
func TestSetTabClosesTheEditor(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, keys(down, rowBase)...)
	m, _ = press(t, m, keyA)
	m = typeText(t, m, "-x")
	m.SetTab(SortTab)
	if m.mode != rowsMode || m.Capturing() || m.Tab() != SortTab || m.row != sortByRow {
		t.Fatalf("mode %v, capturing %v, tab %v, row %d; want the sort's first row", m.mode, m.Capturing(), m.Tab(), m.row)
	}
	if v, _ := m.Value("base"); v.Text() != "main-x" {
		t.Errorf("base = %q, want what was typed kept", v.Text())
	}
	m, _ = press(t, m, keyBigG, keyI)
	m.SetTab(FiltersTab)
	if m.Capturing() || m.Tab() != FiltersTab || m.row != rowState {
		t.Errorf("capturing %v, tab %v, row %d; want the filters' first row", m.Capturing(), m.Tab(), m.row)
	}
}

// ] and [ are typed where the form takes every key.
func TestTabKeysAreTypedInInputs(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, keys(down, rowBase)...)
	m, _ = press(t, m, keyA, nextTab, prevTab, esc)
	if v, _ := m.Value("base"); v.Text() != "main][" || m.Tab() != FiltersTab {
		t.Errorf("base = %q on %v, want main][ on the filters", v.Text(), m.Tab())
	}
}

func TestTabHelp(t *testing.T) {
	has := func(m Model, desc string) bool {
		for _, b := range m.ShortHelp() {
			if b.Help().Desc == desc {
				return true
			}
		}
		return false
	}
	m := open(t, prSpec(nil))
	if !has(m, "sort") {
		t.Error("the filters' help doesn't offer the sort")
	}
	m, _ = press(t, m, nextTab)
	if !has(m, "filters") {
		t.Error("the sort's help doesn't offer the filters")
	}
	single := open(t, noSortSpec(nil))
	if has(single, "sort") {
		t.Error("a form without tabs offers the sort")
	}
	for _, group := range single.FullHelp() {
		for _, b := range group {
			if b.Enabled() && strings.Contains(b.Help().Desc, "tab") {
				t.Errorf("a form without tabs lists %q", b.Help().Desc)
			}
		}
	}
}

// Choosing an option sorts in its order, whatever the order before: names
// from A, and the rest descending. Best match writes no sort, and keeps
// the order.
func TestChosenOptionSortsInItsOrder(t *testing.T) {
	spec := Spec{Sort: &SortField{
		Options: []SortOption{
			{Label: "Best match"},
			{Label: "Updated", Value: "updated"},
			{Label: "Name", Value: "name", Ascending: true},
		},
		Default: Sort{By: "updated", Desc: true},
	}}
	tests := []struct {
		query string
		keys  []tea.Msg
		want  string
	}{
		{query: "sort:updated-desc", keys: []tea.Msg{right}, want: "sort:name-asc"},
		{query: "sort:updated-asc", keys: []tea.Msg{right}, want: "sort:name-asc"},
		{query: "sort:name-asc", keys: []tea.Msg{left}, want: "sort:updated-desc"},
		{query: "sort:name-desc", keys: []tea.Msg{left}, want: "sort:updated-desc"},
		{query: "sort:updated-asc", keys: []tea.Msg{left, right}, want: "sort:updated-desc"},
		{query: "sort:name-desc", keys: []tea.Msg{right}, want: ""},
	}
	for _, tt := range tests {
		m := open(t, spec, WithQuery(tt.query), WithTab(SortTab))
		m, _ = press(t, m, tt.keys...)
		if got := m.Query(); got != tt.want {
			t.Errorf("%q then %v: Query = %q, want %q", tt.query, tt.keys, got, tt.want)
		}
	}
}
