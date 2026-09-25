package filterform

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
			name: "space picks the next choice", keys: step{space, space},
			wantQuery: strings.Replace(prDefaults, "is:open", "is:merged", 1),
		},
		{
			name: "whole-token choice", keys: step{down, down, right},
			wantQuery: strings.Replace(prDefaults, "review-requested:@me", "review:approved", 1),
			wantRow:   rowReview,
		},
		{
			name: "space flips a toggle", keys: step{tab, tab, tab, tab, space},
			wantQuery: strings.Replace(prDefaults, " base:main", " -is:draft base:main", 1),
			wantRow:   rowDrafts,
		},
		{
			name: "left flips a toggle too", keys: step{shiftTab, shiftTab, shiftTab, shiftTab, left, left},
			wantQuery: prDefaults, wantRow: rowDrafts,
		},
		{
			name: "x turns a toggle off", query: "-is:draft", keys: step{up, up, up, up, keyX},
			wantQuery: "sort:updated-desc", wantRow: rowDrafts,
		},
		{
			name: "right sorts by the next option", keys: append(keys(down, rowSort), right),
			wantQuery: strings.Replace(prDefaults, "sort:updated-desc", "sort:created-desc", 1),
			wantRow:   rowSort,
		},
		{
			name: "space flips the sort", keys: append(keys(down, rowSort), space),
			wantQuery: strings.Replace(prDefaults, "desc", "asc", 1), wantRow: rowSort,
		},
		{
			name: "x removes the last chip", keys: step{down, down, down, keyX},
			wantQuery: strings.Replace(prDefaults, "label:bug,enhancement", "label:bug", 1),
			wantRow:   rowLabels,
		},
		{
			name: "left then backspace removes the chip under the cursor", keys: step{down, down, down, left, left, bksp},
			wantQuery: strings.Replace(prDefaults, "label:bug,enhancement", "label:enhancement", 1),
			wantRow:   rowLabels,
		},
		{
			name: "removing every chip drops the qualifier", keys: step{down, down, down, keyX, keyX, keyX},
			wantQuery: strings.Replace(prDefaults, "label:bug,enhancement ", "", 1),
			wantRow:   rowLabels,
		},
		{
			name: "x clears a person", keys: step{down, keyX},
			wantQuery: strings.Replace(prDefaults, "author:@me ", "", 1), wantRow: rowAuthor,
		},
		{
			name: "enter edits a text", keys: append(keys(down, rowBase), enter, bksp, bksp, bksp, bksp),
			typed: "release 1.0", after: step{enter},
			wantQuery: strings.Replace(prDefaults, "base:main", `base:"release 1.0"`, 1), wantRow: rowBase,
		},
		{
			name: "esc undoes a text", keys: append(keys(down, rowBase), enter),
			typed: "-x", after: step{esc},
			wantQuery: prDefaults, wantRow: rowBase,
		},
		{
			name: "a person takes the highlighted login", keys: step{down, keyX, enter},
			typed: "octo", after: step{enter},
			wantQuery: strings.Replace(prDefaults, "@me", "octocat", 1), wantRow: rowAuthor,
		},
		{
			name: "a person takes a typed login nothing matches", keys: step{down, enter},
			typed: "hubot", after: step{enter},
			wantQuery: strings.Replace(prDefaults, "author:@me", "author:hubot", 1), wantRow: rowAuthor,
		},
		{
			name: "esc undoes a person", keys: step{down, enter},
			typed: "hubot", after: step{space, esc},
			wantQuery: prDefaults, wantRow: rowAuthor,
		},
		{
			name: "r resets", query: "is:closed fix", keys: step{keyR},
			wantQuery: prDefaults,
		},
		{
			name: "up wraps to the query line, where letters are typed", keys: step{up},
			typed: " fix x r", wantQuery: prDefaults + " fix x r", wantRow: rowQuery,
		},
		{
			name: "the query line sets the fields as it is typed", query: "is:open", keys: step{up, ctrlU},
			typed: "is:closed label:docs", after: step{down},
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
			if m.row != tt.wantRow {
				t.Errorf("row = %d, want %d", m.row, tt.wantRow)
			}
			if m.editing {
				t.Error("an editor is still open")
			}
		})
	}
}

// Typing on the query line leaves what was typed alone, and it is
// normalized once the line loses focus.
func TestQueryLineKeepsTyping(t *testing.T) {
	m := open(t, prSpec(nil), WithQuery(""))
	m, _ = press(t, m, up, ctrlU)
	m = typeText(t, m, "label:  is:closed  ")
	if got := m.query.Value(); got != "label:  is:closed  " {
		t.Errorf("query line = %q, want what was typed", got)
	}
	if v, _ := m.Value("state"); v.Text() != "closed" {
		t.Errorf("state = %q, want closed", v.Text())
	}
	m, _ = press(t, m, up)
	if got, want := m.query.Value(), "is:closed sort:updated-desc label:"; got != want {
		t.Errorf("query line = %q, want %q", got, want)
	}
}

func TestMultiEditor(t *testing.T) {
	tests := []struct {
		name  string
		typed string
		keys  []tea.Msg
		want  []string
	}{
		{name: "enter adds the highlighted item", typed: "docs", keys: []tea.Msg{enter}, want: []string{"bug", "enhancement", "docs"}},
		{name: "enter doesn't remove a chosen item", typed: "bug", keys: []tea.Msg{enter}, want: []string{"bug", "enhancement"}},
		{name: "space removes a chosen item", typed: "bug", keys: []tea.Msg{space, enter}, want: []string{"enhancement"}},
		{
			name: "space chooses several and enter only closes",
			keys: []tea.Msg{down, down, space, down, space, up, enter},
			want: []string{"bug", "enhancement", "docs", "good first issue"},
		},
		{name: "esc undoes the editor", keys: []tea.Msg{down, down, space, esc}, want: []string{"bug", "enhancement"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLoader{}
			m := open(t, prSpec(f.load))
			m, _ = press(t, m, down, down, down, enter)
			if !m.picking || !m.Capturing() {
				t.Fatalf("picking = %v, Capturing = %v after enter; want an open picker", m.picking, m.Capturing())
			}
			m = typeText(t, m, tt.typed)
			m, sent := press(t, m, tt.keys...)
			if len(sent) > 0 {
				t.Errorf("sent %+v, want nothing", sent)
			}
			if v, _ := m.Value("labels"); !slices.Equal(v.List(), tt.want) {
				t.Errorf("labels = %q, want %q", v.List(), tt.want)
			}
			if m.editing || m.Capturing() {
				t.Error("the editor is still open")
			}
		})
	}
}

// Space marks the item it chose, and the highlight stays on it.
func TestMultiEditorMarks(t *testing.T) {
	f := &fakeLoader{}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down, enter, down, down, space)
	it, ok := m.pick.Selected()
	if !ok || it.Value != "docs" || !strings.HasPrefix(it.Title, chosenMark) {
		t.Errorf("selected %+v, want docs marked as chosen", it)
	}
	if got := m.query.Value(); !strings.Contains(got, "label:bug,enhancement,docs") {
		t.Errorf("query line = %q, want it to follow the picker", got)
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
	m, cmd = m.Update(enter)
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
	m, _ = press(t, m, enter)
	if !m.picking || m.pick.Len() != len(labels) {
		t.Fatalf("picking = %v with %d items after retrying; want the labels", m.picking, m.pick.Len())
	}
	m, _ = press(t, m, esc, enter)
	if f.calls() != 2 {
		t.Errorf("the loader was called %d times, want 2: once failing, once again", f.calls())
	}
	if !m.picking {
		t.Error("opening the field again didn't show the loaded labels")
	}
}

func TestLoadIgnoresOtherForms(t *testing.T) {
	f := &fakeLoader{}
	a, b := open(t, prSpec(f.load)), open(t, prSpec(f.load))
	a, _ = press(t, a, down, down, down)
	a, cmd := a.Update(enter)
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
	m, cmd := m.Update(enter)
	ctx := m.ctx
	m.Blur()
	if ctx.Err() == nil {
		t.Error("Blur didn't cancel the load")
	}
	close(f.block)
	m, _ = run(t, m, cmd)
	if m.fields[rowLabels].state != notLoaded || m.editing {
		t.Errorf("state = %v, editing = %v; want the load dropped", m.fields[rowLabels].state, m.editing)
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
	m, _ = press(t, m, enter)
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
		{name: "enter on the query line applies", keys: []tea.Msg{right, up, enter}},
		{name: "enter on a toggle applies", keys: []tea.Msg{right, down, down, down, down, enter}},
		{name: "esc cancels", keys: []tea.Msg{right, esc}, want: CancelMsg{}},
		{name: "esc on the query line cancels", keys: []tea.Msg{right, up, esc}, want: CancelMsg{}},
		{name: "esc closes the editor first", keys: []tea.Msg{right, down, down, down, enter, esc, esc}, want: CancelMsg{}},
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

// Enter on a Multi, Person or Text row opens its editor rather than
// applying.
func TestEnterOpensEditors(t *testing.T) {
	for _, row := range []int{rowAuthor, rowLabels, rowBase} {
		m := open(t, prSpec(nil))
		m, _ = press(t, m, keys(down, row)...)
		m, sent := press(t, m, enter)
		if len(sent) > 0 || !m.editing || !m.Capturing() {
			t.Errorf("row %d: sent %v, editing %v; want an open editor", row, sent, m.editing)
		}
	}
}

func TestCapturing(t *testing.T) {
	m := New(prSpec(nil))
	m, _ = press(t, m, up)
	if m.Capturing() || m.row != rowState {
		t.Error("a blurred form took a key")
	}
	m.Focus()
	if m.Capturing() {
		t.Error("a form on a choice row captures keys")
	}
	m, _ = press(t, m, up)
	if !m.Capturing() {
		t.Error("the query line doesn't capture keys")
	}
	m.Blur()
	if m.Capturing() {
		t.Error("a blurred form captures keys")
	}
}

func TestSetQueryClosesEditor(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, down, down, down, enter)
	m.SetQuery("is:closed")
	if m.editing || m.picking {
		t.Error("SetQuery left the editor open")
	}
	if got := m.query.Value(); got != "is:closed sort:updated-desc" {
		t.Errorf("query line = %q", got)
	}
}

// Keys pressed on a copy don't change the original.
func TestCopiesAreIndependent(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, down, down, down)
	c, _ := press(t, m, left, keyX)
	if m.fields[rowLabels].chip != 2 || m.Query() != prDefaults {
		t.Errorf("the original changed: chip %d, query %q", m.fields[rowLabels].chip, m.Query())
	}
	if c.Query() == prDefaults {
		t.Error("the copy didn't change")
	}
}
