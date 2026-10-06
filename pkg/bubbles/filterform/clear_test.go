package filterform

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// stateNoAny is prSpec with a state choice that has no empty option.
func stateNoAny() Spec {
	s := prSpec(nil)
	s.Fields[rowState].Options = slices.Clone(s.Fields[rowState].Options[:3])
	return s
}

// bestMatch is prSpec with a sort option that writes no sort.
func bestMatch() Spec {
	s := prSpec(nil)
	s.Sort.Options = append(slices.Clone(s.Sort.Options), SortOption{Label: "Best match"})
	return s
}

// A choice with no empty option has nothing to clear, so delete and
// backspace leave it and help disables the key; the sort takes the same
// rule for its "sort by" row.
func TestClearNeedsAnEmptyOption(t *testing.T) {
	const noSort = "is:open author:@me review-requested:@me label:bug,enhancement base:main"
	tests := []struct {
		name      string
		spec      Spec
		move      []tea.Msg
		clear     tea.Msg
		wantQuery string
		wantClear bool
	}{
		{name: "choice with no empty option", spec: stateNoAny(), clear: del, wantQuery: prDefaults},
		{name: "backspace on a choice with no empty option", spec: stateNoAny(), clear: bksp, wantQuery: prDefaults},
		{
			name: "choice with an empty option", spec: prSpec(nil), move: keys(down, rowReview), clear: del, wantClear: true,
			wantQuery: "is:open author:@me label:bug,enhancement base:main sort:updated-desc",
		},
		{name: "sort by with an empty option", spec: bestMatch(), move: []tea.Msg{nextTab}, clear: del, wantQuery: noSort, wantClear: true},
		{name: "backspace on sort by with an empty option", spec: bestMatch(), move: []tea.Msg{nextTab}, clear: bksp, wantQuery: noSort, wantClear: true},
		{name: "order has nothing to clear", spec: bestMatch(), move: []tea.Msg{nextTab, down}, clear: del, wantQuery: prDefaults},
		{name: "sort by with no empty option", spec: prSpec(nil), move: []tea.Msg{nextTab}, clear: del, wantQuery: prDefaults},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := press(t, open(t, tt.spec), tt.move...)
			enabled := false
			for _, g := range m.FullHelp() {
				for _, b := range g {
					enabled = enabled || b.Help().Desc == "clear" && b.Enabled()
				}
			}
			if enabled != tt.wantClear {
				t.Errorf("clear enabled in help = %v, want %v", enabled, tt.wantClear)
			}
			m, _ = press(t, m, tt.clear)
			if got := m.Query(); got != tt.wantQuery {
				t.Errorf("Query = %q, want %q", got, tt.wantQuery)
			}
		})
	}
}

// The query line, a text editor and a picker take F as a letter.
func TestFIsTypedWhereTyping(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.Msg
	}{
		{name: "query line", keys: keys(down, rowQuery)},
		{name: "text editor", keys: append(keys(down, rowBase), enter)},
		{name: "picker", keys: append(keys(down, rowAuthor), enter)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, prSpec(nil), WithQuery("is:closed fix"))
			m, _ = press(t, m, tt.keys...)
			m = typeText(t, m, "F")
			switch tt.name {
			case "query line":
				if got := m.query.Value(); got != "is:closed sort:updated-desc fixF" {
					t.Errorf("query line = %q, want F typed at the end", got)
				}
			case "text editor":
				if got := m.text.Value(); got != "mainF" && got != "F" {
					t.Errorf("text = %q, want F typed", got)
				}
			case "picker":
				if got := m.pick.Query().Text; got != "F" {
					t.Errorf("picker query = %q, want F typed", got)
				}
			}
		})
	}
}
