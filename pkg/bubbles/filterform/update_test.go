package filterform

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestUpdate(t *testing.T) {
	type step = []tea.Msg
	tests := []struct {
		name string
		// query, when set, is where the form starts.
		query string
		keys  []tea.Msg
		typed string
		// after is pressed after typed.
		after     []tea.Msg
		wantQuery string
		wantTab   Tab
		wantRow   int
	}{
		{
			name: "right picks the next choice", keys: step{right},
			wantQuery: strings.Replace(prDefaults, "is:open", "is:closed", 1),
		},
		{
			name: "left wraps to the last choice", keys: step{left},
			wantQuery: strings.Replace(prDefaults, "is:open ", "", 1),
		},
		{
			name: "esc closes the list of a choice as it was", keys: step{space, down, esc},
			wantQuery: prDefaults,
		},
		{
			name: "whole-token choice", keys: step{down, down, right},
			wantQuery: strings.Replace(prDefaults, "review-requested:@me", "review:approved", 1),
			wantRow:   rowReview,
		},
		{
			name: "space flips a toggle", keys: append(keys(down, rowDrafts), space),
			wantQuery: strings.Replace(prDefaults, " base:main", " -is:draft base:main", 1),
			wantRow:   rowDrafts,
		},
		{
			name: "left flips a toggle too", keys: append(keys(down, rowDrafts), left, left),
			wantQuery: prDefaults, wantRow: rowDrafts,
		},
		{
			name: "delete turns a toggle off", query: "-is:draft", keys: append(keys(down, rowDrafts), del),
			wantQuery: "sort:updated-desc", wantRow: rowDrafts,
		},
		{
			name: "] shows the sort", keys: step{down, nextTab},
			wantQuery: prDefaults, wantTab: SortTab, wantRow: sortByRow,
		},
		{
			name: "] wraps back to the filters", keys: step{nextTab, down, nextTab},
			wantQuery: prDefaults, wantTab: FiltersTab, wantRow: rowState,
		},
		{
			name: "[ goes back to the filters", keys: step{nextTab, prevTab},
			wantQuery: prDefaults, wantTab: FiltersTab, wantRow: rowState,
		},
		{
			name: "right sorts by the next option", keys: step{nextTab, right},
			wantQuery: strings.Replace(prDefaults, "sort:updated-desc", "sort:created-desc", 1),
			wantTab:   SortTab, wantRow: sortByRow,
		},
		{
			name: "left wraps to the last option", keys: step{nextTab, left},
			wantQuery: strings.Replace(prDefaults, "sort:updated-desc", "sort:comments-desc", 1),
			wantTab:   SortTab, wantRow: sortByRow,
		},
		{
			name: "esc closes the list of the sort as it was", keys: step{nextTab, space, down, esc},
			wantQuery: prDefaults, wantTab: SortTab, wantRow: sortByRow,
		},
		{
			name: "right flips the order", keys: step{nextTab, down, right},
			wantQuery: strings.Replace(prDefaults, "desc", "asc", 1), wantTab: SortTab, wantRow: sortOrderRow,
		},
		{
			name: "esc closes the list of the order as it was", keys: step{nextTab, down, space, down, esc},
			wantQuery: prDefaults, wantTab: SortTab, wantRow: sortOrderRow,
		},
		{
			name: "the next option sorts in its own order", keys: step{nextTab, down, right, up, right},
			wantQuery: strings.Replace(prDefaults, "sort:updated-desc", "sort:created-desc", 1),
			wantTab:   SortTab, wantRow: sortByRow,
		},
		{
			name: "delete clears nothing on a sort with no empty option", keys: step{nextTab, del, down, bksp},
			wantQuery: prDefaults, wantTab: SortTab, wantRow: sortOrderRow,
		},
		{
			name: "F does nothing on a sort row", query: "is:closed sort:comments-asc fix", keys: step{nextTab, keyF, down, keyF},
			wantQuery: "is:closed sort:comments-asc fix", wantTab: SortTab, wantRow: sortOrderRow,
		},
		{
			name: "F does nothing on a filter row", query: "is:closed sort:comments-asc fix", keys: step{keyF, down, keyF},
			wantQuery: "is:closed sort:comments-asc fix", wantRow: rowAuthor,
		},
		{
			name: "up on the sort stops at the first row", keys: step{nextTab, up},
			wantQuery: prDefaults, wantTab: SortTab, wantRow: sortByRow,
		},
		{
			name: "G goes to the query line, where ] is typed", keys: step{nextTab, keyBigG, keyA},
			typed: " ]", after: step{esc}, wantQuery: prDefaults + " ]", wantTab: SortTab, wantRow: sortRows,
		},
		{
			name: "delete unchecks every label", keys: step{down, down, down, del},
			wantQuery: strings.Replace(prDefaults, "label:bug,enhancement ", "", 1),
			wantRow:   rowLabels,
		},
		{
			name: "backspace unchecks every label", keys: step{down, down, down, bksp},
			wantQuery: strings.Replace(prDefaults, "label:bug,enhancement ", "", 1),
			wantRow:   rowLabels,
		},
		{
			name: "delete clears a choice", keys: step{del},
			wantQuery: strings.Replace(prDefaults, "is:open ", "", 1),
		},
		{
			name: "backspace clears a choice", keys: step{bksp},
			wantQuery: strings.Replace(prDefaults, "is:open ", "", 1),
		},
		{
			name: "x does nothing in the rows", keys: step{keyX, down, keyX, down, down, keyX},
			wantQuery: prDefaults, wantRow: rowLabels,
		},
		{
			name: "r does nothing in the rows", query: "is:closed fix", keys: step{keyR, down, keyR},
			wantQuery: "is:closed sort:updated-desc fix", wantRow: rowAuthor,
		},
		{
			name: "delete clears a person", keys: step{down, del},
			wantQuery: strings.Replace(prDefaults, "author:@me ", "", 1), wantRow: rowAuthor,
		},
		{
			name: "a edits a text from its end", keys: append(keys(down, rowBase), keyA, bksp, bksp, bksp, bksp),
			typed: "release 1.0", after: step{esc},
			wantQuery: strings.Replace(prDefaults, "base:main", `base:"release 1.0"`, 1), wantRow: rowBase,
		},
		{
			name: "esc keeps a text", keys: append(keys(down, rowBase), keyA),
			typed: "-x", after: step{esc},
			wantQuery: strings.Replace(prDefaults, "base:main", "base:main-x", 1), wantRow: rowBase,
		},
		{
			name: "a person takes the highlighted login", keys: step{down, del, space, keyI},
			typed: "octo", after: step{enter},
			wantQuery: strings.Replace(prDefaults, "@me", "octocat", 1), wantRow: rowAuthor,
		},
		{
			name: "a person takes a typed login nothing matches", keys: step{down, space, keyI},
			typed: "hubot", after: step{enter},
			wantQuery: strings.Replace(prDefaults, "author:@me", "author:hubot", 1), wantRow: rowAuthor,
		},
		{
			name: "esc leaves a person as it was", keys: step{down, space, keyI},
			typed: "hubot", after: step{esc, esc},
			wantQuery: prDefaults, wantRow: rowAuthor,
		},
		{
			name: "G goes to the query line, where i types", keys: step{keyBigG, keyA},
			typed: " fix x r", after: step{esc}, wantQuery: prDefaults + " fix x r", wantRow: rowQuery,
		},
		{
			name: "the query line sets the fields as it is typed", query: "is:open", keys: step{keyBigG, keyA, ctrlU},
			typed: "is:closed label:docs", after: step{esc, keyG},
			wantQuery: "is:closed label:docs sort:updated-desc", wantRow: rowState,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			if tt.query != "" {
				opts = append(opts, WithQuery(tt.query))
			}
			m := open(t, prSpec(nil), opts...)
			m, sent := press(t, m, tt.keys...)
			m = typeText(t, m, tt.typed)
			m, more := press(t, m, tt.after...)
			if sent = append(sent, more...); len(sent) > 0 {
				t.Errorf("sent %+v, want nothing", sent)
			}
			if got := m.Query(); got != tt.wantQuery {
				t.Errorf("Query = %q, want %q", got, tt.wantQuery)
			}
			if m.Tab() != tt.wantTab || m.row != tt.wantRow {
				t.Errorf("tab, row = %v, %d, want %v, %d", m.Tab(), m.row, tt.wantTab, tt.wantRow)
			}
			if m.mode != rowsMode {
				t.Error("an editor is still open")
			}
		})
	}
}

// Typing on the query line leaves what was typed alone, and it is
// normalized once insert mode ends.
func TestQueryLineKeepsTyping(t *testing.T) {
	m := open(t, prSpec(nil), WithQuery(""))
	m, _ = press(t, m, keyBigG, keyA, ctrlU)
	m = typeText(t, m, "label:  is:closed  ")
	if got := m.query.Value(); got != "label:  is:closed  " {
		t.Errorf("query line = %q, want what was typed", got)
	}
	if v, _ := m.Value("state"); v.Text() != "closed" {
		t.Errorf("state = %q, want closed", v.Text())
	}
	m, _ = press(t, m, esc)
	if got, want := m.query.Value(), "is:closed sort:updated-desc label:"; got != want {
		t.Errorf("query line = %q, want %q", got, want)
	}
}

func TestMultiEditor(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.Msg
		want []string
	}{
		{name: "enter adds the highlighted item", keys: []tea.Msg{down, down, enter}, want: []string{"bug", "enhancement", "docs"}},
		{name: "enter doesn't remove a chosen item", keys: []tea.Msg{enter}, want: []string{"bug", "enhancement"}},
		{name: "enter after space keeps the item space unchecked", keys: []tea.Msg{space, enter}, want: []string{"enhancement"}},
		{name: "enter adds the item below the one space checked", keys: []tea.Msg{space, down, down, enter}, want: []string{"enhancement", "docs"}},
		{
			name: "space chooses several and enter only closes",
			keys: []tea.Msg{down, down, space, down, space, up, enter},
			want: []string{"bug", "enhancement", "docs", "good first issue"},
		},
		{name: "esc undoes what space checked", keys: []tea.Msg{down, down, space, esc}, want: []string{"bug", "enhancement"}},
		{name: "enter keeps what space checked", keys: []tea.Msg{down, down, space, enter}, want: []string{"bug", "enhancement", "docs"}},
		{name: "esc undoes a clear", keys: []tea.Msg{del, esc}, want: []string{"bug", "enhancement"}},
		{name: "enter after a clear adds nothing", keys: []tea.Msg{del, enter}, want: nil},
		{name: "enter after a clear and a move adds the item", keys: []tea.Msg{del, keyJ, enter}, want: []string{"enhancement"}},
		{name: "esc leaves the highlighted item", keys: []tea.Msg{down, down, esc}, want: []string{"bug", "enhancement"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLoader{}
			m := open(t, prSpec(f.load))
			m, _ = press(t, m, down, down, down, space)
			if !m.picking || m.mode != listMode {
				t.Fatalf("picking = %v, mode = %v after space; want an open list", m.picking, m.mode)
			}
			m, sent := press(t, m, tt.keys...)
			if len(sent) > 0 {
				t.Errorf("sent %+v, want nothing", sent)
			}
			if v, _ := m.Value("labels"); !slices.Equal(v.List(), tt.want) {
				t.Errorf("labels = %q, want %q", v.List(), tt.want)
			}
			if m.mode != rowsMode || m.Capturing() {
				t.Error("the list is still open")
			}
		})
	}
}

// Space marks the item it checked, and the highlight stays on it.
func TestMultiEditorMarks(t *testing.T) {
	f := &fakeLoader{}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down, space, down, down, space)
	it, ok := m.pick.Selected()
	if !ok || it.Value != "docs" || !strings.Contains(ansi.Strip(m.pick.View()), "[x] docs") {
		t.Errorf("selected %+v, want docs marked as checked:\n%s", it, m.pick.View())
	}
	if got := m.query.Value(); !strings.Contains(got, "label:bug,enhancement,docs") {
		t.Errorf("query line = %q, want it to follow the list", got)
	}
}

func TestLoad(t *testing.T) {
	f := &fakeLoader{fail: errBoom}
	m := open(t, prSpec(f.load))
	if f.calls() != 0 {
		t.Fatal("the loader was called before the field was opened")
	}
	m, _ = press(t, m, down, down, down)
	var cmd tea.Cmd
	m, cmd = m.Update(space)
	if !m.Loading() || m.fields[rowLabels].state != loading {
		t.Error("opening the field didn't start loading")
	}
	m, _ = run(t, m, cmd)
	if m.fields[rowLabels].state != failed || m.picking {
		t.Fatalf("state = %v, picking = %v; want a failed load", m.fields[rowLabels].state, m.picking)
	}
	if !strings.Contains(m.View(), "Couldn't load labels: github: 502 Bad Gateway") {
		t.Errorf("the error isn't shown:\n%s", m.View())
	}
	// Space does nothing while there is nothing to pick from.
	m, _ = press(t, m, space)
	f.setFail(nil)
	m, _ = press(t, m, keyR)
	if !m.picking || m.pick.Len() != len(labels) {
		t.Fatalf("picking = %v with %d items after retrying; want the labels", m.picking, m.pick.Len())
	}
	m, _ = press(t, m, esc, space)
	if f.calls() != 2 {
		t.Errorf("the loader was called %d times, want 2: once failing, once again", f.calls())
	}
	if !m.picking {
		t.Error("opening the field again didn't show the loaded labels")
	}
}

// TestRetry checks that Retry loads again the options that failed, and
// does nothing when none did.
func TestRetry(t *testing.T) {
	f := &fakeLoader{fail: errBoom}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down, space)
	if !errors.Is(m.Err(), errBoom) {
		t.Fatalf("Err() = %v, want %v", m.Err(), errBoom)
	}
	f.setFail(nil)
	m, _ = run(t, m, m.Retry())
	if m.Err() != nil || m.fields[rowLabels].state != loaded {
		t.Fatalf("after Retry: Err() = %v, state = %v", m.Err(), m.fields[rowLabels].state)
	}
	if !m.picking {
		t.Error("the editor waiting for the options didn't open them")
	}
	if cmd := m.Retry(); cmd != nil {
		t.Error("Retry with nothing failed returned a command")
	}
	if f.calls() != 2 {
		t.Errorf("the loader was called %d times, want 2", f.calls())
	}
}

func TestLoadIgnoresOtherForms(t *testing.T) {
	f := &fakeLoader{}
	a, b := open(t, prSpec(f.load)), open(t, prSpec(f.load))
	a, _ = press(t, a, down, down, down)
	a, cmd := a.Update(space)
	b, _ = run(t, b, cmd)
	if b.fields[rowLabels].state != notLoaded {
		t.Error("a load for one form landed in another")
	}
	a, _ = run(t, a, cmd)
	if !a.picking {
		t.Error("the load didn't land in its own form")
	}
}

// Blurring cancels the load in flight and drops what it returns.
func TestBlurCancelsLoad(t *testing.T) {
	f := &fakeLoader{block: make(chan struct{})}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down)
	m, cmd := m.Update(space)
	ctx := m.ctx
	m.Blur()
	if ctx.Err() == nil {
		t.Error("Blur didn't cancel the load")
	}
	close(f.block)
	m, _ = run(t, m, cmd)
	if m.fields[rowLabels].state != notLoaded || m.mode != rowsMode {
		t.Errorf("state = %v, mode = %v; want the load dropped", m.fields[rowLabels].state, m.mode)
	}
}

func TestErrorText(t *testing.T) {
	offline := func(error) (string, string) { return "Can't reach GitHub", "↵ to retry" }
	tests := []struct {
		name   string
		opts   []Option
		lines  []string
		closed string
	}{
		{"default", nil, []string{"✗ Couldn't load labels: github: 502 Bad Gateway", "r to retry · esc to close"}, "✗ couldn't load"},
		{"custom", []Option{WithErrorText(offline)}, []string{"✗ Can't reach GitHub", "↵ to retry · esc to close"}, "✗ couldn't load"},
		{"custom without a hint", []Option{WithErrorText(func(error) (string, string) { return "No access to o/r", "" })}, []string{"✗ No access to o/r", "esc to close"}, "✗ couldn't load"},
		{"empty", []Option{WithErrorText(func(error) (string, string) { return "", "" })}, []string{"esc to close"}, ""},
		{"text with a dot", []Option{WithErrorText(func(error) (string, string) { return "GitHub says a · b", "" })}, []string{"✗ GitHub says a · b", "esc to close"}, "✗ couldn't load"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLoader{fail: errBoom}
			m := open(t, prSpec(f.load), tt.opts...)
			m, _ = press(t, m, down, down, down, space)
			var got []string
			for _, l := range m.statusLines() {
				got = append(got, strings.TrimSpace(ansi.Strip(l)))
			}
			if !slices.Equal(got, tt.lines) {
				t.Errorf("dropdown = %q, want %q", got, tt.lines)
			}
			m, _ = press(t, m, esc)
			var closed []string
			for _, l := range m.loadSegment(rowLabels) {
				closed = append(closed, ansi.Strip(l))
			}
			if strings.Join(closed, "") != tt.closed {
				t.Errorf("closed row = %q, want %q", closed, tt.closed)
			}
		})
	}
}

// The picker of a Person says a failed search in the words of the error
// text, without its hint, since typing searches again.
func TestPersonSearchErrorText(t *testing.T) {
	spec := Spec{Fields: []Field{{
		Key: "assignee", Label: "Assignee", Kind: Person, Qualifier: "assignee",
		Load: func(_ context.Context, q string) ([]Item, error) {
			if q == "" {
				return []Item{{Label: "@me", Value: "@me"}}, nil
			}
			return nil, errBoom
		},
	}}}
	m := open(t, spec, WithErrorText(func(error) (string, string) { return "Can't reach GitHub", "↵ to retry" }))
	m, _ = press(t, m, space, keyI)
	m = typeText(t, m, "hu")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "✗ Can't reach GitHub") || strings.Contains(v, "to retry") || strings.Contains(v, "502") {
		t.Errorf("View() = %q, want the error text without its hint", v)
	}
}

// A Person with a Loader searches with it for what the user types.
func TestPersonSearch(t *testing.T) {
	var queries []string
	spec := Spec{Fields: []Field{{
		Key: "assignee", Label: "Assignee", Kind: Person, Qualifier: "assignee",
		Load: func(_ context.Context, q string) ([]Item, error) {
			queries = append(queries, q)
			if q == "" {
				return []Item{{Label: "@me", Value: "@me"}}, nil
			}
			return []Item{{Label: "hubot", Value: "hubot", Detail: "Hubot"}}, nil
		},
	}}}
	m := open(t, spec)
	m, _ = press(t, m, space, keyI)
	m = typeText(t, m, "hu")
	m, _ = press(t, m, enter)
	if got := m.Query(); got != "assignee:hubot" {
		t.Errorf("Query = %q, want assignee:hubot", got)
	}
	if len(queries) == 0 || queries[0] != "" || queries[len(queries)-1] != "hu" {
		t.Errorf("queries = %q, want the empty one first and hu last", queries)
	}
}

func TestApplyAndCancel(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.Msg
		want tea.Msg
	}{
		{name: "enter on a choice applies", keys: []tea.Msg{right, enter}},
		{name: "enter on the query line applies", keys: []tea.Msg{right, keyBigG, enter}},
		{name: "enter on a toggle applies", keys: []tea.Msg{right, down, down, down, down, enter}},
		{name: "esc cancels", keys: []tea.Msg{right, esc}, want: CancelMsg{}},
		{name: "q cancels", keys: []tea.Msg{right, keyQ}, want: CancelMsg{}},
		{name: "esc on the query line cancels", keys: []tea.Msg{right, keyBigG, esc}, want: CancelMsg{}},
		{name: "esc closes the picker first", keys: []tea.Msg{right, down, down, down, space, esc, esc}, want: CancelMsg{}},
		{name: "esc leaves insert first", keys: []tea.Msg{right, keyBigG, keyA, esc, esc}, want: CancelMsg{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, prSpec(nil))
			m, sent := press(t, m, tt.keys...)
			if len(sent) != 1 {
				t.Fatalf("sent %+v, want one message", sent)
			}
			switch msg := sent[0].(type) {
			case AppliedMsg:
				if tt.want != nil {
					t.Fatalf("sent %+v, want %T", msg, tt.want)
				}
				want := strings.Replace(prDefaults, "is:open", "is:closed", 1)
				if msg.ID != m.ID() || msg.Query != want || msg.Sort != (Sort{By: "updated", Desc: true}) {
					t.Errorf("applied %+v, want query %q", msg, want)
				}
				if v := msg.Values["state"]; v.Text() != "closed" || !equalValues(msg.Values, m.Values()) {
					t.Errorf("values = %+v, want the form's", msg.Values)
				}
			case CancelMsg:
				if tt.want == nil || msg.ID != m.ID() {
					t.Errorf("sent %+v, want an AppliedMsg", msg)
				}
			}
		})
	}
}

// Enter applies from every kind of row and from the query line, on either
// tab, and no row opens an editor with it.
func TestEnterAppliesFromEveryRow(t *testing.T) {
	tests := []struct {
		name string
		tab  Tab
		row  int
	}{
		{name: "choice", row: rowState},
		{name: "person", row: rowAuthor},
		{name: "choice with a whole token", row: rowReview},
		{name: "multi", row: rowLabels},
		{name: "toggle", row: rowDrafts},
		{name: "text", row: rowBase},
		{name: "query", row: rowQuery},
		{name: "sort by", tab: SortTab, row: sortByRow},
		{name: "order", tab: SortTab, row: sortOrderRow},
		{name: "sort query", tab: SortTab, row: sortRows},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, prSpec(nil), WithTab(tt.tab))
			m, _ = press(t, m, keys(down, tt.row)...)
			if m.row != tt.row {
				t.Fatalf("row = %d, want %d", m.row, tt.row)
			}
			m, sent := press(t, m, enter)
			if len(sent) != 1 || m.mode != rowsMode {
				t.Fatalf("sent %v, mode %v; want one AppliedMsg and the rows", sent, m.mode)
			}
			if a, ok := sent[0].(AppliedMsg); !ok || a.ID != m.ID() || a.Query != prDefaults {
				t.Errorf("sent %+v, want this form's AppliedMsg of the defaults", sent[0])
			}
		})
	}
}

// Space opens the dropdown of a choice, a Multi, a Person, what is sorted
// by and the order, and does nothing on a text or the query line. The keys
// of a dropdown in its normal mode are not captured.
func TestSpaceOpensLists(t *testing.T) {
	tests := []struct {
		name string
		tab  Tab
		row  int
		open bool
	}{
		{name: "person", row: rowAuthor, open: true},
		{name: "multi", row: rowLabels, open: true},
		{name: "choice", row: rowState, open: true},
		{name: "text", row: rowBase},
		{name: "query", row: rowQuery},
		{name: "sort by", tab: SortTab, row: sortByRow, open: true},
		{name: "order", tab: SortTab, row: sortOrderRow, open: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, prSpec(nil), WithTab(tt.tab))
			m, _ = press(t, m, keys(down, tt.row)...)
			m, sent := press(t, m, space)
			if len(sent) > 0 || (m.mode == listMode) != tt.open || m.Capturing() {
				t.Errorf("sent %v, mode %v, Capturing %v; want a list open: %v", sent, m.mode, m.Capturing(), tt.open)
			}
			if m.picking != tt.open {
				t.Errorf("picking = %v, want %v", m.picking, tt.open)
			}
			if m.Query() != prDefaults {
				t.Errorf("Query = %q, want the defaults", m.Query())
			}
		})
	}
}

// j and k move between the rows and the query line, and stop at the ends;
// g and G jump to the first row and the query line.
func TestRowsMoveWithJK(t *testing.T) {
	m := open(t, prSpec(nil))
	moves := []struct {
		key  tea.Msg
		want int
	}{
		{keyK, rowState},
		{up, rowState},
		{keyJ, rowAuthor},
		{down, rowReview},
		{keyBigG, rowQuery},
		{keyJ, rowQuery},
		{down, rowQuery},
		{keyK, rowBase},
		{keyG, rowState},
		{keyK, rowState},
		{tea.KeyPressMsg{Code: tea.KeyEnd}, rowQuery},
		{tea.KeyPressMsg{Code: tea.KeyHome}, rowState},
	}
	for _, mv := range moves {
		m, _ = press(t, m, mv.key)
		if m.row != mv.want {
			t.Fatalf("after %v: row = %d, want %d", mv.key, m.row, mv.want)
		}
	}
	// The sort tab has two rows, then the query line.
	m, _ = press(t, m, nextTab, keyJ, keyJ, keyJ)
	if m.row != sortRows {
		t.Errorf("sort tab: row = %d, want the query line, %d", m.row, sortRows)
	}
	if m.Capturing() {
		t.Error("the query line captures keys outside insert mode")
	}
	m, _ = press(t, m, keyG)
	if m.row != sortByRow {
		t.Errorf("sort tab: g went to row %d, want %d", m.row, sortByRow)
	}
}

// h and l change a choice, a toggle, what is sorted by and the order, and
// do nothing on the other rows.
func TestPrevNextChange(t *testing.T) {
	tests := []struct {
		name string
		tab  Tab
		row  int
		keys []tea.Msg
		want string
	}{
		{name: "choice", row: rowState, keys: []tea.Msg{keyL}, want: strings.Replace(prDefaults, "is:open", "is:closed", 1)},
		{name: "choice back", row: rowState, keys: []tea.Msg{keyL, keyH}, want: prDefaults},
		{name: "choice wraps", row: rowState, keys: []tea.Msg{keyH}, want: strings.Replace(prDefaults, "is:open ", "", 1)},
		{name: "toggle", row: rowDrafts, keys: []tea.Msg{keyL}, want: strings.Replace(prDefaults, " base:main", " -is:draft base:main", 1)},
		{name: "toggle flips back", row: rowDrafts, keys: []tea.Msg{keyH, keyL}, want: prDefaults},
		{name: "sort by", tab: SortTab, row: sortByRow, keys: []tea.Msg{keyL}, want: strings.Replace(prDefaults, "updated", "created", 1)},
		{name: "order", tab: SortTab, row: sortOrderRow, keys: []tea.Msg{keyH}, want: strings.Replace(prDefaults, "desc", "asc", 1)},
		{name: "person", row: rowAuthor, keys: []tea.Msg{keyL, keyH, left, right}, want: prDefaults},
		{name: "multi", row: rowLabels, keys: []tea.Msg{keyL, keyH, left, right}, want: prDefaults},
		{name: "text", row: rowBase, keys: []tea.Msg{keyL, keyH, left, right}, want: prDefaults},
		{name: "query", row: rowQuery, keys: []tea.Msg{keyL, keyH, left, right}, want: prDefaults},
		{name: "sort query", tab: SortTab, row: sortRows, keys: []tea.Msg{keyL, keyH}, want: prDefaults},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, prSpec(nil), WithTab(tt.tab))
			m, _ = press(t, m, keys(down, tt.row)...)
			m, sent := press(t, m, tt.keys...)
			if len(sent) > 0 || m.mode != rowsMode {
				t.Errorf("sent %v, mode %v; want nothing and the rows", sent, m.mode)
			}
			if got := m.Query(); got != tt.want {
				t.Errorf("Query = %q, want %q", got, tt.want)
			}
		})
	}
	// A choice with no options has nothing to change.
	spec := Spec{Fields: []Field{{Key: "wf", Label: "Workflow", Kind: Choice, Qualifier: "workflow"}}}
	m := open(t, spec)
	m, _ = press(t, m, keyL, keyH, left, right)
	if got := m.Query(); got != "" {
		t.Errorf("Query = %q, want none", got)
	}
}

// i starts typing with the cursor at the start and a at the end, in a text
// field and on the query line, and the help line says INSERT.
func TestInsertAndAppend(t *testing.T) {
	tests := []struct {
		name  string
		row   int
		start tea.Msg
		want  int
	}{
		{name: "i in a field", row: rowBase, start: keyI, want: 0},
		{name: "a in a field", row: rowBase, start: keyA, want: len("main")},
		{name: "i on the query", row: rowQuery, start: keyI, want: 0},
		{name: "a on the query", row: rowQuery, start: keyA, want: len(prDefaults)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, prSpec(nil), WithSize(100, 20))
			m, _ = press(t, m, keys(down, tt.row)...)
			if strings.Contains(ansi.Strip(m.View()), "INSERT") {
				t.Fatal("INSERT shows before insert mode")
			}
			m, _ = press(t, m, tt.start)
			if m.mode != insertMode || !m.Capturing() {
				t.Fatalf("mode = %v, capturing = %v; want insert mode", m.mode, m.Capturing())
			}
			pos := m.text.Position()
			if tt.row == rowQuery {
				pos = m.query.Position()
			}
			if pos != tt.want {
				t.Errorf("cursor at %d, want %d", pos, tt.want)
			}
			m = typeText(t, m, "xy")
			v := ansi.Strip(m.View())
			if !strings.Contains(v, "INSERT") {
				t.Errorf("the view doesn't say INSERT:\n%s", v)
			}
			if tt.row == rowBase {
				want := "xymain"
				if tt.start == keyA {
					want = "mainxy"
				}
				if got, _ := m.Value("base"); got.Text() != want {
					t.Errorf("base = %q, want %q", got.Text(), want)
				}
			}
		})
	}
	// Elsewhere they do nothing.
	for _, row := range []int{rowState, rowAuthor, rowLabels, rowDrafts} {
		m := open(t, prSpec(nil))
		m, _ = press(t, m, keys(down, row)...)
		m, _ = press(t, m, keyI, keyA)
		if m.mode != rowsMode || m.Query() != prDefaults {
			t.Errorf("row %d: mode %v, query %q; want the rows and the defaults", row, m.mode, m.Query())
		}
	}
}

// Esc leaves insert mode and keeps the text, trimmed in a field, in a
// field and on the query line.
func TestEscLeavesInsertKeepingText(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, keys(down, rowBase)...)
	m, _ = press(t, m, keyA)
	m = typeText(t, m, "-x  ")
	m, sent := press(t, m, esc)
	if len(sent) > 0 || m.mode != rowsMode || m.Capturing() {
		t.Fatalf("sent %v, mode %v; want the rows and nothing sent", sent, m.mode)
	}
	if v, _ := m.Value("base"); v.Text() != "main-x" {
		t.Errorf("base = %q, want the text kept and trimmed", v.Text())
	}
	m, _ = press(t, m, keyBigG, keyA)
	m = typeText(t, m, " fix")
	m, sent = press(t, m, esc)
	if len(sent) > 0 || m.mode != rowsMode {
		t.Fatalf("sent %v, mode %v; want the rows and nothing sent", sent, m.mode)
	}
	if got, want := m.Query(), strings.Replace(prDefaults, "base:main", "base:main-x", 1)+" fix"; got != want {
		t.Errorf("Query = %q, want %q", got, want)
	}
	if got := m.query.Value(); got != m.Query() {
		t.Errorf("query line = %q, want it to follow the fields", got)
	}
}

// Enter in insert mode applies, from a field and from the query line, with
// the text kept.
func TestEnterInInsertApplies(t *testing.T) {
	for _, row := range []int{rowBase, rowQuery} {
		m := open(t, prSpec(nil))
		m, _ = press(t, m, keys(down, row)...)
		m, _ = press(t, m, keyA)
		m = typeText(t, m, " fix")
		m, sent := press(t, m, enter)
		if len(sent) != 1 || m.mode != rowsMode {
			t.Fatalf("row %d: sent %v, mode %v; want one AppliedMsg", row, sent, m.mode)
		}
		a, ok := sent[0].(AppliedMsg)
		if !ok || a.ID != m.ID() {
			t.Fatalf("row %d: sent %+v, want this form's AppliedMsg", row, sent[0])
		}
		want := prDefaults + " fix"
		if row == rowBase {
			want = strings.Replace(prDefaults, "base:main", `base:"main fix"`, 1)
		}
		if a.Query != want {
			t.Errorf("row %d: applied %q, want %q", row, a.Query, want)
		}
	}
}

// Outside insert mode a letter moves or acts and never types, on the query
// row too.
func TestLettersDontTypeOnTheQueryRow(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, keyBigG)
	before := m.query.Value()
	m, sent := press(t, m, keyJ, keyX, keyR, keyF)
	if len(sent) > 0 || m.row != rowQuery || m.query.Value() != before || m.Query() != prDefaults {
		t.Errorf("sent %v, row %d, query line %q; want no move, no typing", sent, m.row, m.query.Value())
	}
	m, _ = press(t, m, keyK)
	if m.row != rowBase || m.query.Value() != before {
		t.Errorf("row = %d, query line %q; want k to move up and type nothing", m.row, m.query.Value())
	}
}

// q closes from the rows and from a list, and is typed in insert mode and
// in a list's filter.
func TestQuitCloses(t *testing.T) {
	m := open(t, prSpec(nil))
	_, sent := press(t, m, keyQ)
	if len(sent) != 1 || sent[0] != (CancelMsg{ID: m.ID()}) {
		t.Fatalf("sent %v, want a CancelMsg", sent)
	}
	m, _ = press(t, m, keys(down, rowBase)...)
	m, sent = press(t, m, keyA, keyQ)
	if len(sent) > 0 {
		t.Fatalf("sent %v, want q typed in insert mode", sent)
	}
	if v, _ := m.Value("base"); v.Text() != "mainq" {
		t.Errorf("base = %q, want q typed", v.Text())
	}
	p := open(t, prSpec(nil))
	p, _ = press(t, p, down, space)
	if _, sent = press(t, p, keyQ); len(sent) != 1 || sent[0] != (CancelMsg{ID: p.ID()}) {
		t.Errorf("sent %v, want a CancelMsg from a list", sent)
	}
	p, sent = press(t, p, keyI, keyQ)
	if len(sent) > 0 || p.pick.Query().Text != "q" {
		t.Errorf("sent %v, picker query %q; want q typed in the filter", sent, p.pick.Query().Text)
	}
}

func TestCapturing(t *testing.T) {
	m := New(prSpec(nil))
	m, _ = press(t, m, down)
	if m.Capturing() || m.row != rowState {
		t.Error("a blurred form took a key")
	}
	m.Focus()
	if m.Capturing() {
		t.Error("a form on a choice row captures keys")
	}
	m, _ = press(t, m, keyBigG)
	if m.Capturing() {
		t.Error("the query line captures keys outside insert mode")
	}
	m, _ = press(t, m, keyI)
	if !m.Capturing() {
		t.Error("insert mode doesn't capture keys")
	}
	m.Blur()
	if m.Capturing() || m.mode != rowsMode {
		t.Error("a blurred form captures keys")
	}
	m.Focus()
	m, _ = press(t, m, keyG, down, space)
	if m.Capturing() {
		t.Error("a list in its normal mode captures keys")
	}
	m, _ = press(t, m, keyI)
	if !m.Capturing() || m.CapturedBy() != CapturePicker {
		t.Error("a list's filter doesn't capture keys")
	}
}

// A paste is typed in insert mode and ignored in the rows.
func TestPasteOnlyInInsert(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, keys(down, rowBase)...)
	m, _ = press(t, m, tea.PasteMsg{Content: "zzz"})
	if v, _ := m.Value("base"); v.Text() != "main" {
		t.Errorf("base = %q, want the paste ignored in the rows", v.Text())
	}
	m, _ = press(t, m, keyA, tea.PasteMsg{Content: "zzz"})
	if v, _ := m.Value("base"); v.Text() != "mainzzz" {
		t.Errorf("base = %q, want the paste typed", v.Text())
	}
}

func TestSetQueryClosesEditor(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, down, down, down, space)
	m.SetQuery("is:closed")
	if m.mode != rowsMode || m.picking {
		t.Error("SetQuery left the list open")
	}
	if got := m.query.Value(); got != "is:closed sort:updated-desc" {
		t.Errorf("query line = %q", got)
	}
	m, _ = press(t, m, keyBigG, keyA)
	m.SetQuery("is:open")
	if m.mode != rowsMode || m.Capturing() {
		t.Error("SetQuery left insert mode")
	}
	m, _ = press(t, m, keyI)
	m.Reset()
	if m.mode != rowsMode {
		t.Error("Reset left insert mode")
	}
	m, _ = press(t, m, keyI)
	m.SetTab(SortTab)
	if m.mode != rowsMode {
		t.Error("SetTab left insert mode")
	}
}

// Keys pressed on a copy don't change the original.
func TestCopiesAreIndependent(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, down, down, down)
	c, _ := press(t, m, del)
	if m.Query() != prDefaults {
		t.Errorf("the original changed: query %q", m.Query())
	}
	if c.Query() == prDefaults {
		t.Error("the copy didn't change")
	}
}
