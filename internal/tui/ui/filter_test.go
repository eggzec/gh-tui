package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

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
