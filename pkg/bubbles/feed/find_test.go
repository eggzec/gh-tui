package feed

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// typed presses the keys named esc, enter and backspace, and types every
// other part rune by rune.
func typed[T any](tb testing.TB, m Model[T], parts ...string) Model[T] {
	tb.Helper()
	for _, p := range parts {
		switch p {
		case "esc", "enter", "backspace":
			m = keys(tb, m, p)
		default:
			for _, r := range p {
				m = keys(tb, m, string(r))
			}
		}
	}
	return m
}

// bugs is a source of n items in chunks of 10, where every fifth item,
// from the third, is a "bug fix" and the rest are "item".
func bugs(n int) *source {
	src := newSource(n, 10)
	for i := range src.items {
		if i%5 == 2 {
			src.items[i].title = fmt.Sprintf("bug fix %d", i)
		}
	}
	return src
}

func selectedID(tb testing.TB, m Model[item]) string {
	tb.Helper()
	it, ok := m.Selected()
	if !ok {
		tb.Fatal("nothing is selected")
	}
	return it.id
}

func TestFind(t *testing.T) {
	tests := []struct {
		name      string
		keys      []string
		want      string
		wantNote  string
		wantCount int
	}{
		{"moves to the first match", []string{"/", "bug", "enter"}, "2", "", 2},
		{"n goes on", []string{"/", "bug", "enter", "n"}, "7", "", 2},
		{"n wraps around the end", []string{"/", "bug", "enter", "n", "n"}, "2", "", 2},
		{"N goes back, wrapping around the start", []string{"/", "bug", "enter", "N"}, "7", "", 2},
		{"N goes back", []string{"/", "bug", "enter", "n", "N"}, "2", "", 2},
		{"the match at the selection is the first", []string{"j", "j", "/", "bug", "enter"}, "2", "", 2},
		{"a find starts at the selection", []string{"j", "j", "j", "/", "bug", "enter"}, "7", "", 2},
		{"nothing matching leaves the selection", []string{"j", "/", "zzz", "enter"}, "1", noteNotFound, 0},
		{"n does nothing after nothing matched", []string{"/", "zzz", "enter", "n"}, "0", "", 0},
		{"an empty line changes nothing", []string{"/", "enter"}, "0", "", 0},
		{"esc closes the prompt", []string{"/", "bug", "esc"}, "0", "", 0},
		{"esc clears the find", []string{"/", "bug", "enter", "esc", "n"}, "2", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := typed(t, load(t, bugs(10)), tt.keys...)
			if got := selectedID(t, m); got != tt.want {
				t.Errorf("selected %s, want %s", got, tt.want)
			}
			if m.note != tt.wantNote {
				t.Errorf("note %q, want %q", m.note, tt.wantNote)
			}
			if got := m.Matches(); got != tt.wantCount {
				t.Errorf("%d matches, want %d", got, tt.wantCount)
			}
			assertVisible(t, m)
			assertFits(t, m.View(), m.Width(), m.Height())
		})
	}
}

// The note stays until the next key.
func TestFindNoteGoesOnNextKey(t *testing.T) {
	m := typed(t, load(t, bugs(10)), "/", "zzz", "enter")
	if !strings.Contains(ansi.Strip(m.View()), noteNotFound) {
		t.Fatalf("view lacks the note:\n%s", ansi.Strip(m.View()))
	}
	m = keys(t, m, "j")
	if strings.Contains(ansi.Strip(m.View()), noteNotFound) {
		t.Fatalf("view keeps the note after a key:\n%s", ansi.Strip(m.View()))
	}
}

func TestFindSmartCase(t *testing.T) {
	src := newSource(10, 10)
	src.items[1].title = "Bug in the parser"
	src.items[4].title = "bug in the lexer"
	src.items[8].title = "BUG"
	tests := []struct {
		query string
		want  []string
	}{
		{"bug", []string{"1", "4", "8"}},
		{"Bug", []string{"1"}},
		{"BUG", []string{"8"}},
		{"bug in", []string{"1", "4"}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			m := typed(t, load(t, src), "/", tt.query, "enter")
			var got []string
			for _, p := range m.hits {
				got = append(got, m.rows2id(p))
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("matches %v, want %v", got, tt.want)
			}
		})
	}
}

func (m Model[T]) rows2id(p int) string {
	it, _ := m.item(m.at(p))
	return any(it).(item).id
}

// A find looks at the rows that are loaded, and so does a filter.
func TestFindLooksAtLoadedRows(t *testing.T) {
	src := bugs(100)
	m := typed(t, load(t, src), "/", "bug", "enter")
	if got := m.Matches(); got != 2 {
		t.Errorf("%d matches, want the 2 of the 10 loaded rows", got)
	}
	if got := src.callCount(); got != 1 {
		t.Errorf("a find fetched %d times, want 1", got)
	}
}

func TestQuickFilter(t *testing.T) {
	m := typed(t, load(t, bugs(30)), "&", "bug", "enter")
	if m.FilterQuery() != "bug" || m.Shown() != 2 || m.Shown() != 2 {
		t.Fatalf("filter %q shows %d rows, want bug and 2", m.FilterQuery(), m.Shown())
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"#2 bug fix 2", "#7 bug fix 7", "&bug", "2 in 10 loaded"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	for _, hidden := range []string{"item 0", "item 1", "item 3"} {
		if strings.Contains(view, hidden) {
			t.Errorf("view shows %q, which the filter hides:\n%s", hidden, view)
		}
	}
	assertFits(t, m.View(), m.Width(), m.Height())

	m = keys(t, m, "esc")
	if m.FilterQuery() != "" || m.Shown() != 10 {
		t.Errorf("after esc, filter %q shows %d rows, want none and 10", m.FilterQuery(), m.Shown())
	}
	if view := ansi.Strip(m.View()); strings.Contains(view, "&bug") || !strings.Contains(view, "item 0") {
		t.Errorf("esc left the chip, or the rows hidden:\n%s", view)
	}
}

func TestQuickFilterOfAllLoadedRowsSaysOf(t *testing.T) {
	m := typed(t, load(t, bugs(10)), "&", "bug", "enter")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "2 of 10") || strings.Contains(view, "loaded") {
		t.Errorf("view doesn't say 2 of 10:\n%s", view)
	}
}

// The selection stays on its item if the filter keeps it, and moves to the
// next row it keeps if not, and goes back to its item when the filter goes.
func TestQuickFilterKeepsCursorValid(t *testing.T) {
	m := typed(t, load(t, bugs(10)), "j", "j", "j", "j") // item 4
	m = typed(t, m, "&", "bug", "enter")
	if got := selectedID(t, m); got != "7" {
		t.Errorf("filter moved the selection to %s, want 7, the next row it keeps", got)
	}
	assertVisible(t, m)
	m = keys(t, m, "k")
	if got := selectedID(t, m); got != "2" {
		t.Errorf("k selected %s, want 2", got)
	}
	m = keys(t, m, "esc")
	if got := selectedID(t, m); got != "2" {
		t.Errorf("esc selected %s, want the item it was on, 2", got)
	}
	assertVisible(t, m)

	m = typed(t, load(t, bugs(10)), "j", "j", "j", "j", "j", "j", "j", "j")
	m = typed(t, m, "&", "bug", "enter")
	if got := selectedID(t, m); got != "7" {
		t.Errorf("filter moved the selection to %s, want 7, the last row it keeps", got)
	}
	// Past the last match, the selection goes to the last row.
	m = typed(t, load(t, bugs(10)), "G", "&", "bug", "enter")
	if got := selectedID(t, m); got != "7" {
		t.Errorf("filter moved the selection to %s, want 7", got)
	}
	m = keys(t, m, "G", "g", "j", "j", "ctrl+d", "ctrl+f", "ctrl+b", "ctrl+u")
	assertVisible(t, m)
}

func TestQuickFilterWithNoMatch(t *testing.T) {
	m := typed(t, load(t, bugs(10)), "j", "&", "zzz", "enter")
	if m.Shown() != 0 {
		t.Fatalf("filter shows %d rows, want none", m.Shown())
	}
	if _, ok := m.Selected(); ok {
		t.Error("a row is selected with none shown")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "No loaded row matches") || !strings.Contains(view, "&zzz") {
		t.Errorf("view doesn't say nothing matches:\n%s", view)
	}
	assertFits(t, m.View(), m.Width(), m.Height())
	m = keys(t, m, "j", "k", "G", "g", "ctrl+f", "ctrl+u")
	assertFits(t, m.View(), m.Width(), m.Height())
	m = keys(t, m, "esc")
	if got := selectedID(t, m); got != "1" {
		t.Errorf("esc selected %s, want the item it was on, 1", got)
	}
}

// A filter over a paged list looks at what is loaded, and fetches the next
// chunk only when the user goes past its last row.
func TestQuickFilterDoesNotFetchEverything(t *testing.T) {
	src := bugs(1000)
	m := typed(t, load(t, src), "&", "bug", "enter")
	if got := src.callCount(); got != 1 {
		t.Fatalf("filtering fetched %d times, want 1", got)
	}
	m = typed(t, m, "&", "zzz", "enter")
	m = keys(t, m, "j")
	if got := src.callCount(); got != 2 {
		t.Fatalf("going past the last row fetched %d times in all, want 2", got)
	}
	if m.Loaded() != 20 {
		t.Errorf("%d items loaded, want 20", m.Loaded())
	}
	m = typed(t, m, "&", "bug", "enter")
	if m.Shown() != 4 {
		t.Errorf("filter shows %d rows, want 4 of the 20 loaded", m.Shown())
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "4 in 20 loaded") {
		t.Errorf("view lacks the loaded count:\n%s", view)
	}
	// Rows that arrive while the filter shows are filtered too.
	m = keys(t, m, "j", "j", "j", "j")
	if got := src.callCount(); got != 3 {
		t.Errorf("fetched %d times in all, want 3", got)
	}
	if m.Shown() != 6 {
		t.Errorf("filter shows %d rows, want 6 of the 30 loaded", m.Shown())
	}
}

// Only moving down from the last row of a filter, or from where it shows
// none, fetches the next chunk.
func TestQuickFilterFetchesOnlyPastItsEnd(t *testing.T) {
	tests := []struct {
		name  string
		keys  []string
		calls int
	}{
		{"down from the last row", []string{"&", "bug", "enter", "j", "j"}, 2},
		{"down from an earlier row", []string{"&", "bug", "enter", "j"}, 1},
		{"page down from an earlier row", []string{"&", "bug", "enter", "ctrl+f"}, 1},
		{"home with no match", []string{"&", "zzz", "enter", "g"}, 1},
		{"end with no match", []string{"&", "zzz", "enter", "G"}, 1},
		{"down with no match", []string{"&", "zzz", "enter", "j"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := bugs(1000)
			m := typed(t, load(t, src), tt.keys...)
			if got := src.callCount(); got != tt.calls {
				t.Errorf("fetched %d times, want %d", got, tt.calls)
			}
			assertFits(t, m.View(), m.Width(), m.Height())
		})
	}
}

// What the rows read is made once for each chunk, not for each search.
func TestMatchTextIsKept(t *testing.T) {
	src := bugs(10)
	renders := 0
	m := newModel(src.fetch, func(it item, sel bool, w int) string {
		renders++
		return renderItem(it, sel, w)
	}, WithSize(40, 5), WithFocused(true))
	m = run(t, m, m.Init())
	m = typed(t, m, "&", "bug", "enter")
	first := renders
	m = typed(t, m, "&", "item", "enter", "&", "fix", "enter", "/", "7", "enter")
	// The rows on screen are drawn again, but no loaded item is read again.
	if renders-first > 3*m.Height()+2*10 {
		t.Errorf("rendered %d times for searches after %d", renders-first, first)
	}
	if m.chunks[0].texts == nil {
		t.Error("the chunk keeps no texts")
	}
	m = run(t, m, (&m).Reload())
	if m.chunks[0].texts != nil && len(m.chunks[0].texts) != 10 {
		t.Error("the texts of a chunk fetched again are stale")
	}
}

func TestPromptHelp(t *testing.T) {
	m := load(t, bugs(10))
	rows := m.FullHelp()
	// The rows of the key map, and the one of the mark key.
	if got, want := len(rows), len(m.KeyMap().FullHelp())+1; got != want {
		t.Errorf("full help has %d rows with nothing to clear, want %d", got, want)
	}
	m = typed(t, m, "&", "bug", "enter")
	last := m.FullHelp()[len(m.FullHelp())-1]
	if len(last) != 1 || last[0].Help().Desc != "cancel" || !last[0].Enabled() {
		t.Errorf("with a filter shown, full help should end with the cancel key: %v", last)
	}
	m = keys(t, m, "/")
	for _, b := range m.ShortHelp() {
		if !b.Enabled() {
			t.Errorf("short help lists %q off while the prompt is open", b.Help().Desc)
		}
	}
	if got := m.ShortHelp()[0].Help().Desc; got != "search" {
		t.Errorf("enter at the find prompt says %q, want search", got)
	}
	for _, g := range m.FullHelp() {
		for _, b := range g {
			if b.Enabled() && b.Help().Desc != "search" && b.Help().Desc != "cancel" && b.Help().Desc != "cancel when empty" {
				t.Errorf("%q is on while the prompt is open", b.Help().Desc)
			}
		}
	}
}

func TestQuickFilterEmptyLineClears(t *testing.T) {
	m := typed(t, load(t, bugs(10)), "&", "bug", "enter", "&", "enter")
	if m.FilterQuery() != "" || m.Shown() != 10 {
		t.Errorf("filter %q shows %d rows, want none and 10", m.FilterQuery(), m.Shown())
	}
}

// Esc clears the find, and then the filter.
func TestEscPeelsFindThenFilter(t *testing.T) {
	m := typed(t, load(t, bugs(10)), "&", "bug", "enter", "/", "7", "enter")
	if got := selectedID(t, m); got != "7" {
		t.Fatalf("selected %s, want 7", got)
	}
	m = keys(t, m, "esc")
	if m.FindQuery() != "" || m.FilterQuery() != "bug" {
		t.Errorf("after one esc, find %q and filter %q, want none and bug", m.FindQuery(), m.FilterQuery())
	}
	m = keys(t, m, "esc")
	if m.FilterQuery() != "" {
		t.Errorf("after two, filter %q, want none", m.FilterQuery())
	}
}

// While the prompt is open, every key is typed: n and & too.
func TestPromptTypesEveryKey(t *testing.T) {
	m := load(t, bugs(10))
	m = keys(t, m, "/")
	if !m.Capturing() {
		t.Fatal("/ didn't open the prompt")
	}
	m = typed(t, m, "n", "&", "N", "j", "/")
	if got := m.prompt.Value(); got != "n&Nj/" {
		t.Errorf("the prompt holds %q, want n&Nj/", got)
	}
	if got := selectedID(t, m); got != "0" || m.FilterQuery() != "" {
		t.Errorf("a key typed in the prompt acted: selected %s, filter %q", got, m.FilterQuery())
	}
	m = keys(t, m, "esc")
	if m.Capturing() {
		t.Error("esc left the prompt open")
	}
	// Backspace on an empty line closes the prompt.
	m = typed(t, m, "&", "backspace")
	if m.Capturing() {
		t.Error("backspace on an empty line left the prompt open")
	}
	// The prompt is the filter's after &.
	m = typed(t, m, "&", "bug")
	if got := ansi.Strip(m.View()); !strings.Contains(got, "&bug") {
		t.Errorf("view lacks the filter prompt:\n%s", got)
	}
	m.Blur()
	if m.Capturing() {
		t.Error("blurring left the prompt open")
	}
}

// Keys do nothing in a blurred feed, and the find keys only work while
// there is something to find.
func TestFindKeysEnabledWhenTheyWork(t *testing.T) {
	m := load(t, bugs(10))
	if k := m.KeyMap(); k.Next.Enabled() || k.Prev.Enabled() {
		t.Error("n or N is enabled with nothing to find")
	}
	if !m.KeyMap().Find.Enabled() || !m.KeyMap().QuickFilter.Enabled() {
		t.Error("/ or & is disabled")
	}
	m = keys(t, m, "/")
	m = typed(t, m, "bug", "enter")
	if k := m.KeyMap(); !k.Next.Enabled() || !k.Prev.Enabled() {
		t.Error("with a find shown, n and N should be enabled")
	}
	m = keys(t, m, "esc")
	if k := m.KeyMap(); k.Next.Enabled() || k.Prev.Enabled() {
		t.Error("n or N is enabled after the find was cleared")
	}
	m = typed(t, m, "&", "bug", "enter")
	// The feed ignores them while blurred.
	m.Blur()
	before := m.FilterQuery()
	m = keys(t, m, "esc", "/")
	if m.FilterQuery() != before || m.Capturing() {
		t.Error("a blurred feed reacted to a key")
	}
}

// A new query's rows are not those the find and filter looked at.
func TestResetClearsFindAndFilter(t *testing.T) {
	src := bugs(10)
	m := typed(t, load(t, src), "&", "bug", "enter", "/", "7", "enter")
	m = run(t, m, (&m).Reset())
	if m.FindQuery() != "" || m.FilterQuery() != "" || m.Shown() != 10 {
		t.Errorf("after Reset, find %q, filter %q, %d rows", m.FindQuery(), m.FilterQuery(), m.Shown())
	}
}

// Reload keeps the selection on its item while a filter shows.
func TestReloadUnderFilter(t *testing.T) {
	src := bugs(10)
	m := typed(t, load(t, src), "&", "bug", "enter", "j")
	if got := selectedID(t, m); got != "7" {
		t.Fatalf("selected %s, want 7", got)
	}
	m = run(t, m, (&m).Reload())
	if got := selectedID(t, m); got != "7" || m.Shown() != 2 {
		t.Errorf("after Reload, selected %s of %d rows, want 7 of 2", got, m.Shown())
	}
}

// texted says what its row reads, which is not what it renders.
type texted struct{ name, secret string }

func (t texted) Text() string { return t.name }

func TestItemsSayTheirText(t *testing.T) {
	items := []texted{{"alpha", "hidden"}, {"beta", "alpha"}, {"gamma", "x"}}
	m := New(func(context.Context, string) ([]texted, string, error) { return items, "", nil },
		func(it texted, _ bool, _ int) string { return it.secret },
		WithKeyMap(testKeyMap), WithPromptKeys(testPromptKeys), WithSize(40, 5), WithFocused(true))
	m = run(t, m, m.Init())
	m = typed(t, m, "&", "alpha", "enter")
	if m.Shown() != 1 {
		t.Fatalf("the filter shows %d rows, want the one named alpha", m.Shown())
	}
	if it, _ := m.Selected(); it.name != "alpha" {
		t.Errorf("selected %q, want alpha", it.name)
	}
}

func TestFindAndFilterViews(t *testing.T) {
	tests := []struct {
		name  string
		model func(t *testing.T) Model[item]
	}{
		{"find prompt", func(t *testing.T) Model[item] {
			t.Helper()
			return typed(t, load(t, bugs(30)), "/", "bu")
		}},
		{"filter prompt", func(t *testing.T) Model[item] {
			t.Helper()
			return typed(t, load(t, bugs(30)), "&", "bu")
		}},
		{"find shown", func(t *testing.T) Model[item] {
			t.Helper()
			return typed(t, load(t, bugs(10)), "/", "bug", "enter", "n")
		}},
		{"find not found", func(t *testing.T) Model[item] {
			t.Helper()
			return typed(t, load(t, bugs(10)), "/", "zzz", "enter")
		}},
		{"filter chip", func(t *testing.T) Model[item] {
			t.Helper()
			return typed(t, load(t, bugs(30)), "&", "bug", "enter")
		}},
		{"find of a paged list", func(t *testing.T) Model[item] {
			t.Helper()
			return typed(t, load(t, bugs(30)), "/", "bug", "enter")
		}},
		{"filter chip of all", func(t *testing.T) Model[item] {
			t.Helper()
			return typed(t, load(t, bugs(10)), "&", "bug", "enter")
		}},
		{"filter without rows", func(t *testing.T) Model[item] {
			t.Helper()
			return typed(t, load(t, bugs(30)), "&", "zzz", "enter")
		}},
		{"filter and find", func(t *testing.T) Model[item] {
			t.Helper()
			return typed(t, load(t, bugs(10)), "&", "bug", "enter", "/", "7", "enter")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model(t)
			v := m.View()
			assertFits(t, v, m.Width(), m.Height())
			golden.RequireEqual(t, v)
		})
	}
}
