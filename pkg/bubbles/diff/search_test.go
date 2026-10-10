package diff

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// searchSource has "foo" in three files, as "Foo" once. Its rows are: a.go
// header 0, hunk 1, then rows 2 to 4; b.go header 5, hunk 6, rows 7 and 8;
// c.go header 9, hunk 10, rows 11 and 12.
func searchSource() *source {
	return &source{size: 10, files: []File{
		fileOf("a.go", "@@ -1,3 +1,3 @@\n keep Foo\n-old foo\n+new bar"),
		fileOf("b.go", "@@ -1,2 +1,2 @@\n-foo gone\n+foo here"),
		fileOf("c.go", "@@ -1 +1 @@\n-x\n+tail foo"),
	}}
}

// typed presses the keys of a query, as typed into the input.
func typed(query string) []string {
	ks := make([]string, 0, len(query))
	for _, r := range query {
		if r == ' ' {
			ks = append(ks, "space")
			continue
		}
		ks = append(ks, string(r))
	}
	return ks
}

func search1(t *testing.T, m Model, query string) Model {
	t.Helper()
	m = keys(t, m, "/")
	m = keys(t, m, typed(query)...)
	return keys(t, m, "enter")
}

// A search reads the lines of every file, and goes to the first match at or
// after the cursor; n and N go through them across files, wrapping round.
func TestSearchAcrossFiles(t *testing.T) {
	m := search1(t, load(t, searchSource()), "foo")
	if got := m.Matches(); got != 5 {
		t.Fatalf("Matches = %d, want 5", got)
	}
	if m.Query() != "foo" {
		t.Errorf("Query = %q", m.Query())
	}
	want := []int{2, 3, 7, 8, 12, 2, 3}
	for i, row := range want {
		if i > 0 {
			m = keys(t, m, "n")
		}
		if m.Cursor() != row {
			t.Fatalf("after %d presses of n the cursor is on row %d, want %d", i, m.Cursor(), row)
		}
	}
	for _, row := range []int{2, 12, 8} {
		m = keys(t, m, "N")
		if m.Cursor() != row {
			t.Fatalf("N went to row %d, want %d", m.Cursor(), row)
		}
	}
}

func TestSearchStartsAtTheCursor(t *testing.T) {
	m := load(t, searchSource())
	m = keys(t, m, "J", "J") // c.go
	m = search1(t, m, "foo")
	if m.Cursor() != 12 {
		t.Errorf("cursor = %d, want the match at row 12", m.Cursor())
	}
	// Past the last match, the search wraps to the first.
	m = keys(t, load(t, searchSource()), "G")
	m = search1(t, m, "foo")
	if m.Cursor() != 12 {
		t.Errorf("cursor = %d, want 12, the last row", m.Cursor())
	}
	m = keys(t, m, "n")
	if m.Cursor() != 2 {
		t.Errorf("n from the last match went to %d, want 2", m.Cursor())
	}
}

// A query without capitals ignores case; one with a capital does not.
func TestSearchSmartCase(t *testing.T) {
	for _, tt := range []struct {
		query string
		want  int
	}{{"foo", 5}, {"FOO", 0}, {"Foo", 1}, {"fOo", 0}, {"gone", 1}, {"GONE", 0}} {
		m := search1(t, load(t, searchSource()), tt.query)
		if got := m.Matches(); got != tt.want {
			t.Errorf("%q: %d matches, want %d", tt.query, got, tt.want)
		}
	}
}

// The query is text, not a pattern.
func TestSearchIsLiteral(t *testing.T) {
	src := &source{files: []File{fileOf("a.go", "@@ -1 +1 @@\n-f(x)\n+f.x[0]")}, size: 5}
	m := search1(t, load(t, src), "f(x)")
	if m.Matches() != 1 {
		t.Errorf("f(x): %d matches, want 1", m.Matches())
	}
	m = search1(t, load(t, src), "f.x")
	if m.Matches() != 1 {
		t.Errorf("f.x: %d matches, want 1", m.Matches())
	}
}

// Tabs read as the spaces they are drawn with, and the headers and the
// notes are not searched.
func TestSearchReadsTheLinesShown(t *testing.T) {
	src := &source{files: []File{
		fileOf("foo/bar.go", "@@ -1 +1 @@ func foo()\n-a\tb\n+\\ foo"),
		{Path: "foo.png", Status: StatusAdded},
	}, size: 5}
	m := load(t, src)
	if got := search1(t, m, "a   b").Matches(); got != 1 {
		t.Errorf("a tab is not read as its spaces: %d matches", got)
	}
	if got := search1(t, m, "foo").Matches(); got != 1 {
		t.Errorf("foo: %d matches, want the one line", got)
	}
}

func TestSearchNoMatches(t *testing.T) {
	m := search1(t, load(t, searchSource()), "nothing")
	if m.Matches() != 0 || m.Query() != "nothing" {
		t.Fatalf("matches = %d, query = %q", m.Matches(), m.Query())
	}
	before := m.Cursor()
	m = keys(t, m, "n", "N")
	if m.Cursor() != before {
		t.Error("n moved the cursor with no match")
	}
	if !strings.Contains(ansi.Strip(m.View()), "no matches") {
		t.Errorf("the view does not say so:\n%s", ansi.Strip(m.View()))
	}
	if m.KeyMap().Next.Enabled() {
		t.Error("next match is enabled with none")
	}
}

// While the search input is open, every key is typed, and the keys of the
// view do nothing.
func TestSearchInputTakesEveryKey(t *testing.T) {
	m := load(t, searchSource())
	m = keys(t, m, "/")
	if !m.Capturing() {
		t.Fatal("the input is not open")
	}
	if !m.KeyMap().Confirm.Enabled() || !m.KeyMap().Cancel.Enabled() {
		t.Error("the keys that run and close the search are disabled")
	}
	before := m.Cursor()
	m = keys(t, m, "J", "n", "N", "G", "j", "/", "}")
	if m.Cursor() != before {
		t.Errorf("a typed key moved the cursor to %d", m.Cursor())
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "/Jn") || !strings.Contains(v, "/Jn"+"NGj/}") {
		t.Errorf("the input does not show what was typed:\n%s", v)
	}
	for _, g := range m.FullHelp() {
		for _, b := range g {
			if b.Enabled() && b.Help().Desc != "search" && b.Help().Desc != "cancel" {
				t.Errorf("%q works while the input is open", b.Help().Desc)
			}
		}
	}
	// Esc closes the input, and leaves no search.
	m = keys(t, m, "esc")
	if m.Capturing() || m.Query() != "" {
		t.Error("esc did not close the input")
	}
	// An empty query runs no search.
	m = keys(t, m, "/", "enter")
	if m.Capturing() || m.Query() != "" {
		t.Error("an empty query was searched for")
	}
}

// A blurred view closes the input.
func TestSearchInputClosesOnBlur(t *testing.T) {
	m := keys(t, load(t, searchSource()), "/", "f")
	m.Blur()
	if m.Capturing() {
		t.Error("the input stayed open on blur")
	}
}

// Esc clears the search, first; the view's own close is the parent's.
func TestSearchEscClears(t *testing.T) {
	m := search1(t, load(t, searchSource()), "foo")
	if !m.KeyMap().Cancel.Enabled() {
		t.Fatal("cancel is disabled while a search is shown")
	}
	at := m.Cursor()
	m = keys(t, m, "esc")
	if m.Query() != "" || m.Matches() != 0 {
		t.Errorf("esc left the search: %q, %d", m.Query(), m.Matches())
	}
	if m.Cursor() != at {
		t.Error("clearing the search moved the cursor")
	}
	if m.KeyMap().Cancel.Enabled() || m.KeyMap().Next.Enabled() {
		t.Error("the keys of a search that is gone are enabled")
	}
	if ansi.Strip(m.View()) != ansi.Strip(load(t, searchSource(), WithSize(80, 12)).View()) && strings.Contains(ansi.Strip(m.View()), "match") {
		t.Error("the status still shows the search")
	}
	// The parent clears it with ClearSearch, and learns whether it did.
	m = search1(t, load(t, searchSource()), "foo")
	if !m.ClearSearch() || m.Query() != "" {
		t.Error("ClearSearch did not clear the search")
	}
	if m.ClearSearch() {
		t.Error("ClearSearch reported a search that was gone")
	}
}

// A match in a folded file unfolds it, and the search counts it.
func TestSearchUnfoldsTheFileOfAMatch(t *testing.T) {
	src := &source{size: 5, files: []File{goFile("a.go", 1), goFile("big.go", 3), goFile("c.go", 1)}}
	m := load(t, src, WithCollapseOver(10))
	if !m.layout.Collapsed(1) || m.layout.Collapsed(0) {
		t.Fatal("big.go should be folded alone")
	}
	m = search1(t, m, "new b2")
	if m.Matches() != 1 {
		t.Fatalf("Matches = %d, want the one in the folded file", m.Matches())
	}
	if m.layout.Collapsed(1) {
		t.Error("the file of the match stayed folded")
	}
	row, ok := m.layout.RowAt(m.Cursor())
	if !ok || row.Text != "new b2" || row.File != 1 {
		t.Errorf("the cursor is on %+v, want the line new b2 of big.go", row)
	}
	// Other files stay as they were.
	if f, _ := m.layout.FileAt(m.Cursor()); f != 1 {
		t.Errorf("cursor in file %d", f)
	}
}

// The matches reach the pages that are not fetched yet, which a search
// fetches, and the first one found is jumped to.
func TestSearchFetchesTheRestOfTheFiles(t *testing.T) {
	files := sizedFiles(6, 1)
	files[5] = fileOf("last.go", "@@ -1 +1 @@\n-x\n+needle")
	src := &source{size: 1, files: files}
	m := load(t, src, WithSize(80, 6))
	if m.Done() {
		t.Fatal("every page was fetched without a search")
	}
	m = search1(t, m, "needle")
	if !m.Done() {
		t.Error("the search left pages unfetched")
	}
	if m.Matches() != 1 {
		t.Fatalf("Matches = %d, want 1", m.Matches())
	}
	if f, _ := m.layout.FileAt(m.Cursor()); f != 5 {
		t.Errorf("the cursor is in file %d, want the last", f)
	}
	// A page that arrives later is searched too: more matches appear.
	more := &source{size: 1, files: append(sizedFiles(3, 1), fileOf("x.go", "@@ -1 +1 @@\n-x\n+ctx a0"), fileOf("y.go", "@@ -1 +1 @@\n-x\n+ctx a0"))}
	m = search1(t, load(t, more, WithSize(80, 6)), "ctx a0")
	if m.Matches() != 5 || !m.Done() {
		t.Errorf("Matches = %d, done = %v; want 5 and done", m.Matches(), m.Done())
	}
}

// A key press ends the wait for a first match, so a late page does not
// take the cursor from where the user went, and none of the matches is
// current until n goes to the first one at or after the cursor.
func TestSearchWaitEndsOnKey(t *testing.T) {
	files := sizedFiles(4, 1)
	files[3] = fileOf("last.go", "@@ -1 +1 @@\n-x\n+needle")
	m := load(t, &source{size: 1, files: files}, WithSize(80, 6))
	m = keys(t, m, "/", "n", "e", "e", "d", "l", "e")
	m, pending := m.Update(press("enter"))
	if m.Matches() != 0 || m.Done() {
		t.Fatalf("matches = %d, done = %v; want a search still waiting", m.Matches(), m.Done())
	}
	m = keys(t, m, "j")
	at := m.Cursor()
	m = run(t, m, pending)
	if !m.Done() || m.Matches() != 1 {
		t.Fatalf("matches = %d, done = %v after the pages arrived", m.Matches(), m.Done())
	}
	if m.Cursor() != at {
		t.Errorf("a late match took the cursor from %d to %d", at, m.Cursor())
	}
	if m.search.cur != -1 {
		t.Errorf("match %d is current, though none was jumped to", m.search.cur)
	}
	st := m.Styles()
	if strings.Contains(m.View(), wrapOf(st.CurrentMatch).pre) {
		t.Error("a match is drawn as current")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "1 match") || strings.Contains(v, "match 1/") {
		t.Errorf("the status does not say one match, none current:\n%s", v)
	}
	m = keys(t, m, "n")
	if f, _ := m.layout.FileAt(m.Cursor()); f != 3 || m.search.cur != 0 {
		t.Errorf("n went to file %d, match %d; want match 0 in the last file", f, m.search.cur)
	}
}

// n and N go on from the cursor once it has left the current match, as in
// less: n to the first match after it, N to the last one before it.
func TestSearchStepsFromTheCursor(t *testing.T) {
	m := search1(t, load(t, searchSource()), "foo") // rows 2, 3, 7, 8, 12
	m = keys(t, m, "G")                             // row 12 is the last row; the match is there
	if m.Cursor() != 12 {
		t.Fatalf("cursor = %d", m.Cursor())
	}
	m = keys(t, m, "g", "j", "j", "j", "j", "j") // row 5: b.go header, no match on it
	if m.Cursor() != 5 {
		t.Fatalf("cursor = %d, want 5", m.Cursor())
	}
	if got := keys(t, m, "n").Cursor(); got != 7 {
		t.Errorf("n from row 5 went to %d, want 7", got)
	}
	if got := keys(t, m, "N").Cursor(); got != 3 {
		t.Errorf("N from row 5 went to %d, want 3", got)
	}
	// Past the last match, n wraps; before the first, N does.
	m = keys(t, m, "G")
	m = keys(t, m, "k") // row 11, off the match at 12
	if got := keys(t, m, "n").Cursor(); got != 12 {
		t.Errorf("n from row 11 went to %d, want 12", got)
	}
	m = keys(t, m, "g")
	if got := keys(t, m, "N").Cursor(); got != 12 {
		t.Errorf("N from the top went to %d, want to wrap to 12", got)
	}
	if got := keys(t, m, "n").Cursor(); got != 2 {
		t.Errorf("n from the top went to %d, want 2", got)
	}
}

// Backspace on an empty line closes the input, as in the pager; on a line
// with text it erases.
func TestSearchBackspaceOnEmptyLineCloses(t *testing.T) {
	for _, k := range []string{"backspace", "ctrl+h"} {
		m := keys(t, load(t, searchSource()), "/")
		m = keys(t, m, k)
		if m.Capturing() {
			t.Errorf("%s on an empty line left the input open", k)
		}
		m = keys(t, m, "/", "f", "o")
		m = keys(t, m, k)
		if !m.Capturing() || m.input.Value() != "f" {
			t.Errorf("%s with text: open %v, value %q; want it erased to f", k, m.Capturing(), m.input.Value())
		}
	}
}

// The matches show in the view: the current one in its own style, and the
// others in another, over plain lines and highlighted ones alike.
func TestSearchHighlightsMatches(t *testing.T) {
	m := search1(t, load(t, searchSource()), "foo")
	st := m.Styles()
	cur, other := wrapOf(st.CurrentMatch), wrapOf(st.Match)
	v := m.View()
	if !strings.Contains(v, cur.on("Foo")) {
		t.Errorf("the current match is not in its style:\n%q", v)
	}
	if !strings.Contains(v, other.on("foo")) {
		t.Errorf("the other matches are not in their style:\n%q", v)
	}
	if n := strings.Count(v, cur.pre); n != 1 {
		t.Errorf("%d current matches shown, want 1", n)
	}
	// n moves the current style on.
	m = keys(t, m, "n")
	v = m.View()
	if !strings.Contains(v, other.on("Foo")) || !strings.Contains(v, cur.on("foo")) {
		t.Errorf("the current style did not move:\n%q", v)
	}
	assertFits(t, v, 80, 12)
	// Without the search, nothing is highlighted.
	m = keys(t, m, "esc")
	if strings.Contains(m.View(), other.pre) {
		t.Error("a cleared search is still highlighted")
	}
	// The text under the highlights is the text of the lines.
	on := search1(t, load(t, searchSource()), "foo")
	off := keys(t, on, "esc")
	if got, want := bodyOf(ansi.Strip(on.View())), bodyOf(ansi.Strip(off.View())); got != want {
		t.Errorf("highlighting changed the text:\n%s\nwant\n%s", got, want)
	}
}

// bodyOf is the rows of a view, without its status line.
func bodyOf(s string) string {
	ls := strings.Split(s, "\n")
	return strings.Join(ls[:len(ls)-1], "\n")
}

// A match far to the right is scrolled into view.
func TestSearchScrollsToTheMatch(t *testing.T) {
	long := strings.Repeat("0123456789", 12)
	src := &source{size: 5, files: []File{fileOf("a.go", "@@ -1 +1 @@\n-x\n+"+long+"needle")}}
	m := search1(t, load(t, src, WithSize(40, 8)), "needle")
	if m.left == 0 {
		t.Fatal("the view did not scroll sideways")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "needle") {
		t.Errorf("the match is not in view:\n%s", v)
	}
	assertFits(t, m.View(), 40, 8)
	// A match in view leaves the scroll as it is.
	m2 := search1(t, load(t, searchSource(), WithSize(40, 8)), "foo")
	if m2.left != 0 {
		t.Errorf("left = %d for a match in view", m2.left)
	}
}

// Highlights and the status fit every size.
func TestSearchViewFitsAnySize(t *testing.T) {
	m := search1(t, load(t, searchSource()), "foo")
	for w := 1; w <= 60; w += 7 {
		for _, h := range []int{1, 2, 3, 12} {
			m.SetSize(w, h)
			for _, k := range []string{"n", "N", "right", "J", "/"} {
				m = keys(t, m, k)
				assertFits(t, m.View(), w, h)
			}
			m = keys(t, m, "esc", "esc")
			m = search1(t, m, "foo")
		}
	}
}

func TestViewSearch(t *testing.T) {
	src := &source{size: 5, files: []File{
		fileOf("main.go", "@@ -1,4 +1,4 @@ package main\n import \"fmt\"\n-func hello(name string) {\n+func hello(who string) {\n \tfmt.Println(\"hello\", who)\n }"),
		fileOf("notes.txt", "@@ -1,2 +1,2 @@\n hello there\n-hello world\n+Hello world"),
	}}
	syntaxtest.Use(t, syntaxtest.Stopped{})
	m := load(t, src, WithSize(60, 14), WithStyles(tintedStyles()))
	m = search1(t, m, "hello")
	m = keys(t, m, "n")
	v := m.View()
	assertFits(t, v, 60, 14)
	golden.RequireEqual(t, v)
}

func TestViewSearchStatus(t *testing.T) {
	for _, tt := range []struct {
		name string
		do   func(t *testing.T) Model
	}{
		{"prompt", func(t *testing.T) Model {
			t.Helper()
			return keys(t, load(t, searchSource(), WithSize(50, 8), WithStyles(asciiStyles())), "/", "f", "o")
		}},
		{"match", func(t *testing.T) Model {
			t.Helper()
			return keys(t, search1(t, load(t, searchSource(), WithSize(50, 8), WithStyles(asciiStyles())), "foo"), "n")
		}},
		{"none", func(t *testing.T) Model {
			t.Helper()
			return search1(t, load(t, searchSource(), WithSize(50, 8), WithStyles(asciiStyles())), "zzz")
		}},
		{"more pages", func(t *testing.T) Model {
			t.Helper()
			src := &source{size: 1, files: searchSource().files}
			m := load(t, src, WithSize(50, 8), WithStyles(asciiStyles()))
			// Without the next page, as before it arrives.
			m.pg.done = false
			m.search = search{query: "foo", matches: []match{{0, 1, 0, 3}, {0, 2, 0, 3}}, cur: 1}
			return m
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.do(t)
			v := ansi.Strip(m.View())
			assertFits(t, v, 50, 8)
			if strings.ContainsFunc(v, func(r rune) bool { return r > 127 }) {
				t.Errorf("glyphs beyond ASCII:\n%s", v)
			}
			golden.RequireEqual(t, v)
		})
	}
}

// BenchmarkSearch runs a search over 3000 files of 33 lines each, about
// 100k lines, with the files not parsed yet, so it parses them all: for a
// query in the lines of every file, and for one in none.
func BenchmarkSearch(b *testing.B) {
	for _, tt := range []struct {
		name, query string
		want        int
	}{{"common", "new b3", 3000}, {"rare", "no such text", 0}} {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				m := benchView(b)
				b.StartTimer()
				m.runSearch(tt.query)
				if m.Matches() != tt.want {
					b.Fatalf("Matches = %d, want %d", m.Matches(), tt.want)
				}
			}
		})
	}
}
