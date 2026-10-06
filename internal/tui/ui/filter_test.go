package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
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

// A field whose options failed to load says what went wrong in the
// voice, naming the key that loads them again, and no key the form lacks.
func TestFilterModalErrorWords(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{"offline", fmt.Errorf("list labels: github: GET /repos/o/r/labels: %w", core.ErrOffline), []string{"✗ Can't reach GitHub", "↵ to retry · esc to go back"}},
		{"forbidden", fmt.Errorf("list labels: github: 403 Forbidden: %w", core.ErrForbidden), []string{"✗ You don't have access to eggzec/x", "esc to go back"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			load := func(context.Context, string) ([]filterform.Item, error) { return nil, tt.err }
			spec := filterform.Spec{Fields: []filterform.Field{{Key: "labels", Label: "Labels", Kind: filterform.Multi, Qualifier: "label", Load: load}}}
			m := NewFilterModal(t.Context(), "Issues", &fakeFilterable{}, Filter{Spec: spec, Subject: "eggzec/x"}, WithFormVoice(NewVoice(config.Default().Keys, "")))
			m.SetSize(80, 10)
			cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			for _, msg := range drain(cmd) {
				m.Update(msg)
			}
			v := ansi.Strip(m.View())
			for _, want := range tt.want {
				if !strings.Contains(v, want) || strings.Contains(v, "github") || strings.Contains(v, "to open") {
					t.Errorf("View() = %q, want %q", v, want)
				}
			}
		})
	}
}

// With the ASCII icons the filter modal is ASCII alone: its title, the
// form, the spinner while options load, and the words of a failed load,
// which name the keys in words.
func TestFilterModalASCII(t *testing.T) {
	ic := NewIcons(config.IconsASCII)
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	loads := map[string]filterform.Loader{
		"loading": func(ctx context.Context, _ string) ([]filterform.Item, error) {
			select {
			case <-block:
			case <-ctx.Done():
			}
			return nil, ctx.Err()
		},
		"failed": func(context.Context, string) ([]filterform.Item, error) {
			return nil, fmt.Errorf("list labels: %w", core.ErrOffline)
		},
	}
	for name, load := range loads {
		spec := filterform.Spec{Fields: []filterform.Field{{Key: "labels", Label: "Labels", Kind: filterform.Multi, Qualifier: "label", Load: load}}}
		m := NewFilterModal(t.Context(), "Issues", &fakeFilterable{}, Filter{Spec: spec, Subject: "eggzec/x"},
			WithFormVoice(NewVoice(config.Default().Keys, "")), WithFormIcons(ic))
		p, _ := config.Default().Palette(true)
		m.SetTheme(NewTheme(p, true))
		m.SetSize(80, 10)
		if got, want := m.Title(), "Filter - Issues - eggzec/x"; got != want {
			t.Errorf("%s: title = %q, want %q", name, got, want)
		}
		cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if name == "failed" {
			for _, msg := range drain(cmd) {
				m.Update(msg)
			}
		}
		v := ansi.Strip(m.View())
		if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("%s: view isn't ASCII:\n%s", name, v)
		}
		if name == "failed" && !strings.Contains(v, "enter to retry - esc to go back") {
			t.Errorf("%s: view doesn't name the keys in words:\n%s", name, v)
		}
	}
}

// drain runs cmd and the commands of its batches, and returns what they
// sent, save the spinner's ticks.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, drain(c)...)
		}
		return out
	case spinner.TickMsg:
		return nil
	default:
		return []tea.Msg{msg}
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
			name: "switches with the keys of next_tab", sort: sort,
			opts:      []FilterOption{WithFormKeys(FilterFormKeys(config.Keymap{config.ContextGlobal: {"next_tab": {"}"}}}))},
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
		if got := Chips(tt.query, " · "); got != tt.want {
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
