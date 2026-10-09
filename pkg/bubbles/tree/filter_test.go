package tree

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// named matches the nodes whose name holds word.
func named(word string) func(Node) bool {
	return func(n Node) bool { return strings.Contains(n.Name, word) }
}

func TestTreeExpand(t *testing.T) {
	f := repo()
	m := load(t, f)
	before := selectedID(m)

	m = run(t, m, m.Expand("internal"))
	want := []string{"cmd", "docs", "internal", "internal/core", "internal/tui", "README.md", "go.mod"}
	if got := rowIDs(m); !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
	if got := selectedID(m); got != before {
		t.Errorf("Expand moved the cursor to %q, from %q", got, before)
	}
	if n := f.callCount("internal"); n != 1 {
		t.Errorf("internal was loaded %d times, want 1", n)
	}

	// Expanding what is open and loaded reads nothing.
	m = run(t, m, m.Expand("internal"))
	if n := f.callCount("internal"); n != 1 {
		t.Errorf("Expand again loaded it %d times, want 1", n)
	}

	// A leaf, an unknown ID and the root are left alone.
	for _, id := range []string{"go.mod", "nowhere", ""} {
		if cmd := m.Expand(id); cmd != nil {
			t.Errorf("Expand(%q) started a load", id)
		}
	}
	if got := rowIDs(m); !slices.Equal(got, want) {
		t.Errorf("rows after the no-ops = %v, want %v", got, want)
	}
}

// Expand retries a branch whose load failed, as the expand key does.
func TestTreeExpandRetries(t *testing.T) {
	f := repo()
	f.setFail("docs", errors.New("boom"))
	m := run(t, load(t, f), nil)
	m = run(t, m, m.Expand("docs"))
	if m.nodes["docs"].err == nil {
		t.Fatal("the load of docs didn't fail")
	}
	f.setFail("docs", nil)
	m = run(t, m, m.Expand("docs"))
	if m.nodes["docs"].err != nil || !m.nodes["docs"].loaded {
		t.Errorf("Expand didn't read docs again: err %v, loaded %v", m.nodes["docs"].err, m.nodes["docs"].loaded)
	}
}

func TestTreeLoad(t *testing.T) {
	f := repo()
	m := load(t, f)
	before := rowIDs(m)

	m = run(t, m, m.Load("internal"))
	if got := rowIDs(m); !slices.Equal(got, before) {
		t.Errorf("Load changed the rows to %v, want %v", got, before)
	}
	if n := f.callCount("internal"); n != 1 {
		t.Errorf("internal was loaded %d times, want 1", n)
	}
	if e := m.nodes["internal"]; e.expanded || !e.loaded {
		t.Errorf("internal: expanded %v, loaded %v; want collapsed and loaded", e.expanded, e.loaded)
	}

	// Loaded, it is read no more, and opening it costs no request.
	if cmd := m.Load("internal"); cmd != nil {
		t.Error("Load of a loaded branch started a load")
	}
	m = run(t, m, m.Expand("internal"))
	if n := f.callCount("internal"); n != 1 {
		t.Errorf("Expand after Load loaded it %d times, want 1", n)
	}
	for _, id := range []string{"go.mod", "nowhere", ""} {
		if cmd := m.Load(id); cmd != nil {
			t.Errorf("Load(%q) started a load", id)
		}
	}
}

func TestTreeFilter(t *testing.T) {
	f := repo()
	m := load(t, f)
	m = run(t, m, m.Expand("cmd"))
	m = run(t, m, m.Expand("internal"))
	m = run(t, m, m.Expand("internal/core"))
	open := rowIDs(m)

	m.SetFilter(named("pull"))
	want := []string{"internal", "internal/core", "internal/core/pull.go"}
	if got := rowIDs(m); !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	if got := selectedID(m); got != "internal" {
		t.Errorf("the cursor is on %q, want internal, the next row shown after cmd", got)
	}
	if !m.nodes["internal"].open() || !m.nodes["internal/core"].open() {
		t.Error("a branch with a match shows open")
	}
	// Only what is loaded counts: tui and gh-tui weren't opened.
	if shown, total := m.Filtered("internal"); shown != 1 || total != 2 {
		t.Errorf("Filtered(internal) = %d of %d, want 1 of 2", shown, total)
	}
	if shown, total := m.Filtered("cmd"); shown != 0 || total != 0 {
		t.Errorf("Filtered(cmd) = %d of %d, want 0 of 0", shown, total)
	}

	// The filter doesn't touch the folds: clearing it puts them back.
	m.SetFilter(nil)
	if got := rowIDs(m); !slices.Equal(got, open) {
		t.Errorf("rows after clearing = %v, want %v", got, open)
	}
	if shown, total := m.Filtered("internal"); shown != 2 || total != 2 {
		t.Errorf("Filtered without a filter = %d of %d, want 2 of 2", shown, total)
	}
}

// A branch the filter holds open can't be collapsed, and the keys that
// would collapse it move to the parent instead.
func TestTreeFilterHoldsBranchesOpen(t *testing.T) {
	m := load(t, repo())
	m = run(t, m, m.Expand("internal"))
	m = run(t, m, m.Expand("internal/core"))
	m = keys(t, m, "g", "j", "j")
	m.SetFilter(named("repo"))
	if got, want := rowIDs(m), []string{"internal", "internal/core", "internal/core/repo.go"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	// Enter on the held-open branch changes nothing.
	m = keys(t, m, "j", "enter")
	if got := rowIDs(m); len(got) != 3 {
		t.Errorf("enter collapsed a branch the filter holds open: %v", got)
	}
	m = keys(t, m, "h")
	if got := selectedID(m); got != "internal" {
		t.Errorf("h went to %q, want the parent internal", got)
	}
	// A leaf of the filter opens as always.
	m = keys(t, m, "j", "j")
	if got := selectedID(m); got != "internal/core/repo.go" {
		t.Fatalf("the cursor is on %q", got)
	}
}

// Branches whose children aren't loaded have nothing to match, so they
// show once Load has read them.
func TestTreeFilterLoadsBranches(t *testing.T) {
	m := load(t, repo())
	m.SetFilter(named("app"))
	if m.Len() != 0 {
		t.Fatalf("rows = %v, want none while nothing is loaded", rowIDs(m))
	}
	m = run(t, m, m.Load("internal"))
	if m.Len() != 0 {
		t.Fatalf("rows = %v, want none: internal's subdirectories aren't loaded yet", rowIDs(m))
	}
	m = run(t, m, m.Load("internal/tui"))
	want := []string{"internal", "internal/tui", "internal/tui/app.go"}
	if got := rowIDs(m); !slices.Equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestTreeFilterNoMatch(t *testing.T) {
	m := load(t, repo(), WithEmptyText("No file matches."))
	m = run(t, m, m.Expand("cmd"))
	m.SetFilter(named("zzz"))
	if m.Len() != 0 {
		t.Fatalf("rows = %v, want none", rowIDs(m))
	}
	if _, ok := m.Selected(); ok {
		t.Error("a selection with no rows")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No file matches.") {
		t.Errorf("view lacks the empty text:\n%s", v)
	}
	m.SetFilter(nil)
	if m.Len() != len(roots)+1 {
		t.Errorf("rows after clearing = %v", rowIDs(m))
	}
}

// The cursor stays on its row when it still matches, and moves to the next
// row shown, or else the last one before it, when it doesn't.
func TestTreeFilterCursor(t *testing.T) {
	m := load(t, repo())
	for _, id := range []string{"cmd", "cmd/gh-tui", "internal", "internal/core", "internal/tui"} {
		m = run(t, m, m.Expand(id))
	}
	m = keys(t, m, "G", "k")
	if got := selectedID(m); got != "README.md" {
		t.Fatalf("the cursor is on %q, want README.md", got)
	}
	m.SetFilter(named("."))
	if got := selectedID(m); got != "README.md" {
		t.Errorf("the cursor moved to %q, though README.md matches", got)
	}
	m.SetFilter(named("main"))
	if got := selectedID(m); got != "cmd/gh-tui/main.go" {
		t.Errorf("the cursor is on %q, want the last row before it: main.go", got)
	}
	assertVisible(t, m)
}

func TestViewFiltered(t *testing.T) {
	build := func(word string) Model {
		m := load(t, repo(), WithSize(40, 8))
		m = run(t, m, m.Expand("cmd"))
		m = run(t, m, m.Expand("cmd/gh-tui"))
		m = run(t, m, m.Load("internal"))
		m = run(t, m, m.Load("internal/core"))
		m = run(t, m, m.Load("internal/tui"))
		m.SetFilter(named(word))
		return m
	}
	for name, word := range map[string]string{"filtered": "go", "filtered_to_one": "app"} {
		t.Run(name, func(t *testing.T) {
			m := build(word)
			v := m.View()
			assertFits(t, v, m.Width(), m.Height())
			golden.RequireEqual(t, v)
		})
	}
}

func TestRename(t *testing.T) {
	m := load(t, repo(), WithSize(40, 8))
	m.Rename("docs", "documents")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "documents") || strings.Contains(v, "▸ docs") {
		t.Errorf("view after Rename:\n%s", v)
	}
	if n, _ := m.At(1); n.ID != "docs" || n.Name != "documents" {
		t.Errorf("node = %+v", n)
	}
	m.Rename("nowhere", "x")
}

// A detail leaves its name at least the cells WithMinName says, and is
// dropped beyond that.
func TestMinName(t *testing.T) {
	f := newFiles("a-rather-long-file-name.md")
	f.setDetail("a-rather-long-file-name.md", "12K")
	row := func(opts ...Option) string {
		m := load(t, f, append([]Option{WithSize(20, 2)}, opts...)...)
		return strings.TrimSpace(ansi.Strip(strings.Split(m.View(), "\n")[0]))
	}
	if got, want := row(), "a-rather-lo… 12K"; !strings.HasSuffix(got, want) {
		t.Errorf("default row = %q, want it to end with %q", got, want)
	}
	if got := row(WithMinName(16)); strings.Contains(got, "12K") {
		t.Errorf("row with a name of at least 16 cells = %q, want the detail dropped", got)
	}
}
