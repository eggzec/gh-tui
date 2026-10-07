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
	if w, h := m.Fit(300, 300); w != FilterWidth || h != 4 {
		t.Errorf("Fit = %dx%d, want %dx%d", w, h, FilterWidth, 4)
	}
	if w, h := m.Fit(40, 3); w != 40 || h != 3 {
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
		{"offline", fmt.Errorf("list labels: github: GET /repos/o/r/labels: %w", core.ErrOffline), []string{"✗ Can't reach GitHub", "r to retry · esc to close"}},
		{"forbidden", fmt.Errorf("list labels: github: 403 Forbidden: %w", core.ErrForbidden), []string{"✗ You don't have access to eggzec/x", "esc to close"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			load := func(context.Context, string) ([]filterform.Item, error) { return nil, tt.err }
			spec := filterform.Spec{Fields: []filterform.Field{{Key: "labels", Label: "Labels", Kind: filterform.Multi, Qualifier: "label", Load: load}}}
			m := NewFilterModal(t.Context(), "Issues", &fakeFilterable{}, Filter{Spec: spec, Subject: "eggzec/x"}, WithFormVoice(NewVoice(config.Default().Keys, "")))
			m.SetSize(80, 10)
			cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
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
		cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
		if name == "failed" {
			for _, msg := range drain(cmd) {
				m.Update(msg)
			}
		}
		v := ansi.Strip(m.View())
		if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("%s: view isn't ASCII:\n%s", name, v)
		}
		if name == "failed" && !strings.Contains(v, "r to retry - esc to close") {
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
			if _, h := m.Fit(300, 300); h != rows+filterBelow+1 {
				t.Errorf("Fit height = %d, want %d", h, rows+filterBelow+1)
			}
		})
	}
}

// The modal is as tall as its rows and what the form shows under them.
func TestFilterModalFit(t *testing.T) {
	for _, n := range []int{1, 4, 5, 9, 10, 20} {
		fields := make([]filterform.Field, n)
		for i := range fields {
			fields[i] = filterform.Field{Key: fmt.Sprint("f", i), Label: "Field", Kind: filterform.Text, Qualifier: fmt.Sprint("q", i)}
		}
		m := NewFilterModal(t.Context(), "Issues", &fakeFilterable{}, Filter{Spec: filterform.Spec{Fields: fields}})
		want := n + filterBelow + 1
		if _, h := m.Fit(300, 300); h != want {
			t.Errorf("%d rows: Fit height = %d, want %d", n, h, want)
		}
		if _, h := m.Fit(300, 8); h != min(8, want) {
			t.Errorf("%d rows: Fit height = %d in a short screen, want %d", n, h, min(8, want))
		}
	}
}

// The form shows its own help line, so the modal gives the footer no short
// keys, and the layer still holds every key for the help.
func TestFilterModalKeyLayers(t *testing.T) {
	spec := filterform.Spec{Fields: []filterform.Field{{Key: "base", Label: "Base", Kind: filterform.Text, Qualifier: "base"}}}
	m := NewFilterModal(t.Context(), "Issues", &fakeFilterable{}, Filter{Spec: spec})
	m.SetSize(80, 13)
	layers := m.KeyLayers()
	if len(layers) != 1 || len(layers[0].Short) != 0 || len(layers[0].Bindings) == 0 || layers[0].Typing {
		t.Fatalf("layers = %+v, want one with keys, no short help and no typing", layers)
	}
	if !strings.Contains(ansi.Strip(m.View()), "esc close") {
		t.Errorf("View() = %q, want the form's own help line", ansi.Strip(m.View()))
	}
	m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if layers := m.KeyLayers(); !layers[0].Typing || len(layers[0].Short) != 0 {
		t.Errorf("layer in insert mode = %+v, want typing and no short help", layers[0])
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

// labelsSpec is a form of one row of each kind, whose labels load nine
// options, and that sorts.
func labelsSpec() filterform.Spec {
	load := func(context.Context, string) ([]filterform.Item, error) {
		items := make([]filterform.Item, 9)
		for i := range items {
			items[i] = filterform.Item{Label: fmt.Sprint("label-", i), Value: fmt.Sprint("label-", i)}
		}
		return items, nil
	}
	return filterform.Spec{
		Fields: []filterform.Field{
			{Key: "state", Label: "State", Kind: filterform.Choice, Qualifier: "is", Options: []filterform.Item{{Label: "Open", Value: "open"}, {Label: "Closed", Value: "closed"}}},
			{Key: "author", Label: "Author", Kind: filterform.Person, Qualifier: "author", Options: []filterform.Item{{Label: "@me", Value: "@me"}, {Label: "octocat", Value: "octocat"}}},
			{Key: "labels", Label: "Labels", Kind: filterform.Multi, Qualifier: "label", Load: load},
			{Key: "drafts", Label: "Drafts", Kind: filterform.Toggle, Qualifier: "-is:draft", Hint: "hide drafts"},
			{Key: "base", Label: "Base", Kind: filterform.Text, Qualifier: "base", Hint: "any branch"},
		},
		Sort: &filterform.SortField{Options: []filterform.SortOption{{Label: "Updated", Value: "updated"}, {Label: "Created", Value: "created"}}},
	}
}

// keyOf returns the press of the key named by one character.
func keyOf(s string) tea.Msg {
	if s == "space" {
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	if s == "down" {
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	if s == "enter" {
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	if s == "esc" {
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

// send gives the modal the keys, and what the commands they start send.
func send(m *FilterModal, keys ...string) {
	for _, k := range keys {
		for _, msg := range drain(m.Update(keyOf(k))) {
			m.Update(msg)
		}
	}
}

// With the ASCII icons the help line of the form, in every mode, names its
// keys in words and cuts with the ASCII ellipsis.
func TestFilterModalHelpASCII(t *testing.T) {
	ic := NewIcons(config.IconsASCII)
	p, _ := config.Default().Palette(true)
	steps := []struct {
		name string
		keys []string
		want string
	}{
		{"choice", nil, "h/l change"},
		{"person list", []string{"j"}, "space list"},
		{"person list open", []string{"space"}, "j/k move"},
		{"person filter", []string{"i"}, "up/down move"},
		{"labels list open", []string{"esc", "esc", "j", "space"}, "j/k move"},
		{"labels filter", []string{"i"}, "up/down move"},
		{"toggle", []string{"esc", "esc", "j"}, "space toggle"},
		{"text", []string{"j"}, "i insert"},
		{"insert", []string{"i"}, "INSERT"},
		{"sort tab", []string{"esc", "]"}, "enter apply"},
	}
	for _, width := range []int{100, 40} {
		m := NewFilterModal(t.Context(), "Issues", &fakeFilterable{}, Filter{Spec: labelsSpec(), Subject: "o/r"}, WithFormIcons(ic))
		m.SetTheme(NewTheme(p, true))
		m.SetSize(width, 24)
		for _, st := range steps {
			send(m, st.keys...)
			v := ansi.Strip(m.View())
			if bad := nonASCII(v); bad != "" {
				t.Errorf("%d wide, %s: the view draws %q:\n%s", width, st.name, bad, v)
			}
			lines := strings.Split(v, "\n")
			help := lines[len(lines)-1]
			if width == 100 && !strings.Contains(help, st.want) {
				t.Errorf("%s: the help line is %q, want %q in it", st.name, help, st.want)
			}
		}
	}
}

// nonASCII returns the runes of s that aren't ASCII.
func nonASCII(s string) string {
	var out []rune
	for _, r := range s {
		if r > unicode.MaxASCII {
			out = append(out, r)
		}
	}
	return string(out)
}

// An open picker gets the room it needs: the modal grows by its height,
// enough for every label, and the rows above it stay in view.
func TestFilterModalGrowsForThePicker(t *testing.T) {
	m := NewFilterModal(t.Context(), "Issues", &fakeFilterable{}, Filter{Spec: labelsSpec(), Subject: "o/r"})
	p, _ := config.Default().Palette(true)
	m.SetTheme(NewTheme(p, true))
	_, closed := m.Fit(120, 30)
	send(m, "j", "j", "space")
	_, open := m.Fit(120, 30)
	// The dropdown floats over the rows below its row, the rule and the
	// query, and shows eight labels with its filter line and frame, 12
	// lines. Under the Labels row there are the two rows, the rule and
	// the query line: the modal grows by what is missing.
	if want := closed + 12 - (2 + 1 + 1); open != want {
		t.Errorf("Fit with the picker open = %d, want %d (%d closed)", open, want, closed)
	}
	m.SetSize(100, open)
	v := ansi.Strip(m.View())
	for _, want := range []string{"State", "Author", "label-0", "label-7"} {
		if !strings.Contains(v, want) {
			t.Errorf("the view lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "label-8") || !strings.Contains(strings.Split(v, "\n")[open-1], "j/k") {
		t.Errorf("the list shows more than eight labels, or covers the help line:\n%s", v)
	}
	send(m, "esc")
	if _, h := m.Fit(120, 30); h != closed {
		t.Errorf("Fit after the picker closes = %d, want %d", h, closed)
	}
}

// space in a Person's list marks the person it chose, as a Labels list
// marks what it checked.
func TestFilterModalMarksThePerson(t *testing.T) {
	m := NewFilterModal(t.Context(), "Issues", &fakeFilterable{}, Filter{Spec: labelsSpec(), Subject: "o/r"})
	m.SetSize(100, 24)
	send(m, "j", "space", "down", "enter", "space")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "● octocat") || strings.Contains(v, "● @me") {
		t.Errorf("the list doesn't mark octocat alone:\n%s", v)
	}
}

// A search in a Person's list marks the person chosen now, not the one
// chosen when the search began.
func TestFilterModalMarksThePersonInSearchResults(t *testing.T) {
	people := []filterform.Item{{Label: "mona", Value: "mona"}, {Label: "octomona", Value: "octomona"}}
	spec := filterform.Spec{Fields: []filterform.Field{{
		Key: "author", Label: "Author", Kind: filterform.Person, Qualifier: "author", Options: people,
		Load: func(context.Context, string) ([]filterform.Item, error) { return people, nil },
	}}}
	m := NewFilterModal(t.Context(), "Issues", &fakeFilterable{}, Filter{Spec: spec, Subject: "o/r"})
	m.SetSize(100, 24)
	send(m, "space", "down", "enter", "space", "i", "m", "o", "n", "a")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "● octomona") || strings.Contains(v, "● mona") {
		t.Errorf("the results don't mark octomona alone:\n%s", v)
	}
}

// The query takes one line when it fits and a second when it wraps, and the
// modal is as tall.
func TestFilterModalFitsTheQuery(t *testing.T) {
	height := func(query string) int {
		m := NewFilterModal(t.Context(), "Issues", &fakeFilterable{}, Filter{Spec: labelsSpec(), Query: query})
		_, h := m.Fit(300, 300)
		return h
	}
	short := height("is:open")
	if want := 5 + filterBelow + 1; short != want {
		t.Errorf("a short query: Fit height = %d, want %d", short, want)
	}
	words := make([]string, 30)
	for i := range words {
		words[i] = fmt.Sprint("word", i)
	}
	long := height("is:open " + strings.Join(words, " "))
	if long != short+1 {
		t.Errorf("a wrapping query: Fit height = %d, want %d", long, short+1)
	}
}
