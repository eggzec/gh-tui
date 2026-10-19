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
		// wantHeader is set when the cursor must be on the header of b.go.
		wantHeader bool
	}{
		{"in a later file", 3, "c.go", true, false},
		{"on the header", -1, "b.go", true, true},
		{"in the collapsing file", -2, "b.go", false, true},
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
			if header, _ := m.layout.FileRow(m.layout.FileIndex("b.go")); tt.wantHeader && m.Cursor() != header {
				t.Errorf("cursor on row %d, want the header of b.go at row %d", m.Cursor(), header)
			}
			m.SetCollapsed("b.go", false)
			if m.Collapsed("b.go") {
				t.Error("b.go still collapsed after expanding it")
			}
		})
	}
}

func TestSetFirstFilesNote(t *testing.T) {
	const note = "showing the first 3000 files"
	m := load(t, threeFiles(), WithSize(80, 40))
	m.SeekFile("b.go")
	m = keys(t, m, "down", "down")
	rows, cursor := m.Len(), m.Cursor()
	if strings.Contains(m.View(), note) {
		t.Fatalf("the view has the note before it is set:\n%s", m.View())
	}

	m.SetFirstFilesNote(3000)
	if got := m.Len(); got != rows+1 {
		t.Errorf("Len() = %d with the note, want %d", got, rows+1)
	}
	if got := m.Cursor(); got != cursor {
		t.Errorf("cursor on row %d after the note came, want %d", got, cursor)
	}
	if !strings.Contains(m.View(), note) {
		t.Errorf("the view lacks %q:\n%s", note, m.View())
	}

	m.SetFirstFilesNote(0)
	if got := m.Len(); got != rows {
		t.Errorf("Len() = %d without the note, want %d", got, rows)
	}
	if got := m.Cursor(); got != cursor {
		t.Errorf("cursor on row %d after the note went, want %d", got, cursor)
	}
	if strings.Contains(m.View(), note) {
		t.Errorf("the view still has the note:\n%s", m.View())
	}
}

func TestSetFirstFilesNoteUnderTheCursor(t *testing.T) {
	m := load(t, threeFiles(), WithSize(80, 40), WithFirstFilesNote(5))
	m = keys(t, m, "G")
	if m.Cursor() != m.Len()-1 {
		t.Fatalf("cursor on row %d of %d, want the note, the last", m.Cursor(), m.Len())
	}
	m.SetFirstFilesNote(0)
	if got, last := m.Cursor(), m.Len()-1; got != last {
		t.Errorf("cursor on row %d after the note went, want the last row, %d", got, last)
	}
}

func TestSetCollapsedKeepsCursorInViewWhenScrolled(t *testing.T) {
	// A window of a few rows, scrolled until its top is inside b.go: collapsing
	// b.go must leave the cursor on its header, and the header in the window.
	files := []File{goFile("a.go", 2), goFile("b.go", 3), goFile("c.go", 2)}
	m := load(t, &source{size: 5, files: files}, WithSize(60, 6))
	m.SeekFile("b.go")
	m = keys(t, m, "down", "down", "down", "down", "down", "down", "down", "down", "down", "down")
	header, _ := m.layout.FileRow(m.layout.FileIndex("b.go"))
	if m.top <= header || m.Cursor() <= header {
		t.Fatalf("setup: window top %d and cursor %d should both be inside b.go (header at %d)", m.top, m.Cursor(), header)
	}
	m.SetCollapsed("b.go", true)
	if m.Cursor() != header {
		t.Errorf("cursor on row %d, want the header of b.go at row %d", m.Cursor(), header)
	}
	if m.Cursor() < m.top || m.Cursor() >= m.top+m.bodyHeight() {
		t.Errorf("cursor %d is outside the window of rows %d to %d", m.Cursor(), m.top, m.top+m.bodyHeight()-1)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "b.go") {
		t.Errorf("the header of b.go is not drawn:\n%s", view)
	}
}

func TestSetStatusNoteEndsTheStatus(t *testing.T) {
	m := load(t, threeFiles())
	m.SetStatusNote(func() string { return "2 viewed" })
	if got := m.Status(); got != "file 1/3 · 2 viewed" {
		t.Errorf("Status() = %q, want the note after the file", got)
	}
	m.SetStatusNote(func() string { return "" })
	if got := m.Status(); got != "file 1/3" {
		t.Errorf("Status() = %q with an empty note, want none added", got)
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

func TestLineMark(t *testing.T) {
	m := load(t, &source{size: 5, files: []File{goFile("a.go", 1), goFile("b.go", 1)}}, WithSize(40, 12))
	m.SetLineMark(func(p Pos) string {
		switch {
		case p.Path == "a.go" && p.Side == NewSide && p.Line == 2:
			return "M"
		case p.Path == "a.go" && p.Side == OldSide && p.Line == 2:
			return "O"
		case p.Path == "a.go" && p.Side == NewSide && p.Line == 1:
			return "C"
		}
		return ""
	})
	v := m.View()
	assertFits(t, v, 40, 12)
	var got []string
	for l := range strings.SplitSeq(ansi.Strip(v), "\n") {
		for _, text := range []string{"ctx a0", "old b0", "new b0", "ctx c0"} {
			if strings.Contains(l, text) {
				got = append(got, l[:2])
			}
		}
	}
	// The cursor sits on the header, so the first cell of each line is blank.
	// The lines of b.go carry no mark.
	want := []string{" C", " O", " M", "  ", "  ", "  ", "  ", "  "}
	if len(got) < 5 || strings.Join(got[:5], "|") != strings.Join(want[:5], "|") {
		t.Errorf("gutters of the lines = %q, want C, O, M, none, none", got)
	}
	m.SetLineMark(nil)
	for _, mark := range []string{" C ", " O ", " M "} {
		if strings.Contains(ansi.Strip(m.View()), mark) {
			t.Errorf("the mark %q is drawn after SetLineMark(nil)", mark)
		}
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

func TestFileEndVisibleEdges(t *testing.T) {
	tests := []struct {
		name  string
		files []File
		opts  []Option
		keys  []string
		want  bool
	}{
		{"the last file, with its end in the window", []File{goFile("a.go", 1), goFile("b.go", 1)}, []Option{WithSize(60, 20)}, []string{"G"}, true},
		{"the last file, with its end below the window", []File{goFile("a.go", 1), goFile("b.go", 6)}, []Option{WithSize(60, 6)}, []string{"J"}, false},
		{"a file with no patch", []File{{Path: "e.go", Status: StatusModified}}, []Option{WithSize(60, 6)}, nil, true},
		{"a file with no patch among others", []File{goFile("a.go", 1), {Path: "e.go", Status: StatusModified}, goFile("c.go", 1)}, []Option{WithSize(60, 12)}, []string{"J"}, true},
		{"the first-files note", []File{goFile("a.go", 1), goFile("b.go", 1)}, []Option{WithSize(60, 20), WithFirstFilesNote(5)}, []string{"G"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := load(t, &source{size: 5, files: tt.files}, tt.opts...)
			m = keys(t, m, tt.keys...)
			if tt.name == "the first-files note" {
				if _, ok := m.CurrentFile(); ok || m.Cursor() != m.Len()-1 {
					t.Fatalf("setup: cursor on row %d of %d, want the note after the last file", m.Cursor(), m.Len())
				}
			}
			if got := m.FileEndVisible(); got != tt.want {
				t.Errorf("FileEndVisible() = %v, want %v (cursor %d, top %d, %d rows)", got, tt.want, m.Cursor(), m.top, m.Len())
			}
		})
	}
}

func TestPageDownWithinFileStopsAtTheEnd(t *testing.T) {
	// Windows of 5 to 12 rows put the end of a.go at every phase of a page,
	// whether its tail is shorter than a window or not.
	for height := 6; height <= 13; height++ {
		m := load(t, &source{size: 5, files: []File{goFile("a.go", 8), goFile("b.go", 1)}}, WithSize(60, height))
		pages := 0
		for !m.FileEndVisible() {
			pages++
			if pages > 30 {
				t.Fatalf("height %d: the end of a.go never became visible", height)
			}
			before := m.Cursor()
			m = run(t, m, m.PageDownWithinFile())
			if m.Cursor() <= before {
				t.Fatalf("height %d: PageDownWithinFile stayed at row %d", height, before)
			}
			if f, _ := m.CurrentFile(); f.Path != "a.go" {
				t.Fatalf("height %d: the cursor left a.go for %s before its end showed", height, f.Path)
			}
		}
		if pages < 2 {
			t.Errorf("height %d: a.go showed its end after %d pages, want it longer than a window", height, pages)
		}
		// With the end in view, the next page goes on into b.go.
		m = run(t, m, m.PageDownWithinFile())
		if f, _ := m.CurrentFile(); f.Path != "b.go" {
			t.Errorf("height %d: the page after the end is in %s, want b.go", height, f.Path)
		}
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
