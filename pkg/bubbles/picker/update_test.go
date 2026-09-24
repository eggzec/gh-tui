package picker

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestInitSearchesTheEmptyQuery(t *testing.T) {
	f := &fakeSearch{}
	m := New(f.search, WithSize(60, 12))
	if !m.Loading() {
		t.Error("a new picker isn't loading its first results")
	}
	m, _ = run(t, m, m.Init())
	if got := titlesOf(m); !slices.Equal(got, []string{ghTUI.Title, dotfiles.Title}) {
		t.Errorf("results = %q, want the repositories", got)
	}
	if m.Loading() || m.Len() != 2 {
		t.Errorf("Loading = %v, Len = %d; want done with 2", m.Loading(), m.Len())
	}
	if got := f.Queries(); !slices.Equal(got, []Query{{}}) {
		t.Errorf("queries = %+v, want one empty query", got)
	}
}

func TestUpdate(t *testing.T) {
	scopes := WithScopes(kindRepos, kindIssues, kindPulls)
	tests := []struct {
		name  string
		opts  []Option
		typed string
		keys  []tea.Msg
		// want is the message the keys send the parent, or nil for none.
		want      tea.Msg
		wantQuery Query
	}{
		{
			name: "enter chooses the first result", typed: "crash",
			keys: []tea.Msg{enter}, want: ChosenMsg{Item: crash},
			wantQuery: Query{Text: "crash"},
		},
		{
			name: "down skips the group header", typed: "crash",
			keys: []tea.Msg{down, enter}, want: ChosenMsg{Item: fix},
			wantQuery: Query{Text: "crash"},
		},
		{
			name: "ctrl+n and ctrl+p move", typed: "crash",
			keys: []tea.Msg{ctrlN, ctrlP, ctrlN, enter}, want: ChosenMsg{Item: fix},
			wantQuery: Query{Text: "crash"},
		},
		{
			name: "moves stop at the ends", typed: "crash",
			keys: []tea.Msg{down, down, down, up, up, up, enter}, want: ChosenMsg{Item: crash},
			wantQuery: Query{Text: "crash"},
		},
		{
			name: "pages", typed: "",
			keys: []tea.Msg{pgDown, enter}, want: ChosenMsg{Item: dotfiles},
			wantQuery: Query{},
		},
		{
			name: "pages back", typed: "",
			keys: []tea.Msg{pgDown, pgUp, enter}, want: ChosenMsg{Item: ghTUI},
			wantQuery: Query{},
		},
		{
			name: "esc cancels", typed: "crash",
			keys: []tea.Msg{esc}, want: CancelMsg{},
			wantQuery: Query{Text: "crash"},
		},
		{
			name: "enter without results sends nothing", typed: "zzz",
			keys:      []tea.Msg{enter},
			wantQuery: Query{Text: "zzz"},
		},
		{
			name: "j, k and q are text", typed: "jkq",
			wantQuery: Query{Text: "jkq"},
		},
		{
			name: "backspace searches again", typed: "crashx",
			keys: []tea.Msg{bksp, enter}, want: ChosenMsg{Item: crash},
			wantQuery: Query{Text: "crash"},
		},
		{
			name: "paste searches", keys: []tea.Msg{tea.PasteMsg{Content: "empty config"}, enter},
			want: ChosenMsg{Item: fix}, wantQuery: Query{Text: "empty config"},
		},
		{
			name: "tab scopes the search", opts: []Option{scopes}, typed: "crash",
			keys: []tea.Msg{tab, tab, enter}, want: ChosenMsg{Item: crash},
			wantQuery: Query{Text: "crash", Scope: kindIssues},
		},
		{
			name: "shift+tab wraps to the last scope", opts: []Option{scopes}, typed: "crash",
			keys: []tea.Msg{shiftTab, enter}, want: ChosenMsg{Item: fix},
			wantQuery: Query{Text: "crash", Scope: kindPulls},
		},
		{
			name: "tab wraps back to all", opts: []Option{scopes}, typed: "crash",
			keys: []tea.Msg{tab, tab, tab, tab, enter}, want: ChosenMsg{Item: crash},
			wantQuery: Query{Text: "crash"},
		},
		{
			name: "tab without scopes is ignored", typed: "crash",
			keys: []tea.Msg{tab, enter}, want: ChosenMsg{Item: crash},
			wantQuery: Query{Text: "crash"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSearch{}
			m := open(t, f.search, tt.opts...)
			m = typeText(t, m, tt.typed)
			m, sent := press(t, m, tt.keys...)

			var want []tea.Msg
			switch w := tt.want.(type) {
			case ChosenMsg:
				w.ID = m.ID()
				want = append(want, w)
			case CancelMsg:
				w.ID = m.ID()
				want = append(want, w)
			}
			if len(sent) != len(want) || (len(want) == 1 && !sameMsg(sent[0], want[0])) {
				t.Errorf("sent %+v, want %+v", sent, want)
			}
			if q := m.Query(); q != tt.wantQuery {
				t.Errorf("Query() = %+v, want %+v", q, tt.wantQuery)
			}
			if qs := f.Queries(); qs[len(qs)-1] != tt.wantQuery {
				t.Errorf("last search = %+v, want %+v", qs[len(qs)-1], tt.wantQuery)
			}
		})
	}
}

func sameMsg(a, b tea.Msg) bool {
	switch a := a.(type) {
	case ChosenMsg:
		b, ok := b.(ChosenMsg)
		return ok && a.ID == b.ID && a.Item == b.Item
	case CancelMsg:
		return a == b
	}
	return false
}

func TestDebounce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeSearch{}
		m := New(f.search, WithDebounce(250*time.Millisecond), WithSize(60, 12))
		m.Focus()
		m, _ = run(t, m, m.Init())

		m, first := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
		m, second := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
		if !m.Loading() {
			t.Error("the picker isn't loading while it waits")
		}
		start := time.Now()
		// The first key's tick comes after the second key, so it is stale.
		m, _ = run(t, m, first)
		if got := f.Queries(); len(got) != 1 {
			t.Fatalf("queries = %+v, want only the first empty one", got)
		}
		m, _ = run(t, m, second)
		if waited := time.Since(start); waited < 250*time.Millisecond {
			t.Errorf("searched after %v, want the debounce", waited)
		}
		if got := f.Queries(); !slices.Equal(got, []Query{{}, {Text: "cr"}}) {
			t.Errorf("queries = %+v, want one search for cr", got)
		}
		if m.Loading() || m.Len() != 2 {
			t.Errorf("Loading = %v, Len = %d; want the results", m.Loading(), m.Len())
		}
	})
}

func TestNewQueryCancelsTheLastSearch(t *testing.T) {
	f := &fakeSearch{}
	m := open(t, f.search)
	m, first := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	m, second := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	// The first search runs late, after the second key.
	m, _ = run(t, m, first)
	m, _ = run(t, m, second)
	f.mu.Lock()
	cancelled := slices.Clone(f.cancelled)
	f.mu.Unlock()
	if !slices.Equal(cancelled, []bool{false, true, false}) {
		t.Errorf("cancelled = %v, want only the search for c", cancelled)
	}
	if q := m.Query(); q.Text != "cr" || m.Len() != 2 {
		t.Errorf("query %q with %d results, want the results for cr", q.Text, m.Len())
	}
}

func TestStaleAndForeignResultsAreDropped(t *testing.T) {
	f := &fakeSearch{}
	m := open(t, f.search)
	m = typeText(t, m, "crash")
	for _, msg := range []tea.Msg{
		resultMsg{id: m.ID(), seq: m.seq - 1, items: []Item{dotfiles}},
		resultMsg{id: m.ID() + 1, seq: m.seq, items: []Item{dotfiles}},
		debounceMsg{id: m.ID() + 1, seq: m.seq},
	} {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd != nil {
			t.Errorf("Update(%T) returned a command", msg)
		}
	}
	if got := titlesOf(m); !slices.Equal(got, []string{crash.Title, fix.Title}) {
		t.Errorf("results = %q, want those for crash", got)
	}
}

func TestSearchError(t *testing.T) {
	f := &fakeSearch{fail: errBoom}
	m := open(t, f.search)
	m = typeText(t, m, "crash")
	if !errors.Is(m.Err(), errBoom) || m.Len() != 0 || m.Loading() {
		t.Fatalf("Err = %v, Len = %d, Loading = %v; want the error alone", m.Err(), m.Len(), m.Loading())
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Couldn't search: github: 502 Bad Gateway") {
		t.Errorf("the error isn't shown:\n%s", v)
	}

	f.mu.Lock()
	f.fail = nil
	f.mu.Unlock()
	m, _ = press(t, m, bksp)
	if m.Err() != nil || m.Len() != 2 {
		t.Errorf("Err = %v, Len = %d after another search; want results", m.Err(), m.Len())
	}
}

func TestLocalFilter(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithScopes(kindRepos, kindIssues, kindPulls))
	if cmd := m.Init(); cmd != nil {
		t.Error("a picker without Search has something to start")
	}
	if got := titlesOf(m); !slices.Equal(got, []string{ghTUI.Title, dotfiles.Title, crash.Title, fix.Title}) {
		t.Errorf("empty query lists %q, want every item", got)
	}

	m = typeText(t, m, "ghtui")
	if got := titlesOf(m); !slices.Equal(got, []string{ghTUI.Title}) {
		t.Errorf("ghtui matches %q, want gh-tui", got)
	}
	if got := m.results[0].matches; !slices.Equal(got, []int{7, 8, 10, 11, 12}) {
		t.Errorf("matches = %v, want the offsets of g h t u i", got)
	}

	m, _ = press(t, m, bksp, bksp, bksp, bksp, bksp)
	m = typeText(t, m, "cfg")
	// The best match comes first, and brings its group along.
	if got := titlesOf(m); !slices.Equal(got, []string{fix.Title, crash.Title}) {
		t.Errorf("cfg matches %q, want the pull request, then the issue", got)
	}
	m, _ = press(t, m, shiftTab)
	if got := titlesOf(m); !slices.Equal(got, []string{fix.Title}) {
		t.Errorf("cfg in pull requests matches %q, want the pull request", got)
	}

	m.SetItems([]Item{{Kind: kindPulls, Title: "Config loader"}})
	if got := titlesOf(m); !slices.Equal(got, []string{"Config loader"}) {
		t.Errorf("after SetItems: %q, want the new item", got)
	}
}

func TestItemsWithSearch(t *testing.T) {
	f := &fakeSearch{}
	recent := Item{Kind: kindRepos, Title: "recent/repo"}
	m := open(t, f.search, WithItems([]Item{recent}))
	if got := titlesOf(m); !slices.Equal(got, []string{recent.Title}) || len(f.Queries()) != 0 {
		t.Fatalf("empty query lists %q after %d searches, want the items and none", got, len(f.Queries()))
	}
	m = typeText(t, m, "fix")
	if got := titlesOf(m); !slices.Equal(got, []string{fix.Title}) {
		t.Errorf("fix lists %q, want the search's result", got)
	}
	m, _ = press(t, m, bksp, bksp, bksp)
	if got := titlesOf(m); !slices.Equal(got, []string{recent.Title}) || m.Loading() {
		t.Errorf("cleared query lists %q, loading %v; want the items", got, m.Loading())
	}
	// f, fi and fix, then fi and f again; the empty query lists the items.
	if n := len(f.Queries()); n != 5 {
		t.Errorf("made %d searches, want 5", n)
	}
}

func TestChooseAndCancelDropTheSearchInFlight(t *testing.T) {
	f := &fakeSearch{}
	m := open(t, f.search)
	m, pending := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m, sent := press(t, m, esc)
	if len(sent) != 1 || m.Loading() {
		t.Fatalf("esc sent %v, loading %v; want a cancel and no search", sent, m.Loading())
	}
	before := titlesOf(m)
	m, _ = run(t, m, pending)
	if got := titlesOf(m); !slices.Equal(got, before) {
		t.Errorf("a search landed after esc: %q, want %q", got, before)
	}

	cmd := m.Reset()
	if m.Query() != (Query{}) || !m.Loading() {
		t.Errorf("Reset left query %+v, loading %v", m.Query(), m.Loading())
	}
	m, _ = run(t, m, cmd)
	if m.Len() != 2 {
		t.Errorf("Reset lists %d results, want the repositories", m.Len())
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	f := &fakeSearch{}
	m := New(f.search, WithDebounce(0))
	if m.Focused() {
		t.Fatal("a new picker is focused")
	}
	m, _ = run(t, m, m.Init())
	m = typeText(t, m, "crash")
	if m.Query().Text != "" {
		t.Errorf("a blurred picker took keys: %q", m.Query().Text)
	}
	if _, sent := press(t, m, enter, tea.PasteMsg{Content: "x"}); len(sent) != 0 {
		t.Errorf("a blurred picker sent %v", sent)
	}
	m.Focus()
	m = typeText(t, m, "a")
	m.Blur()
	m = typeText(t, m, "b")
	if m.Query().Text != "a" {
		t.Errorf("Query() = %q after blur, want a", m.Query().Text)
	}
}

func TestMessagesAreScoped(t *testing.T) {
	f := &fakeSearch{}
	a, b := open(t, f.search), open(t, f.search)
	if a.ID() == b.ID() {
		t.Fatal("two pickers share an ID")
	}
	_, cmd := a.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	before := titlesOf(b)
	b, _ = run(t, b, cmd)
	if got := titlesOf(b); !slices.Equal(got, before) || b.Query().Text != "" {
		t.Errorf("b took a's search: %q", got)
	}
	_, sent := press(t, a, esc)
	if c, ok := sent[0].(CancelMsg); !ok || c.ID != a.ID() {
		t.Errorf("cancel = %+v, want the ID of its picker", sent[0])
	}
}

func TestScrollKeepsTheSelectionInView(t *testing.T) {
	items := make([]Item, 0, 30)
	for i := range 30 {
		kind := kindRepos
		if i >= 15 {
			kind = kindIssues
		}
		items = append(items, Item{Kind: kind, Title: "item " + string(rune('a'+i%26)) + string(rune('0'+i/26))})
	}
	// 10 rows tall: the frame takes 2, the input and status 2, so 6 rows
	// of results.
	m := open(t, nil, WithItems(items), WithSize(40, 10))
	for i := range 30 {
		m.sel = i
		m.scroll()
		r := m.itemRow[i]
		if r < m.top || r >= m.top+6 {
			t.Fatalf("item %d at row %d is out of view from %d", i, r, m.top)
		}
	}
	// The first issue brings its header along when moving up to it.
	m, _ = press(t, m, up, up, up, up, up, up, up, up, up, up, up, up, up, up)
	if m.sel != 15 || m.rows[m.top].header != kindIssues {
		t.Errorf("sel %d, top row %+v; want the first issue under its header", m.sel, m.rows[m.top])
	}
	m.SetSize(40, 3)
	if m.listHeight() != 0 || m.top != 0 {
		t.Errorf("no room for results: list height %d, top %d", m.listHeight(), m.top)
	}
}

func TestAccessors(t *testing.T) {
	m := New(nil, WithSize(50, 9), WithItems(catalog))
	if !m.Capturing() || m.Width() != 50 || m.Height() != 9 || m.Err() != nil {
		t.Errorf("Capturing %v, size %d×%d, Err %v", m.Capturing(), m.Width(), m.Height(), m.Err())
	}
	if it, ok := m.Selected(); !ok || it.Title != ghTUI.Title {
		t.Errorf("Selected() = %+v, %v; want the first item", it, ok)
	}
	if m.KeyMap().NextScope.Enabled() || m.KeyMap().PrevScope.Enabled() {
		t.Error("scope keys are enabled without scopes")
	}
	if len(m.ShortHelp()) != 5 || len(m.FullHelp()) != 2 {
		t.Error("help should list the keys")
	}
	k := DefaultKeyMap()
	k.Cancel.SetKeys("ctrl+g")
	m.SetKeyMap(k)
	if m.KeyMap().Cancel.Keys()[0] != "ctrl+g" || m.KeyMap().NextScope.Enabled() {
		t.Error("SetKeyMap didn't take, or enabled the scope keys")
	}
	withScopes := New(nil, WithScopes(kindRepos))
	if !withScopes.KeyMap().NextScope.Enabled() {
		t.Error("scope keys are disabled with scopes")
	}
	s := DefaultStyles(false)
	m.SetStyles(s)
	if m.Styles().Title.GetForeground() != s.Title.GetForeground() {
		t.Error("SetStyles didn't take")
	}
	m.SetSize(-1, -1)
	if m.Width() != 0 || m.Height() != 0 || m.View() != "" {
		t.Errorf("negative size: %d×%d %q", m.Width(), m.Height(), m.View())
	}
	empty := New(nil)
	if _, ok := empty.Selected(); ok || empty.Len() != 0 {
		t.Error("a picker without items has a selection")
	}
}
