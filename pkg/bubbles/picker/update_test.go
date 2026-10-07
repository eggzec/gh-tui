package picker

import (
	"context"
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
	m := New(f.search, WithKeyMap(testKeys(t)), WithSize(60, 12))
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
		m := New(f.search, WithKeyMap(testKeys(t)), WithDebounce(250*time.Millisecond), WithSize(60, 12))
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

func TestErrorText(t *testing.T) {
	offline := func(error) (string, string) { return "Can't reach GitHub", "r to retry" }
	tests := []struct {
		name string
		opts []Option
		want string
	}{
		{"default", nil, "✗ Couldn't search: github: 502 Bad Gateway"},
		{"custom", []Option{WithErrorText(offline)}, "✗ Can't reach GitHub · r to retry"},
		{"custom keeps the hint whole", []Option{WithSize(28, 12), WithErrorText(offline)}, "✗ Can't re… · r to retry"},
		{"empty", []Option{WithErrorText(func(error) (string, string) { return "", "" })}, ""},
		{"text with a dot", []Option{WithErrorText(func(error) (string, string) { return "GitHub says a · b", "" })}, "✗ GitHub says a · b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSearch{fail: errBoom}
			m := typeText(t, open(t, f.search, tt.opts...), "crash")
			v := ansi.Strip(m.View())
			if tt.want == "" {
				if strings.Contains(v, "✗") {
					t.Errorf("View() = %q, want no error", v)
				}
				return
			}
			if !strings.Contains(v, tt.want+" ") && !strings.Contains(v, tt.want+"│") {
				t.Errorf("View() = %q, want the error row %q", v, tt.want)
			}
		})
	}
}

// ASCII styles cut the error row with their own ellipsis, with a hint or
// without.
func TestErrorASCII(t *testing.T) {
	st := DefaultStyles(true)
	st.ErrorGlyph, st.ErrorSeparator, st.ErrorEllipsis = "x", " - ", "..."
	offline := WithErrorText(func(error) (string, string) { return "Can't reach GitHub", "r to retry" })
	tests := []struct {
		name string
		opts []Option
		want string
	}{
		{"without a hint", nil, "x Couldn't search: gi..."},
		{"with a hint", []Option{offline}, "x Can't ... - r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSearch{fail: errBoom}
			opts := append([]Option{WithSize(28, 12), WithStyles(st)}, tt.opts...)
			v := ansi.Strip(typeText(t, open(t, f.search, opts...), "crash").View())
			var row string
			for l := range strings.SplitSeq(v, "\n") {
				if strings.Contains(l, "x ") {
					row = l
				}
			}
			if !strings.Contains(row, tt.want) || strings.Contains(row, "…") {
				t.Errorf("error row = %q, want %q cut in ASCII", row, tt.want)
			}
		})
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
	m := New(f.search, WithKeyMap(testKeys(t)), WithDebounce(0))
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
	m := New(nil, WithKeyMap(testKeys(t)), WithSize(50, 9), WithItems(catalog))
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
	k := testKeys(t)
	k.Cancel.SetKeys("ctrl+g")
	m.SetKeyMap(k)
	if m.KeyMap().Cancel.Keys()[0] != "ctrl+g" || m.KeyMap().NextScope.Enabled() {
		t.Error("SetKeyMap didn't take, or enabled the scope keys")
	}
	withScopes := New(nil, WithKeyMap(testKeys(t)), WithScopes(kindRepos))
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
	empty := New(nil, WithKeyMap(testKeys(t)))
	if _, ok := empty.Selected(); ok || empty.Len() != 0 {
		t.Error("a picker without items has a selection")
	}
}

func letter(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func ctrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

// chosen returns the title of the item that enter chooses.
func chosen(t *testing.T, m Model) string {
	t.Helper()
	_, sent := press(t, m, enter)
	if len(sent) != 1 {
		t.Fatalf("enter sent %v, want one ChosenMsg", sent)
	}
	c, ok := sent[0].(ChosenMsg)
	if !ok {
		t.Fatalf("enter sent %#v, want a ChosenMsg", sent[0])
	}
	return c.Item.Title
}

func TestNormalModeMoves(t *testing.T) {
	items := manyItems(30)
	title := func(i int) string { return items[i].Title }
	tests := []struct {
		name string
		keys []tea.Msg
		want int
	}{
		{"j and k", []tea.Msg{letter('j'), letter('j'), letter('j'), letter('k')}, 2},
		{"down and up", []tea.Msg{down, down, up}, 1},
		{"k stops at the top", []tea.Msg{letter('k')}, 0},
		{"G jumps to the last", []tea.Msg{letter('G')}, 29},
		{"g returns to the first", []tea.Msg{letter('G'), letter('g')}, 0},
		{"j stops at the end", []tea.Msg{letter('G'), letter('j')}, 29},
		{"end and home", []tea.Msg{tea.KeyPressMsg{Code: tea.KeyEnd}, tea.KeyPressMsg{Code: tea.KeyHome}}, 0},
		// 12 rows tall: 2 for the frame, 2 above the list, so 8 rows.
		{"ctrl+d goes half a page", []tea.Msg{ctrl('d')}, 4},
		{"ctrl+d and ctrl+u", []tea.Msg{ctrl('d'), ctrl('d'), ctrl('u')}, 4},
		{"ctrl+f goes a page", []tea.Msg{ctrl('f')}, 8},
		{"ctrl+b goes back a page", []tea.Msg{ctrl('f'), ctrl('b')}, 0},
		{"pgdown and pgup", []tea.Msg{pgDown, pgDown, pgUp}, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, nil, WithItems(items), WithModes(true))
			m, sent := press(t, m, tt.keys...)
			if len(sent) != 0 {
				t.Errorf("sent %v", sent)
			}
			if got := chosen(t, m); got != title(tt.want) {
				t.Errorf("selected %q, want %q", got, title(tt.want))
			}
		})
	}
}

// Moves on lists shorter than the height clamp, and the empty list takes
// them without a selection.
func TestNormalModeShortAndEmptyLists(t *testing.T) {
	m := open(t, nil, WithItems(catalog[:2]), WithModes(true), WithSize(40, 12))
	m, _ = press(t, m, letter('G'), ctrl('d'), ctrl('f'), letter('j'))
	if got := chosen(t, m); got != dotfiles.Title {
		t.Errorf("selected %q, want the last", got)
	}
	m, _ = press(t, m, letter('g'), ctrl('u'), ctrl('b'), letter('k'))
	if got := chosen(t, m); got != ghTUI.Title {
		t.Errorf("selected %q, want the first", got)
	}

	empty := open(t, nil, WithItems(nil), WithModes(true), WithEmptyText("Nothing here."))
	empty, sent := press(t, empty, letter('j'), letter('G'), ctrl('d'), letter('g'), enter)
	if len(sent) != 0 || empty.Len() != 0 {
		t.Errorf("empty list: sent %v, len %d", sent, empty.Len())
	}
	if !strings.Contains(ansi.Strip(empty.View()), "Nothing here.") {
		t.Errorf("the empty text is gone:\n%s", empty.View())
	}
}

func TestNormalModeIgnoresLetters(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithModes(true))
	if m.Typing() || m.Capturing() {
		t.Error("a picker with modes types before i")
	}
	m, sent := press(t, m, letter('x'), letter('z'), letter('1'), tab, bksp,
		tea.PasteMsg{Content: "pasted"})
	if len(sent) != 0 || m.Query().Text != "" || m.Len() != len(catalog) {
		t.Errorf("sent %v, query %q, len %d; want the picker untouched", sent, m.Query().Text, m.Len())
	}
}

// Normal mode keeps the scope keys and the arrows' ctrl chords of insert
// mode.
func TestNormalModeKeepsScopeAndChordKeys(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithModes(true), WithScopes(kindRepos, kindIssues))
	m, _ = press(t, m, tab)
	if m.Query().Scope != kindRepos {
		t.Errorf("tab: scope = %q, want %q", m.Query().Scope, kindRepos)
	}
	m, _ = press(t, m, shiftTab, shiftTab)
	if m.Query().Scope != kindIssues {
		t.Errorf("shift+tab twice: scope = %q, want %q", m.Query().Scope, kindIssues)
	}
	m, _ = press(t, m, shiftTab)
	if got := chosen(t, m); got != ghTUI.Title {
		t.Fatalf("selected %q, want %q", got, ghTUI.Title)
	}
	m, _ = press(t, m, ctrlN)
	if got := chosen(t, m); got != dotfiles.Title {
		t.Errorf("ctrl+n selected %q, want %q", got, dotfiles.Title)
	}
	m, _ = press(t, m, ctrlP)
	if got := chosen(t, m); got != ghTUI.Title {
		t.Errorf("ctrl+p selected %q, want %q", got, ghTUI.Title)
	}
	if m.Typing() {
		t.Error("typing")
	}
}

func TestInsertAndAppendFocusTheInput(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithModes(true))
	m.input.SetValue("crash")
	m.input.SetCursor(2)
	for _, tt := range []struct {
		key  rune
		want string
	}{
		{'i', "Xcrash"},
		{'a', "crashX"},
	} {
		n, _ := press(t, m, letter(tt.key))
		if !n.Typing() || !n.Capturing() {
			t.Errorf("%c: not typing", tt.key)
		}
		n, _ = press(t, n, letter('X'))
		if got := n.Query().Text; got != tt.want {
			t.Errorf("%c then X: query %q, want %q", tt.key, got, tt.want)
		}
	}
	// SetQuery isn't an API: a query typed earlier is just as good.
	n := open(t, nil, WithItems(catalog), WithModes(true))
	n = typeText(t, n, "xyz")
	if n.Query().Text != "" {
		t.Fatal("typed in normal mode")
	}
	n, _ = press(t, n, letter('i'))
	n = typeText(t, n, "abc")
	n, _ = press(t, n, esc, letter('i'), letter('X'))
	if got := n.Query().Text; got != "Xabc" {
		t.Errorf("i after typing: %q, want Xabc", got)
	}
	n, _ = press(t, n, esc, letter('a'), letter('Y'))
	if got := n.Query().Text; got != "XabcY" {
		t.Errorf("a after typing: %q, want XabcY", got)
	}
}

func TestEscLeavesTyping(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithModes(true))
	m, _ = press(t, m, letter('i'))
	m = typeText(t, m, "crash")
	m, sent := press(t, m, esc)
	if len(sent) != 0 {
		t.Fatalf("the first esc sent %v", sent)
	}
	if m.Typing() || m.Query().Text != "crash" || m.Len() != 2 {
		t.Errorf("typing %v, query %q, len %d; want normal mode with the query kept and filtered", m.Typing(), m.Query().Text, m.Len())
	}
	// j moves in the narrowed list instead of typing.
	m, _ = press(t, m, letter('j'))
	if got := chosen(t, m); got != fix.Title {
		t.Errorf("selected %q, want the second result", got)
	}
	m, sent = press(t, m, esc)
	if len(sent) != 1 || sent[0] != (CancelMsg{ID: m.ID()}) {
		t.Errorf("the second esc sent %v, want a CancelMsg", sent)
	}
}

func TestTypingWithModesKeepsToday(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithModes(true))
	m, _ = press(t, m, letter('i'))
	m = typeText(t, m, "crash")
	m, _ = press(t, m, ctrlN)
	m, _ = press(t, m, down)
	if got := chosen(t, m); got != fix.Title {
		t.Errorf("selected %q, want the second result", got)
	}
	m, _ = press(t, m, letter('j'))
	if m.Query().Text != "crashj" {
		t.Errorf("query %q, want j typed", m.Query().Text)
	}
}

func TestFocusReturnsToNormalMode(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithModes(true))
	m, _ = press(t, m, letter('i'))
	m.Blur()
	if m.Typing() {
		t.Error("typing while blurred")
	}
	if cmd := m.Focus(); cmd != nil {
		t.Error("Focus started the cursor in normal mode")
	}
	m, _ = press(t, m, letter('x'))
	if m.Typing() || m.Query().Text != "" {
		t.Errorf("typing %v, query %q after Focus", m.Typing(), m.Query().Text)
	}
}

// A result that a debounced search returns after esc is shown, and the
// selection stays in range.
func TestResultLandsInNormalMode(t *testing.T) {
	f := &fakeSearch{}
	m := open(t, f.search, WithModes(true), WithDebounce(time.Hour))
	m, _ = press(t, m, letter('i'))
	m, _ = m.Update(letter('c'))
	m, _ = press(t, m, esc)
	// The search for "c" was already on its way when esc came.
	m, _ = run(t, m, m.searchCmd())
	if m.Typing() || m.Loading() {
		t.Fatalf("typing %v, loading %v", m.Typing(), m.Loading())
	}
	if m.Len() != 4 {
		t.Fatalf("results %q, want the 4 that contain c", titlesOf(m))
	}
	m, _ = press(t, m, letter('G'), letter('j'))
	if m.sel != m.Len()-1 {
		t.Errorf("sel %d of %d", m.sel, m.Len())
	}
}

func TestSetKeyMapWhileTyping(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithModes(true))
	m, _ = press(t, m, letter('i'))
	k := testKeys(t)
	k.Cancel.SetKeys("ctrl+g")
	m.SetKeyMap(k)
	if !m.Typing() {
		t.Fatal("SetKeyMap left typing mode")
	}
	m, sent := press(t, m, esc)
	if len(sent) != 0 || !m.Typing() {
		t.Errorf("old cancel key acted: sent %v, typing %v", sent, m.Typing())
	}
	m, sent = press(t, m, ctrl('g'))
	if len(sent) != 0 || m.Typing() {
		t.Errorf("new cancel key: sent %v, typing %v; want normal mode", sent, m.Typing())
	}
}

func TestPasteMsg(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithModes(true))
	m, _ = press(t, m, tea.PasteMsg{Content: "crash"})
	if m.Query().Text != "" {
		t.Errorf("paste in normal mode: query %q", m.Query().Text)
	}
	m, _ = press(t, m, letter('i'), tea.PasteMsg{Content: "crash"})
	if m.Query().Text != "crash" || m.Len() != 2 {
		t.Errorf("paste while typing: query %q, len %d", m.Query().Text, m.Len())
	}
}

// A picker without modes keeps its keys: letters type, and none of the
// normal keys act.
func TestModesOffUnchanged(t *testing.T) {
	tests := []struct {
		name  string
		keys  []tea.Msg
		want  string // the title enter chooses
		query string
	}{
		{"letters type", []tea.Msg{letter('j'), letter('k')}, "", "jk"},
		{"g and G type", []tea.Msg{letter('g'), letter('G')}, "", "gG"},
		{"i and a type", []tea.Msg{letter('i'), letter('a')}, "", "ia"},
		{"ctrl+n moves", []tea.Msg{ctrlN}, dotfiles.Title, ""},
		{"down moves", []tea.Msg{down, down}, dotfiles.Title, ""},
		{"pgup and pgdown move", []tea.Msg{pgDown, pgUp}, ghTUI.Title, ""},
		{"ctrl+u clears nothing", []tea.Msg{ctrl('u')}, ghTUI.Title, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, nil, WithItems([]Item{ghTUI, dotfiles}))
			if !m.Typing() || !m.Capturing() {
				t.Error("not typing")
			}
			m, _ = press(t, m, tt.keys...)
			if m.Query().Text != tt.query {
				t.Errorf("query %q, want %q", m.Query().Text, tt.query)
			}
			if tt.want != "" {
				if got := chosen(t, m); got != tt.want {
					t.Errorf("selected %q, want %q", got, tt.want)
				}
			}
		})
	}
	m := open(t, nil, WithItems(catalog))
	m, sent := press(t, m, esc)
	if len(sent) != 1 || sent[0] != (CancelMsg{ID: m.ID()}) {
		t.Errorf("esc sent %v, want a CancelMsg", sent)
	}
	m.Blur()
	if m.Typing() {
		t.Error("typing while blurred")
	}
}

func TestSetMarkedKeepsSelection(t *testing.T) {
	items := manyItems(30)
	values := make([]any, len(items))
	for i := range items {
		items[i].Value = i
		values[i] = i
	}
	m := open(t, nil, WithItems(items), WithModes(true), WithMarks("[x]", "[ ]"))
	m, _ = press(t, m, letter('G'), letter('k'), letter('k'))
	sel, top := m.sel, m.top
	view := m.View()
	m.SetMarked([]any{28, 29, "nothing", nil, []int{1}})
	if m.sel != sel || m.top != top || m.Len() != 30 {
		t.Errorf("sel %d top %d len %d; want %d %d 30", m.sel, m.top, m.Len(), sel, top)
	}
	if m.View() == view {
		t.Error("the view didn't change")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "[x] octo-org/project-29") || !strings.Contains(v, "[ ] octo-org/project-27") {
		t.Errorf("marks are off:\n%s", v)
	}
	// The set is copied, and nil clears it.
	values[0] = 29
	m.SetMarked(nil)
	if strings.Contains(ansi.Strip(m.View()), "[x]") || m.sel != sel {
		t.Errorf("nil left marks, or moved:\n%s", m.View())
	}
	values[0] = 0
	m.SetMarked(values[:1])
	values[0] = 29
	if strings.Contains(ansi.Strip(m.View()), "[x] octo-org/project-29") {
		t.Error("SetMarked kept the caller's slice")
	}
}

func TestSelectMovesToTheValue(t *testing.T) {
	items := manyItems(30)
	for i := range items {
		items[i].Value = i
	}
	m := open(t, nil, WithItems(items), WithSize(40, 8))
	if !m.Select(25) {
		t.Fatal("Select(25) found nothing")
	}
	if it, ok := m.Selected(); !ok || it.Value != 25 {
		t.Errorf("selected %v, want 25", it.Value)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "project-25") {
		t.Errorf("the selection isn't in view:\n%s", v)
	}
	if m.Select("nothing") || m.Select([]int{1}) {
		t.Error("Select found a value that isn't listed")
	}
	if it, _ := m.Selected(); it.Value != 25 {
		t.Errorf("a miss moved the selection to %v", it.Value)
	}
}

func TestTypedItem(t *testing.T) {
	use := func(text string) (Item, bool) {
		if text == "!" {
			return Item{}, false
		}
		return Item{Title: `use "` + text + `"`, Value: "typed:" + text}, true
	}
	last := func(m Model) string {
		if m.Len() == 0 {
			return ""
		}
		return m.results[m.Len()-1].Title
	}
	typing := func(m Model) Model {
		m, _ = press(t, m, letter('i'))
		return m
	}

	t.Run("shown with no exact match", func(t *testing.T) {
		m := typing(open(t, nil, WithItems(catalog), WithModes(true), WithTyped(use)))
		m = typeText(t, m, "crash")
		if got := last(m); got != `use "crash"` || m.Len() != 3 {
			t.Errorf("last %q of %d", got, m.Len())
		}
		if m.rows[len(m.rows)-1].item != 2 {
			t.Error("the typed item has no row of its own")
		}
	})
	t.Run("not shown for an empty query", func(t *testing.T) {
		m := typing(open(t, nil, WithItems(catalog), WithModes(true), WithTyped(use)))
		if m.Len() != len(catalog) {
			t.Errorf("len %d", m.Len())
		}
	})
	t.Run("not shown when not ok", func(t *testing.T) {
		m := open(t, nil, WithItems(catalog), WithTyped(use))
		m = typeText(t, m, "!")
		if m.Len() != 0 {
			t.Errorf("len %d, want none", m.Len())
		}
	})
	t.Run("not shown with an exact match, ignoring case", func(t *testing.T) {
		m := typing(open(t, nil, WithItems(catalog), WithModes(true), WithTyped(use)))
		m = typeText(t, m, "EGGZEC/GH-TUI")
		if strings.HasPrefix(last(m), "use ") {
			t.Errorf("title: typed item listed after %d results", m.Len())
		}
		// The value counts too, which a search may return for other words.
		person := func(context.Context, Query) ([]Item, error) {
			return []Item{{Title: "The Octocat", Value: "octocat"}}, nil
		}
		p := typing(open(t, person, WithModes(true), WithTyped(use)))
		p = typeText(t, p, "OCTOCAT")
		if strings.HasPrefix(last(p), "use ") || p.Len() != 1 {
			t.Errorf("value: typed item listed after %d results", p.Len())
		}
		p = typeText(t, p, "x")
		if got := last(p); got != `use "OCTOCATx"` || p.Len() != 2 {
			t.Errorf("no match: last %q of %d", got, p.Len())
		}
	})
	t.Run("not shown while loading or after an error", func(t *testing.T) {
		f := &fakeSearch{}
		m := open(t, f.search, WithModes(true), WithTyped(use))
		m = typing(m)
		m, cmd := m.Update(letter('c'))
		if !m.Loading() {
			t.Fatal("not loading")
		}
		if strings.HasPrefix(last(m), "use ") {
			t.Error("shown while loading")
		}
		m, _ = run(t, m, cmd)
		if got := last(m); got != `use "c"` {
			t.Errorf("after the search, last %q", got)
		}
		f.fail = errBoom
		m = typeText(t, m, "r")
		if m.Err() == nil {
			t.Fatal("no error")
		}
		if strings.HasPrefix(last(m), "use ") || m.Len() != 0 {
			t.Errorf("shown after an error: len %d", m.Len())
		}
	})
	t.Run("stays listed in normal mode", func(t *testing.T) {
		m := typing(open(t, nil, WithItems(catalog), WithModes(true), WithTyped(use)))
		m = typeText(t, m, "crash")
		m, _ = press(t, m, esc)
		if m.Len() != 3 || last(m) != `use "crash"` {
			t.Errorf("len %d, last %q in normal mode, want the typed item", m.Len(), last(m))
		}
		m, _ = press(t, m, letter('G'))
		_, sent := press(t, m, enter)
		want := ChosenMsg{ID: m.ID(), Item: Item{Title: `use "crash"`, Value: "typed:crash"}}
		if len(sent) != 1 || sent[0] != want {
			t.Errorf("sent %v, want %v", sent, want)
		}
	})
	t.Run("enter after esc chooses what was typed", func(t *testing.T) {
		m := typing(open(t, nil, WithItems(catalog), WithModes(true), WithTyped(use)))
		m = typeText(t, m, "newlabel")
		m, _ = press(t, m, esc)
		_, sent := press(t, m, enter)
		want := ChosenMsg{ID: m.ID(), Item: Item{Title: `use "newlabel"`, Value: "typed:newlabel"}}
		if len(sent) != 1 || sent[0] != want {
			t.Errorf("sent %v, want %v", sent, want)
		}
	})
	t.Run("has a header of its own, unless headers are off", func(t *testing.T) {
		m := open(t, nil, WithItems(catalog), WithTyped(use))
		m = typeText(t, m, "crash")
		n := len(m.rows)
		if m.rows[n-1].item != 2 || m.rows[n-2].header != typedHeader || m.rows[n-2].item >= 0 {
			t.Errorf("last rows %+v, want the typed header and then the item", m.rows[n-2:])
		}
		// With the group headers off it has none either.
		m = open(t, nil, WithItems(catalog), WithTyped(use), WithGroupHeaders(false))
		m = typeText(t, m, "crash")
		for _, r := range m.rows {
			if r.item < 0 {
				t.Errorf("without group headers: rows %+v hold a header", m.rows)
			}
		}
		if n := len(m.rows); m.rows[n-1].item != 2 {
			t.Errorf("without group headers: last rows %+v, want the typed item", m.rows[n-1:])
		}
	})
	t.Run("is not counted", func(t *testing.T) {
		m := open(t, nil, WithItems(catalog), WithTyped(use))
		m = typeText(t, m, "crash")
		if v := ansi.Strip(m.View()); !strings.Contains(v, "2 results") || strings.Contains(v, "3 results") {
			t.Errorf("view counts the typed item:\n%s", v)
		}
	})
	t.Run("is compared trimmed", func(t *testing.T) {
		person := func(context.Context, Query) ([]Item, error) {
			return []Item{{Title: "The Octocat ", Value: " octocat"}}, nil
		}
		m := open(t, person, WithTyped(use))
		m = typeText(t, m, "OCTOCAT ")
		if strings.HasPrefix(last(m), "use ") || m.Len() != 1 {
			t.Errorf("value: typed item listed after %d results", m.Len())
		}
		m = open(t, person, WithTyped(use))
		m = typeText(t, m, " the octocat")
		if strings.HasPrefix(last(m), "use ") || m.Len() != 1 {
			t.Errorf("title: typed item listed after %d results", m.Len())
		}
	})
	t.Run("enter while a search is pending chooses the typed text", func(t *testing.T) {
		for _, debounce := range []time.Duration{0, time.Hour} {
			f := &fakeSearch{}
			m := New(f.search, WithKeyMap(testKeys(t)), WithDebounce(debounce), WithSize(60, 12), WithTyped(use))
			m.Focus()
			m, _ = run(t, m, m.Init())
			if got := chosen(t, m); got != ghTUI.Title {
				t.Fatalf("debounce %v: selected %q before typing", debounce, got)
			}
			// The query changes, and enter comes before the results do.
			m, _ = m.Update(letter('c'))
			if !m.Loading() {
				t.Fatalf("debounce %v: not loading", debounce)
			}
			m, sent := press(t, m, enter)
			want := ChosenMsg{ID: m.ID(), Item: Item{Title: `use "c"`, Value: "typed:c"}}
			if len(sent) != 1 || sent[0] != want {
				t.Errorf("debounce %v: sent %v, want %v", debounce, sent, want)
			}
		}
	})
	t.Run("chosen with enter", func(t *testing.T) {
		m := typing(open(t, nil, WithItems(catalog), WithModes(true), WithTyped(use)))
		m = typeText(t, m, "crash")
		m, _ = press(t, m, down, down)
		_, sent := press(t, m, enter)
		want := ChosenMsg{ID: m.ID(), Item: Item{Title: `use "crash"`, Value: "typed:crash"}}
		if len(sent) != 1 || sent[0] != want {
			t.Errorf("sent %v, want %v", sent, want)
		}
	})
	t.Run("shown in every scope", func(t *testing.T) {
		m := typing(open(t, nil, WithItems(catalog), WithModes(true), WithTyped(use), WithScopes(kindRepos, kindIssues)))
		m = typeText(t, m, "crash")
		for range 3 {
			if got := last(m); got != `use "crash"` {
				t.Errorf("scope %q: last %q", m.Query().Scope, got)
			}
			m, _ = press(t, m, tab)
		}
	})
	t.Run("without modes", func(t *testing.T) {
		m := open(t, nil, WithItems(catalog), WithTyped(use))
		m = typeText(t, m, "crash")
		if got := last(m); got != `use "crash"` {
			t.Errorf("last %q", got)
		}
	})
}

func TestNoFilterLine(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithModes(true), WithFilterLine(false), WithSize(40, 8))
	if m.KeyMap().Normal.Insert.Enabled() || m.KeyMap().Normal.Append.Enabled() {
		t.Error("i and a are enabled without a filter line")
	}
	if m.listHeight() != 6 {
		t.Errorf("list height %d, want the whole 6", m.listHeight())
	}
	m, _ = press(t, m, letter('i'), letter('a'), letter('x'))
	if m.Typing() || m.Capturing() || m.Query().Text != "" {
		t.Errorf("typing %v, query %q", m.Typing(), m.Query().Text)
	}
	m, _ = press(t, m, letter('j'))
	if got := chosen(t, m); got != dotfiles.Title {
		t.Errorf("selected %q, want the second", got)
	}
	// 4 items and 3 headers fit in 6 rows only with scrolling.
	m, _ = press(t, m, letter('G'))
	if got := chosen(t, m); got != fix.Title {
		t.Errorf("selected %q, want the last", got)
	}
	assertFits(t, m.View(), 40, 8)
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n")[1:3] {
		if strings.Contains(row, "Search") || strings.Contains(row, "results") {
			t.Errorf("the filter line is drawn: %q", row)
		}
	}
	// Without modes, a picker without a filter line doesn't type either.
	p := open(t, nil, WithItems(catalog), WithFilterLine(false))
	p, _ = press(t, p, letter('x'), down)
	if p.Query().Text != "" || p.Typing() {
		t.Errorf("query %q, typing %v", p.Query().Text, p.Typing())
	}
	if got := chosen(t, p); got != dotfiles.Title {
		t.Errorf("selected %q, want the second", got)
	}
}

// Without a filter line and without modes nothing types, so a paste has
// nowhere to go.
func TestPasteWithoutFilterLineOrModes(t *testing.T) {
	m := open(t, nil, WithItems(catalog), WithFilterLine(false))
	m, _ = press(t, m, tea.PasteMsg{Content: "crash"})
	if m.Query().Text != "" || m.Len() != len(catalog) {
		t.Errorf("query %q, len %d; want the picker untouched", m.Query().Text, m.Len())
	}
}

// The typed item comes and goes with the search: not while it runs, back
// when it stops with the old results listed, and gone again when a new one
// starts.
func TestTypedItemFollowsTheSearch(t *testing.T) {
	use := func(text string) (Item, bool) { return Item{Title: "use " + text}, true }
	f := &fakeSearch{}
	m := open(t, f.search, WithTyped(use))
	m = typeText(t, m, "crash")
	if m.Len() != 3 || !m.typedAt {
		t.Fatalf("len %d, typed %v after the search", m.Len(), m.typedAt)
	}
	// A new search starts: the row goes at once.
	m, _ = m.Update(letter('x'))
	if m.typedAt || m.Len() != 2 {
		t.Errorf("refresh: len %d, typed %v; want the row gone", m.Len(), m.typedAt)
	}
	// It stops, as it does when a key chooses: the old results stay, with
	// the row back.
	m.stop()
	if !m.typedAt || m.Len() != 3 {
		t.Errorf("stop: len %d, typed %v; want the row back", m.Len(), m.typedAt)
	}
}

// Values that can't be compared are never marked, and never panic.
func TestEqualWithUncomparableValues(t *testing.T) {
	if equal([]int{1}, []int{1}) {
		t.Error("slices are equal")
	}
	if !equal("a", "a") || equal("a", "b") || equal("a", 1) {
		t.Error("comparable values compare wrongly")
	}
	items := []Item{{Title: "alpha", Value: []int{1}}}
	m := open(t, nil, WithItems(items), WithMarks("[x]", "[ ]"))
	m.SetMarked([]any{[]int{1}})
	if strings.Contains(ansi.Strip(m.View()), "[x]") {
		t.Error("an uncomparable value is marked")
	}
}

// A blank mark takes the width of the other, so the titles stay aligned.
func TestMarksShareTheirWidth(t *testing.T) {
	m := open(t, nil, WithItems(markable), WithMarks("●", " "))
	m.SetMarked([]any{"a"})
	col := func(title string) int {
		for l := range strings.SplitSeq(ansi.Strip(m.View()), "\n") {
			if before, _, ok := strings.Cut(l, title); ok {
				return ansi.StringWidth(before)
			}
		}
		t.Fatalf("no row for %q", title)
		return 0
	}
	if a, c := col("alpha"), col("gamma"); a != c {
		t.Errorf("titles at columns %d and %d, want one", a, c)
	}
	// The other way round: a wide off mark pads the on mark.
	m = open(t, nil, WithItems(markable), WithMarks("x", "[ ]"))
	m.SetMarked([]any{"a"})
	if a, c := col("alpha"), col("gamma"); a != c {
		t.Errorf("wide off mark: titles at columns %d and %d, want one", a, c)
	}
}
