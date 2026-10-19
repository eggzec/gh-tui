package diff

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// note is a block of the text under the line at side and line of path. It
// draws a box whose header names it, and folds to one line. With ascii it
// draws no glyph beyond ASCII.
func note(id, path string, side Side, line int, text string, ascii ...bool) Block {
	top, bar, end, shut := "┌ ", "│ ", "└", "▸ "
	if len(ascii) > 0 && ascii[0] {
		top, bar, end, shut = "+ ", "| ", "'", "> "
	}
	return Block{
		ID:     id,
		Anchor: Pos{Path: path, Side: side, Line: line},
		Render: func(width int, folded bool) []string {
			if folded {
				return []string{ansi.Truncate(shut+id+" (folded)", width, "")}
			}
			out := []string{top + id}
			for l := range strings.SplitSeq(ansi.Wrap(text, max(width-len(bar), 1), ""), "\n") {
				out = append(out, bar+l)
			}
			return append(out, end)
		},
	}
}

var kindName = map[Kind]string{KindContext: "context", KindAdded: "added", KindDeleted: "deleted", KindBlock: "block"}

// at is the one-line block under new line n of path.
func at(id, path string, n int) Block { return note(id, path, NewSide, n, "text of "+id) }

// blockFiles is two files: a.go has the lines 1, 2 and 3 in a hunk, and 21,
// 22 and 23 in another (row 0 is its header, 1 the first hunk, 2 "ctx a0",
// 3 "-old b0", 4 "+new b0", 5 "ctx c0", 6 the second hunk, 7 "ctx a1", 8
// "-old b1", 9 "+new b1" and 10 "ctx c1"), then b.go, one hunk.
func blockFiles() *source {
	return &source{size: 5, files: []File{goFile("a.go", 2), goFile("b.go", 1)}}
}

// settle runs what a model that was changed from outside asks for.
func settle(tb testing.TB, m Model) Model {
	tb.Helper()
	m, cmd := m.Update(nil)
	return run(tb, m, cmd)
}

// withBlocks loads src at w by h and sets blocks.
func withBlocks(tb testing.TB, src *source, w, h int, blocks ...Block) Model {
	tb.Helper()
	m := load(tb, src, WithSize(w, h))
	m.SetBlocks(blocks)
	return settle(tb, m)
}

// rowsOf renders the text of every row of m.
func rowTexts(m Model) []string {
	out := make([]string, 0, m.Len())
	for i := range m.Len() {
		r, _ := m.layout.RowAt(i)
		out = append(out, fmt.Sprintf("%d:%s", r.Kind, r.Text))
	}
	return out
}

func TestViewBlocks(t *testing.T) {
	tests := []struct {
		name  string
		model func(t *testing.T) Model
		keys  []string
	}{
		{"unfolded", func(t *testing.T) Model {
			t.Helper()
			return withBlocks(t, blockFiles(), 60, 16,
				note("n1", "a.go", NewSide, 2, "this line changed, and a long remark about why"),
				note("n2", "a.go", OldSide, 2, "why was this here?"))
		}, nil},
		{"folded", func(t *testing.T) Model {
			t.Helper()
			folded := note("n1", "a.go", NewSide, 2, "this line changed, and a long remark about why")
			folded.Folded = true
			return withBlocks(t, blockFiles(), 40, 14, folded, note("n2", "a.go", OldSide, 2, "open"))
		}, nil},
		{"stacked", func(t *testing.T) Model {
			t.Helper()
			return withBlocks(t, blockFiles(), 60, 16,
				note("first", "a.go", NewSide, 2, "one"),
				note("second", "a.go", NewSide, 2, "two"),
				note("on-context", "a.go", OldSide, 1, "a context line takes both sides"))
		}, nil},
		{"file_level", func(t *testing.T) Model {
			t.Helper()
			return withBlocks(t, blockFiles(), 60, 14,
				note("file-note", "a.go", NewSide, 0, "about the whole file"),
				at("on-line", "a.go", 3))
		}, nil},
		{"unplaced", func(t *testing.T) Model {
			t.Helper()
			return withBlocks(t, blockFiles(), 60, 14,
				note("gone", "a.go", NewSide, 99, "the line it was on is not shown"),
				at("here", "a.go", 22))
		}, nil},
		{"collapsed", func(t *testing.T) Model {
			t.Helper()
			src := &source{size: 5, files: []File{goFile("small.go", 1), goFile("big.go", 4), goFile("tail.go", 1)}}
			m := load(t, src, WithSize(60, 12), WithCollapseOver(10))
			m.SetBlocks([]Block{at("hidden", "big.go", 1), at("shown", "small.go", 2), at("last", "tail.go", 3)})
			return settle(t, m)
		}, nil},
		{"narrow", func(t *testing.T) Model {
			t.Helper()
			return withBlocks(t, blockFiles(), 24, 16,
				note("n1", "a.go", NewSide, 2, "a long remark that wraps in a narrow view"))
		}, nil},
		{"ascii", func(t *testing.T) Model {
			t.Helper()
			m := load(t, blockFiles(), WithSize(50, 14), WithStyles(asciiStyles()))
			folded := note("n2", "a.go", OldSide, 2, "closed", true)
			folded.Folded = true
			m.SetBlocks([]Block{note("n1", "a.go", NewSide, 2, "open box", true), folded})
			return settle(t, m)
		}, nil},
		{"cursor", func(t *testing.T) Model {
			t.Helper()
			return withBlocks(t, blockFiles(), 60, 14,
				note("n1", "a.go", NewSide, 2, "one\ntwo"))
		}, []string{"down", "down", "down", "down", "down", "down"}},
		{"search", func(t *testing.T) Model {
			t.Helper()
			return withBlocks(t, blockFiles(), 60, 14,
				note("n1", "a.go", NewSide, 2, "mentions NEEDLE twice: needle"),
				note("n2", "b.go", NewSide, 1, "no match here"))
		}, []string{"/", "n", "e", "e", "d", "l", "e", "enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model(t)
			m = keys(t, m, tt.keys...)
			v := m.View()
			assertFits(t, v, m.Width(), m.Height())
			if tt.name == "ascii" && strings.ContainsFunc(ansi.Strip(v), func(r rune) bool { return r > unicode.MaxASCII }) {
				t.Errorf("view has glyphs beyond ASCII:\n%s", ansi.Strip(v))
			}
			golden.RequireEqual(t, ansi.Strip(v))
		})
	}
}

// The rows of a block come right after the row of its line, a context line
// is both sides, and the line numbers stay blank.
func TestBlockPlacement(t *testing.T) {
	m := withBlocks(t, blockFiles(), 80, 30,
		at("new", "a.go", 2),                      // after "+new b0"
		note("old", "a.go", OldSide, 2, "d"),      // after "-old b0"
		at("ctx-new", "a.go", 3),                  // after "ctx c0"
		note("ctx-old", "a.go", OldSide, 21, "d"), // after "ctx a1"
		at("top", "a.go", 0),                      // under the header
	)
	rows := rowTexts(m)
	var order []string
	for i := range m.Len() {
		if b, ok := m.layout.BlockAt(i); ok {
			if len(order) == 0 || order[len(order)-1] != b.ID {
				order = append(order, b.ID)
			}
			continue
		}
		r, _ := m.layout.RowAt(i)
		order = append(order, r.Text)
	}
	want := []string{
		"a.go", "top", "@@ -1,3 +1,3 @@ func f0() {", "ctx a0", "old b0", "old", "new b0", "new", "ctx c0", "ctx-new",
		"@@ -21,3 +21,3 @@ func f1() {", "ctx a1", "ctx-old", "old b1", "new b1", "ctx c1", "b.go",
	}
	if !slices.Equal(order[:len(want)], want) {
		t.Fatalf("rows are\n%q\nwant\n%q\nall:\n%q", order, want, rows)
	}
	for i := range m.Len() {
		if _, ok := m.layout.BlockAt(i); ok {
			if _, has := m.layout.PosAt(i); has {
				t.Errorf("block row %d has a position", i)
			}
		}
	}
	// A block row stands under its file, and a line is found past the blocks
	// above it.
	row, found := m.layout.Find(Pos{Path: "a.go", Side: NewSide, Line: 22})
	if r, _ := m.layout.RowAt(row); !found || r.Text != "new b1" {
		t.Errorf("Find new 22 = row %d (%q), %v, want new b1", row, r.Text, found)
	}
	v := ansi.Strip(m.View())
	for line := range strings.SplitSeq(v, "\n") {
		if strings.Contains(line, "│ text of new") && strings.TrimSpace(line[2:10]) != "" {
			t.Errorf("a block row has a line number: %q", line)
		}
	}
}

func TestUnplacedBlocks(t *testing.T) {
	m := withBlocks(t, blockFiles(), 60, 20,
		note("a", "a.go", NewSide, 99, "x"),
		note("b", "a.go", NewSide, 0, "x"),
		note("c", "b.go", OldSide, 3, "x"), // old 3 is a context line
		note("d", "b.go", OldSide, 77, "x"),
		note("e", "elsewhere.go", NewSide, 1, "x"), // no such file
	)
	if got := m.HeaderBlocks(); !slices.Equal(got, []string{"a", "b", "d"}) {
		t.Errorf("HeaderBlocks = %q, want a, b and d", got)
	}
	// Both of the a.go ones sit under the header, in the order given.
	if i, ok := m.layout.BlockRow("a"); !ok || i != 1 {
		t.Errorf("a is on row %d, %v, want row 1", i, ok)
	}
	if i, ok := m.layout.BlockRow("b"); !ok || i <= 1 {
		t.Errorf("b is on row %d, %v, want it after a", i, ok)
	}
	if _, ok := m.layout.BlockRow("e"); ok {
		t.Error("a block of a file that is not there has a row")
	}
}

// onlyAt is the one-line block under new line n of path that shows only while
// the line does.
func onlyAt(id, path string, n int) Block {
	b := at(id, path, n)
	b.OnlyAtLine = true
	return b
}

// oldOnlyAt is onlyAt on the old side.
func oldOnlyAt(id, path string, n int) Block {
	b := note(id, path, OldSide, n, "text of "+id)
	b.OnlyAtLine = true
	return b
}

// A block that is only at its line draws no row when the line is not shown,
// and does not move the rows after it.
func TestOnlyAtLineBlocks(t *testing.T) {
	base := load(t, blockFiles(), WithSize(60, 20)).Len()
	kept := withBlocks(t, blockFiles(), 60, 20, onlyAt("kept", "a.go", 2))
	if kept.Len() <= base {
		t.Fatalf("a block at a shown line has %d rows in all, want more than %d", kept.Len(), base)
	}
	if _, ok := kept.layout.BlockRow("kept"); !ok {
		t.Error("the block at a shown line has no row")
	}

	m := withBlocks(t, blockFiles(), 60, 20,
		onlyAt("gone", "a.go", 99),
		onlyAt("zero", "a.go", 0),
		oldOnlyAt("old", "b.go", 77),
		at("under", "a.go", 98),
		onlyAt("kept", "a.go", 2),
	)
	for _, id := range []string{"gone", "zero", "old"} {
		if i, ok := m.layout.BlockRow(id); ok {
			t.Errorf("block %s is on row %d, want none", id, i)
		}
	}
	if got := m.HeaderBlocks(); !slices.Equal(got, []string{"under"}) {
		t.Errorf("HeaderBlocks = %q, want under", got)
	}
	if i, ok := m.layout.BlockRow("kept"); !ok || i <= 1 {
		t.Errorf("kept is on row %d, %v, want a row of its line", i, ok)
	}
	// Only "under" and "kept" take rows: one line each.
	only := withBlocks(t, blockFiles(), 60, 20, at("under", "a.go", 98), onlyAt("kept", "a.go", 2))
	if m.Len() != only.Len() {
		t.Errorf("%d rows, want the %d of the blocks that show", m.Len(), only.Len())
	}

	// Blocks given again, with the line now there, come back.
	m.SetBlocks([]Block{onlyAt("kept", "a.go", 2)})
	m = settle(t, m)
	if m.Len() != kept.Len() {
		t.Errorf("%d rows after the other blocks went, want %d", m.Len(), kept.Len())
	}
}

// A block that is only at its line waits for its file like any other.
func TestOnlyAtLineBlockWaitsForItsFile(t *testing.T) {
	src := &source{size: 1, files: []File{goFile("a.go", 8), goFile("b.go", 8), goFile("c.go", 8), goFile("d.go", 8)}}
	m := load(t, src, WithSize(60, 12))
	m.SetBlocks([]Block{onlyAt("late", "d.go", 2), onlyAt("never", "d.go", 99)})
	m = settle(t, m)
	for range 8 {
		m = keys(t, m, "G")
	}
	if m.Files() != 4 {
		t.Fatalf("%d files fetched, want 4", m.Files())
	}
	if _, ok := m.layout.BlockRow("late"); !ok {
		t.Error("the block did not find its line")
	}
	if i, ok := m.layout.BlockRow("never"); ok {
		t.Errorf("the block of a line that is not there is on row %d", i)
	}
	if got := m.HeaderBlocks(); len(got) != 0 {
		t.Errorf("HeaderBlocks = %q", got)
	}
}

// A block waits for its file, and takes its place when the page it is in
// arrives.
func TestBlocksWaitForTheirFile(t *testing.T) {
	// Files taller than the view, so that only the first pages are asked for.
	src := &source{size: 1, files: []File{goFile("a.go", 8), goFile("b.go", 8), goFile("c.go", 8), goFile("d.go", 8)}}
	m := load(t, src, WithSize(60, 12))
	m.SetBlocks([]Block{at("late", "d.go", 2)})
	m = settle(t, m)
	// The block's file is on a page that has not arrived, so the test starts
	// from the block waiting.
	if m.Files() >= 4 {
		t.Fatalf("%d files fetched before the last page, want fewer than 4", m.Files())
	}
	if _, ok := m.layout.BlockRow("late"); ok {
		t.Fatal("the block has a row before its file is there")
	}
	for range 8 {
		m = keys(t, m, "G")
	}
	if m.Files() != 4 {
		t.Fatalf("%d files fetched, want 4", m.Files())
	}
	if _, ok := m.layout.BlockRow("late"); !ok {
		t.Error("the block did not find its file")
	}
	if got := m.HeaderBlocks(); len(got) != 0 {
		t.Errorf("HeaderBlocks = %q", got)
	}
}

// The fold state of an ID outlives SetBlocks and a resize, and a block given
// folded stays so until it is unfolded.
func TestBlockFoldKept(t *testing.T) {
	long := "words that take several lines in a narrow view of the diff"
	mk := func(folded bool) []Block {
		a := note("a", "a.go", NewSide, 2, long)
		a.Folded = folded
		return []Block{a, note("b", "a.go", NewSide, 3, "short")}
	}
	m := withBlocks(t, blockFiles(), 60, 20, mk(true)...)
	if !m.BlockFolded("a") || m.BlockFolded("b") {
		t.Fatalf("fold states %v %v, want a folded and b open", m.BlockFolded("a"), m.BlockFolded("b"))
	}
	if !m.SetBlockFolded("a", false) || !m.SetBlockFolded("b", true) {
		t.Fatal("folding did not change")
	}
	if m.SetBlockFolded("a", false) || m.SetBlockFolded("nope", true) {
		t.Error("a fold that changes nothing says it did")
	}
	// The same blocks again: the folds the user made win over those given.
	m.SetBlocks(mk(true))
	if m.BlockFolded("a") || !m.BlockFolded("b") {
		t.Errorf("after SetBlocks: a folded %v, b folded %v, want the user's folds", m.BlockFolded("a"), m.BlockFolded("b"))
	}
	before := m.Len()
	for _, w := range []int{20, 90, 60} {
		m.SetSize(w, 20)
		m = settle(t, m)
		if m.BlockFolded("a") || !m.BlockFolded("b") {
			t.Errorf("at width %d: a folded %v, b folded %v", w, m.BlockFolded("a"), m.BlockFolded("b"))
		}
		assertFits(t, m.View(), w, 20)
	}
	if m.Len() != before {
		t.Errorf("rows %d after the resizes, want %d as before", m.Len(), before)
	}
	// A block that leaves forgets its state, and a new one starts as given.
	m.SetBlocks(nil)
	m.SetBlocks(mk(true))
	if !m.BlockFolded("a") {
		t.Error("a block that was gone and is back keeps a fold state")
	}
}

// Narrow views draw the blocks again at the new width.
func TestBlocksFollowTheWidth(t *testing.T) {
	m := withBlocks(t, blockFiles(), 80, 30, note("n", "a.go", NewSide, 2, strings.Repeat("word ", 12)))
	wide := m.Len()
	m.SetSize(30, 30)
	m = settle(t, m)
	if m.Len() <= wide {
		t.Errorf("a narrow view has %d rows, want more than the %d of a wide one", m.Len(), wide)
	}
	for i := range m.Len() {
		if _, ok := m.layout.BlockAt(i); !ok {
			continue
		}
		if v := m.layout.rowIn(0, i-0-1); ansi.StringWidth(v.Text) > m.blockWidth() {
			t.Errorf("row %d is %d cells, wider than %d", i, ansi.StringWidth(v.Text), m.blockWidth())
		}
	}
	m.SetSize(80, 30)
	m = settle(t, m)
	if m.Len() != wide {
		t.Errorf("back at 80 there are %d rows, want %d", m.Len(), wide)
	}
}

// The cursor stays on its row when rows above it come and go.
func TestCursorStaysOnItsRow(t *testing.T) {
	m := withBlocks(t, blockFiles(), 60, 30)
	m = keys(t, m, "J") // b.go
	m = keys(t, m, "down", "down")
	row := func(m Model) string { r, _ := m.layout.RowAt(m.Cursor()); return r.Text }
	if row(m) != "ctx a0" || m.Cursor() != 13 {
		t.Fatalf("setup: on %q at %d", row(m), m.Cursor())
	}
	m.SetBlocks([]Block{at("above", "a.go", 2), at("here", "b.go", 1)})
	if row(m) != "ctx a0" {
		t.Errorf("after blocks above: on %q (row %d), want ctx a0", row(m), m.Cursor())
	}
	m.SetBlocks([]Block{at("above", "a.go", 2), note("tall", "a.go", NewSide, 3, strings.Repeat("word ", 40))})
	m.SetSize(30, 30)
	if row(m) != "ctx a0" {
		t.Errorf("after a resize: on %q (row %d), want ctx a0", row(m), m.Cursor())
	}
	m.SetBlocks(nil)
	if row(m) != "ctx a0" {
		t.Errorf("after the blocks left: on %q (row %d), want ctx a0", row(m), m.Cursor())
	}
	// On a block that goes, it goes to the line the block was under.
	m = keys(t, m, "g")
	m.SetBlocks([]Block{at("b", "a.go", 2)})
	m = keys(t, m, "down", "down", "down", "down", "down")
	if b, ok := m.CursorBlock(); !ok || b.ID != "b" {
		t.Fatalf("setup: cursor on %v, %v", b.ID, ok)
	}
	m.SetBlocks(nil)
	if row(m) != "new b0" {
		t.Errorf("a block gone under the cursor leaves it on %q, want new b0", row(m))
	}
}

func TestEnterFoldsABlockOrAFile(t *testing.T) {
	m := withBlocks(t, blockFiles(), 60, 20, at("n", "a.go", 2))
	m = keys(t, m, "down", "down", "down", "down", "down") // "+new b0" is 4; the block starts on 5
	b, ok := m.CursorBlock()
	if !ok || b.ID != "n" {
		t.Fatalf("cursor is on %v, %v, want the block", b.ID, ok)
	}
	rows := m.Len()
	m = keys(t, m, "enter")
	if !m.BlockFolded("n") || m.Len() >= rows {
		t.Errorf("enter on a block: folded %v, rows %d (was %d)", m.BlockFolded("n"), m.Len(), rows)
	}
	if b, ok := m.CursorBlock(); !ok || b.ID != "n" {
		t.Error("the cursor left the folded block")
	}
	m = keys(t, m, "enter")
	if m.BlockFolded("n") || m.Len() != rows {
		t.Errorf("enter again: folded %v, rows %d, want it open with %d", m.BlockFolded("n"), m.Len(), rows)
	}
	// Inside an open block, folding goes to its first row.
	m = keys(t, m, "down", "down")
	m = keys(t, m, "enter")
	if r, _ := m.layout.RowAt(m.Cursor()); r.Kind != KindBlock || !strings.HasPrefix(r.Text, "▸") {
		t.Errorf("folding from the third row left the cursor on %q", r.Text)
	}
	// On a header it folds the file, with its blocks.
	m = keys(t, m, "g", "enter")
	if !m.layout.Collapsed(0) {
		t.Error("enter on a header did not fold the file")
	}
	if _, ok := m.layout.BlockRow("n"); ok {
		t.Error("a collapsed file shows its block")
	}
	if got := m.layout.rowsOf(0); got != 1 {
		t.Errorf("a collapsed file has %d rows, want 1", got)
	}
	m = keys(t, m, "enter")
	if m.layout.Collapsed(0) {
		t.Error("enter on the header did not unfold the file")
	}
}

func TestSeekBlock(t *testing.T) {
	src := &source{size: 1, files: []File{goFile("a.go", 2), goFile("b.go", 1), goFile("c.go", 1)}}
	m := load(t, src, WithSize(60, 4))
	m.SetBlocks([]Block{at("x", "a.go", 22), at("y", "c.go", 2), at("nofile", "z.go", 1)})
	m = settle(t, m)
	if m.SeekBlock("missing") {
		t.Error("SeekBlock found a block that is not there")
	}
	// A collapsed file opens when it is seeked.
	m.layout.SetCollapsed(0, true)
	if m.Files() != 1 {
		t.Fatalf("setup: %d files fetched, want 1", m.Files())
	}
	if !m.SeekBlock("x") || m.layout.Collapsed(0) {
		t.Fatal("SeekBlock did not open the file and find x")
	}
	if b, ok := m.CursorBlock(); !ok || b.ID != "x" {
		t.Errorf("cursor on %v, %v, want x", b.ID, ok)
	}
	// y is in a file that is not fetched yet: the view fetches until it is.
	if m.SeekBlock("y") {
		t.Error("SeekBlock says y is shown before its file came")
	}
	m = settle(t, m)
	if b, ok := m.CursorBlock(); !ok || b.ID != "y" {
		t.Errorf("after the pages arrived the cursor is on %v, %v, want y", b.ID, ok)
	}
	// z.go never arrives: the seek ends with the last page, without a cursor
	// move.
	here := m.Cursor()
	m.SeekBlock("nofile")
	m = settle(t, m)
	if m.Cursor() != here {
		t.Errorf("the cursor moved to %d for a block of no file", m.Cursor())
	}
}

func TestNextAndPrevBlock(t *testing.T) {
	src := &source{size: 5, files: []File{goFile("a.go", 2), goFile("b.go", 1), goFile("c.go", 1)}}
	m := load(t, src, WithSize(60, 12), WithCollapseOver(5))
	m.SetBlocks([]Block{
		at("a1", "a.go", 2), at("a2", "a.go", 22), at("b1", "b.go", 0),
		note("c1", "c.go", NewSide, 3, "closed"),
	})
	m = settle(t, m)
	all := func(Block) bool { return true }
	only := func(ids ...string) func(Block) bool {
		return func(b Block) bool { return slices.Contains(ids, b.ID) }
	}
	cursor := func() string {
		if b, ok := m.CursorBlock(); ok {
			return b.ID
		}
		r, _ := m.layout.RowAt(m.Cursor())
		return "row:" + r.Text
	}
	var got []string
	for m.NextBlock(all) {
		got = append(got, cursor())
	}
	if want := []string{"a1", "a2", "b1", "c1"}; !slices.Equal(got, want) {
		t.Errorf("NextBlock visits %q, want %q", got, want)
	}
	got = nil
	for m.PrevBlock(all) {
		got = append(got, cursor())
	}
	if want := []string{"b1", "a2", "a1"}; !slices.Equal(got, want) {
		t.Errorf("PrevBlock visits %q, want %q", got, want)
	}
	// The predicate picks; on a miss the cursor stays.
	m = keys(t, m, "g")
	if !m.NextBlock(only("a2", "c1")) || cursor() != "a2" {
		t.Errorf("next of a2 or c1 went to %q", cursor())
	}
	if !m.NextBlock(only("a2", "c1")) || cursor() != "c1" {
		t.Errorf("next after it went to %q", cursor())
	}
	here := m.Cursor()
	if m.NextBlock(only("a2", "c1")) || m.Cursor() != here {
		t.Error("NextBlock past the last match moved")
	}
	if m.NextBlock(only("nothing")) || m.PrevBlock(only("nothing")) || m.Cursor() != here {
		t.Error("a predicate that accepts nothing moved the cursor")
	}
	// From the middle of a tall block, the previous block is the one before it.
	m.SetBlocks([]Block{at("p", "a.go", 2), note("tall", "a.go", NewSide, 3, strings.Repeat("word ", 30))})
	m.SeekBlock("tall")
	m = keys(t, m, "down", "down")
	if !m.PrevBlock(all) || cursor() != "p" {
		t.Errorf("PrevBlock from inside a block went to %q, want p", cursor())
	}
	m.SeekBlock("tall")
	m = keys(t, m, "down")
	if m.NextBlock(all) {
		t.Errorf("NextBlock from inside the last block went to %q", cursor())
	}
}

// A search reads the text of the blocks, in the order of the rows, and a
// fold takes the text it hides out of it.
func TestSearchBlocks(t *testing.T) {
	src := &source{size: 5, files: []File{
		fileOf("a.go", "@@ -1,2 +1,2 @@\n keep foo\n-old\n+new foo"),
		fileOf("b.go", "@@ -1 +1 @@\n-x\n+foo again"),
	}}
	m := load(t, src, WithSize(60, 20))
	m.SetBlocks([]Block{
		note("n1", "a.go", NewSide, 1, "foo in a block"),
		note("n2", "b.go", NewSide, 1, "\x1b[1mFOO\x1b[m styled, and foo"),
	})
	m = settle(t, m)
	m = search1(t, m, "foo")
	// Lines: keep foo, new foo, n1's text, b.go's line, n2's two matches (the
	// capital one too, as the query has none).
	if got := m.Matches(); got != 6 {
		t.Fatalf("Matches = %d, want 6", got)
	}
	visited := make([]string, 0, 6)
	for range 6 {
		r, _ := m.layout.RowAt(m.Cursor())
		visited = append(visited, kindName[r.Kind]+" "+ansi.Strip(r.Text))
		m = keys(t, m, "n")
	}
	want := []string{
		"context keep foo", "block │ foo in a block", "added new foo", "added foo again",
		"block │ FOO styled, and foo", "block │ FOO styled, and foo",
	}
	if !slices.Equal(visited, want) {
		t.Errorf("n visits\n%q\nwant\n%q", visited, want)
	}
	// A match in a block of a collapsed file opens the file.
	m.layout.SetCollapsed(0, true)
	m = keys(t, m, "g", "n")
	if m.layout.Collapsed(0) {
		t.Error("jumping to a match in a collapsed file left it collapsed")
	}
	// Folding the block takes its text out of the matches.
	m = keys(t, search1(t, m, "foo"), "n")
	if b, ok := m.CursorBlock(); !ok || b.ID != "n1" {
		t.Fatalf("cursor on %v, %v, want n1", b.ID, ok)
	}
	m = keys(t, m, "enter")
	if got := m.Matches(); got != 5 {
		t.Errorf("after folding n1: %d matches, want 5 (its text is hidden)", got)
	}
	m = keys(t, m, "enter")
	if got := m.Matches(); got != 6 {
		t.Errorf("after unfolding n1: %d matches, want 6", got)
	}
	// Blocks that come after a search shown are read too.
	m.SetBlocks([]Block{note("n3", "b.go", NewSide, 1, "foo foo foo")})
	if got := m.Matches(); got != 6 {
		t.Errorf("after SetBlocks: %d matches, want the 3 lines and 3 of n3", got)
	}
	m.SetBlocks(nil)
	if got := m.Matches(); got != 3 {
		t.Errorf("without blocks: %d matches, want 3", got)
	}
}

// A search keeps its current match when the blocks of other files change.
func TestSearchKeepsCurrentThroughBlockChanges(t *testing.T) {
	src := &source{size: 5, files: []File{
		fileOf("a.go", "@@ -1 +1 @@\n-x\n+foo a"),
		fileOf("b.go", "@@ -1 +1 @@\n-x\n+foo b"),
	}}
	m := load(t, src, WithSize(60, 20))
	m = search1(t, m, "foo")
	m = keys(t, m, "n") // b.go's
	if r, _ := m.layout.RowAt(m.Cursor()); r.Text != "foo b" {
		t.Fatalf("setup: on %q", r.Text)
	}
	m.SetBlocks([]Block{note("n", "a.go", NewSide, 1, "foo")})
	if r, _ := m.layout.RowAt(m.Cursor()); r.Text != "foo b" {
		t.Fatalf("the cursor moved to %q", r.Text)
	}
	if got := m.Matches(); got != 3 {
		t.Fatalf("Matches = %d, want 3", got)
	}
	// n goes on from b.go's match to the first one again, a.go's.
	m = keys(t, m, "n")
	if r, _ := m.layout.RowAt(m.Cursor()); r.Text != "foo a" {
		t.Errorf("n went to %q, want foo a", r.Text)
	}
	status, _ := m.searchStatus()
	if status != "match 1/3" {
		t.Errorf("status %q, want match 1/3", status)
	}
}

// Blocks that arrive above the cursor don't push its line down the window.
func TestSetBlocksKeepsCursorRow(t *testing.T) {
	m := load(t, blockFiles(), WithSize(60, 6))
	m.moveTo(8)
	row, _ := m.layout.RowAt(m.Cursor())
	below := m.cursor - m.top
	if below < 1 {
		t.Fatalf("setup: the cursor is at the top of the window (%d)", below)
	}
	m.SetBlocks([]Block{at("a", "a.go", 2), at("b", "a.go", 1), note("c", "a.go", NewSide, 0, "x")})
	if r, _ := m.layout.RowAt(m.Cursor()); r.Text != row.Text || r.Kind != row.Kind {
		t.Errorf("the cursor moved to %q, want %q", r.Text, row.Text)
	}
	if got := m.cursor - m.top; got != below {
		t.Errorf("the cursor is %d rows into the window, want %d", got, below)
	}
}

// The moves between hunks and files do not stop at blocks, and a block under
// a header does not hide the file's first hunk.
func TestMovesAcrossBlocks(t *testing.T) {
	m := withBlocks(t, blockFiles(), 60, 30,
		at("top-b", "b.go", 0), note("top-b2", "b.go", NewSide, 0, "x"),
		at("in-a", "a.go", 2), at("mid-a", "a.go", 22))
	text := func() string { r, _ := m.layout.RowAt(m.Cursor()); return r.Text }
	seen := make([]string, 0, 4)
	m = keys(t, m, "g")
	for range 4 {
		m = keys(t, m, "}")
		seen = append(seen, text())
	}
	want := []string{"@@ -1,3 +1,3 @@ func f0() {", "@@ -21,3 +21,3 @@ func f1() {", "@@ -1,3 +1,3 @@ func f0() {", "@@ -1,3 +1,3 @@ func f0() {"}
	if !slices.Equal(seen, want) {
		t.Errorf("} visits %q, want %q", seen, want)
	}
	if f, _ := m.layout.FileAt(m.Cursor()); f != 1 {
		t.Errorf("the third } is in file %d, want b.go", f)
	}
	m = keys(t, m, "{")
	if f, _ := m.layout.FileAt(m.Cursor()); f != 0 || text() != "@@ -21,3 +21,3 @@ func f1() {" {
		t.Errorf("{ from b.go's hunk went to %q in file %d", text(), f)
	}
	m = keys(t, m, "J")
	if text() != "b.go" {
		t.Errorf("J from a.go went to %q", text())
	}
	m = keys(t, m, "K", "K")
	if text() != "a.go" {
		t.Errorf("K went to %q, want a.go", text())
	}
	// The sticky header stands in for the file when the window starts below it.
	m = withBlocks(t, blockFiles(), 60, 8, at("n", "a.go", 2))
	m = keys(t, m, "ctrl+d", "ctrl+d")
	v := ansi.Strip(m.View())
	if first, _, _ := strings.Cut(v, "\n"); !strings.Contains(first, "a.go") && !strings.Contains(first, "b.go") {
		t.Errorf("the first row of the window is not a file header:\n%s", v)
	}
}

// A window that starts inside a block, or a file with blocks, still has its
// file's header stuck on its first row.
func TestStickyHeaderOverBlocks(t *testing.T) {
	m := withBlocks(t, blockFiles(), 60, 8,
		note("tall", "a.go", NewSide, 2, strings.Repeat("word ", 40)), at("later", "a.go", 22))
	m = keys(t, m, "down", "down", "down", "down", "down", "down", "down")
	if _, ok := m.CursorBlock(); !ok {
		t.Fatalf("setup: cursor on row %d is not in a block", m.Cursor())
	}
	for _, k := range []string{"", "down", "down", "ctrl+d", "ctrl+u"} {
		if k != "" {
			m = keys(t, m, k)
		}
		v := strings.Split(ansi.Strip(m.View()), "\n")
		top, _ := m.layout.RowAt(m.top)
		if top.Kind == KindFileHeader {
			continue
		}
		if !strings.Contains(v[0], "a.go") {
			t.Errorf("after %q the first row is %q, want the header of a.go\n%s", k, v[0], strings.Join(v, "\n"))
		}
	}
}

// The tokens of a line stay with it when blocks come before it.
func TestHighlightWithBlocks(t *testing.T) {
	syntaxtest.Use(t, syntaxtest.Stopped{})
	m := load(t, &source{size: 5, files: []File{fileOf("p.go", openComment)}}, WithSize(60, 20))
	m.SetBlocks([]Block{at("n", "p.go", 1)})
	m = settle(t, m)
	// header 0, hunk 1, "package p" 2, block rows 3 to 5, then the lines.
	for _, c := range []struct {
		row, body int
	}{{2, 1}, {6, 2}, {7, 3}, {8, 4}} {
		r, _ := m.layout.RowAt(c.row)
		got, want := m.rowSpans(r, c.row), spansOfRow(m, c.body)
		if len(got) == 0 || !slices.Equal(got, want) {
			t.Errorf("row %d (%q): spans %v, want those of body row %d, %v", c.row, r.Text, got, c.body, want)
		}
	}
	if r, _ := m.layout.RowAt(4); m.rowSpans(r, 4) != nil {
		t.Error("a block row has spans")
	}
	assertFits(t, m.View(), 60, 20)
}

func TestBlockRowsFitAnySize(t *testing.T) {
	m := withBlocks(t, blockFiles(), 80, 12,
		at("a", "a.go", 2), note("b", "a.go", OldSide, 2, strings.Repeat("long ", 40)), at("c", "b.go", 0))
	for w := 1; w <= 90; w += 7 {
		for _, h := range []int{1, 2, 3, 12} {
			m.SetSize(w, h)
			for _, k := range []string{"", "G", "J", "enter", "}", "right", "g", "down", "down", "down", "enter"} {
				if k != "" {
					m = keys(t, m, k)
				}
				assertFits(t, m.View(), w, h)
			}
		}
	}
}

// A block that renders nothing still has a row to fold it from, and one with
// a newline in a line is split in two.
func TestOddBlocks(t *testing.T) {
	empty := Block{ID: "empty", Anchor: Pos{Path: "a.go", Side: NewSide, Line: 2}}
	multi := Block{ID: "multi", Anchor: Pos{Path: "a.go", Side: NewSide, Line: 3},
		Render: func(int, bool) []string { return []string{"one\ntwo", "three"} }}
	m := withBlocks(t, blockFiles(), 60, 20, empty, multi)
	if i, ok := m.layout.BlockRow("empty"); !ok {
		t.Error("an empty block has no row")
	} else if r, _ := m.layout.RowAt(i); r.Text != "" {
		t.Errorf("an empty block shows %q", r.Text)
	}
	i, _ := m.layout.BlockRow("multi")
	var lines []string
	for ; ; i++ {
		r, _ := m.layout.RowAt(i)
		if r.Block != "multi" {
			break
		}
		lines = append(lines, r.Text)
	}
	if !slices.Equal(lines, []string{"one", "two", "three"}) {
		t.Errorf("multi shows %q", lines)
	}
	// Two blocks with one ID: the first wins.
	m.SetBlocks([]Block{at("same", "a.go", 2), note("same", "a.go", NewSide, 3, "second")})
	if i, _ := m.layout.BlockRow("same"); i != 5 {
		t.Errorf("same is on row %d, want 5, under new line 2", i)
	}
	assertFits(t, m.View(), 60, 20)
}
