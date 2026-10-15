package diff

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSetCollapsedKeepsCursor(t *testing.T) {
	files := []File{goFile("a.go", 2), goFile("b.go", 2), goFile("c.go", 2)}
	tests := []struct {
		name string
		// start is the row of file c the cursor starts on, with -1 for the
		// header of file b and -2 for a line of file b.
		start     int
		wantFile  string
		wantExact bool // the cursor is still on the same row of its file
	}{
		{"in a later file", 3, "c.go", true},
		{"on the header", -1, "b.go", true},
		{"in the collapsing file", -2, "b.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := load(t, &source{size: 5, files: files}, WithSize(60, 30))
			switch tt.start {
			case -1:
				m.SeekFile("b.go")
			case -2:
				m.SeekFile("b.go")
				m = keys(t, m, "down", "down")
			default:
				m.SeekFile("c.go")
				m = keys(t, m, "down", "down", "down")
			}
			before, _ := m.Position()
			m.SetCollapsed("b.go", true)
			if !m.Collapsed("b.go") || m.Collapsed("a.go") {
				t.Fatalf("Collapsed: b.go %v, a.go %v", m.Collapsed("b.go"), m.Collapsed("a.go"))
			}
			if f, _ := m.CurrentFile(); f.Path != tt.wantFile {
				t.Fatalf("cursor in %s, want %s", f.Path, tt.wantFile)
			}
			if after, _ := m.Position(); tt.wantExact && after != before {
				t.Errorf("cursor moved from %+v to %+v", before, after)
			}
			m.SetCollapsed("b.go", false)
			if m.Collapsed("b.go") {
				t.Error("b.go still collapsed after expanding it")
			}
		})
	}
}

func TestSetCollapsedUnknownPath(t *testing.T) {
	m := load(t, &source{size: 5, files: sizedFiles(2, 1)})
	n, cursor := m.Len(), m.Cursor()
	m.SetCollapsed("nope.go", true)
	if m.Collapsed("nope.go") || m.Len() != n || m.Cursor() != cursor {
		t.Errorf("an unknown path changed the view: collapsed %v, len %d (was %d)", m.Collapsed("nope.go"), m.Len(), n)
	}
	m.SetCollapsed("pkg/f0.go", true)
	m.SetCollapsed("pkg/f0.go", true)
	if !m.Collapsed("pkg/f0.go") {
		t.Error("pkg/f0.go is not collapsed")
	}
}

func TestSetCollapsedMatchesFold(t *testing.T) {
	files := []File{goFile("a.go", 1), goFile("b.go", 3)}
	byKey := keys(t, load(t, &source{size: 5, files: files}), "J", "enter")
	byCall := load(t, &source{size: 5, files: files})
	byCall.SetCollapsed("b.go", true)
	if byKey.Collapsed("b.go") != byCall.Collapsed("b.go") || byKey.Len() != byCall.Len() {
		t.Errorf("by key: collapsed %v, %d rows; by call: collapsed %v, %d rows",
			byKey.Collapsed("b.go"), byKey.Len(), byCall.Collapsed("b.go"), byCall.Len())
	}
}

func TestHeaderMark(t *testing.T) {
	m := load(t, &source{size: 5, files: []File{goFile("a.go", 1), goFile("b.go", 1)}}, WithSize(40, 10),
		WithHeaderMark(func(f File) string { return map[string]string{"a.go": "OK"}[f.Path] }))
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if !strings.Contains(lines[0], "a.go") || !strings.Contains(strings.TrimRight(lines[0], " "), " OK") {
		t.Errorf("header of a.go lacks the mark: %q", lines[0])
	}
	if got := strings.Count(ansi.Strip(m.View()), "OK"); got != 1 {
		t.Errorf("%d marks drawn, want 1", got)
	}
	// A path longer than the room gives up cells to the mark, not the mark.
	long := load(t, &source{size: 5, files: []File{goFile("a/very/long/path/to/a/file/in/the/tree.go", 1)}}, WithSize(30, 6),
		WithHeaderMark(func(File) string { return "OK" }))
	v := long.View()
	assertFits(t, v, 30, 6)
	if first, _, _ := strings.Cut(ansi.Strip(v), "\n"); !strings.Contains(first, " OK") {
		t.Errorf("a narrow header lost its mark: %q", first)
	}
	m.SetHeaderMark(nil)
	if strings.Contains(ansi.Strip(m.View()), "OK") {
		t.Error("the mark is drawn after SetHeaderMark(nil)")
	}
}

func TestFileCarriesSHA(t *testing.T) {
	f := goFile("a.go", 1)
	f.SHA = "0123abc"
	m := load(t, &source{size: 5, files: []File{f}})
	if got, _ := m.CurrentFile(); got.SHA != f.SHA {
		t.Errorf("SHA = %q, want %q", got.SHA, f.SHA)
	}
}

func TestFileEndVisible(t *testing.T) {
	m := load(t, &source{size: 5, files: []File{goFile("a.go", 1), goFile("b.go", 6)}}, WithSize(60, 8))
	if !m.FileEndVisible() {
		t.Error("the whole of a.go is in the window, but its end is not visible")
	}
	m.SeekFile("b.go")
	if m.FileEndVisible() {
		t.Error("the end of b.go is visible at its header")
	}
	pages := 0
	for !m.FileEndVisible() {
		pages++
		if pages > 20 {
			t.Fatal("the end of b.go never became visible")
		}
		before := m.Cursor()
		m = run(t, m, m.PageDown())
		if m.Cursor() <= before {
			t.Fatalf("PageDown stayed at row %d", before)
		}
	}
	if f, _ := m.CurrentFile(); f.Path != "b.go" {
		t.Errorf("the cursor left b.go for %s before its end showed", f.Path)
	}
	if empty := load(t, &source{size: 5}); empty.FileEndVisible() {
		t.Error("an empty view reports the end of a file")
	}
}

func TestFileEndVisibleCollapsed(t *testing.T) {
	m := load(t, &source{size: 5, files: []File{goFile("a.go", 6), goFile("b.go", 1)}}, WithSize(60, 6))
	m.SetCollapsed("a.go", true)
	if !m.FileEndVisible() {
		t.Error("a collapsed file is one row, but its end is not visible")
	}
}

func TestPageDownMatchesKey(t *testing.T) {
	src := func() *source { return &source{size: 2, files: sizedFiles(6, 3)} }
	byKey := keys(t, load(t, src(), WithSize(60, 9)), "pgdown")
	byCall := load(t, src(), WithSize(60, 9))
	byCall = run(t, byCall, byCall.PageDown())
	if byKey.Cursor() != byCall.Cursor() || byKey.Cursor() == 0 {
		t.Errorf("cursor after the key %d, after PageDown %d", byKey.Cursor(), byCall.Cursor())
	}
}
