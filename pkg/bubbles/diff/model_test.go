package diff

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// threeFiles is a file of three hunks (rows 0 to 15, hunk headers at 1, 6
// and 11), one of two hunks (rows 16 to 26, hunk headers at 17 and 22) and a
// binary one (rows 27 and 28).
func threeFiles() *source {
	return &source{size: 10, files: []File{
		goFile("a.go", 3),
		goFile("b.go", 2),
		{Path: "logo.png", Status: StatusAdded},
	}}
}

// Every key moves the cursor, or the view, as it says.
func TestKeys(t *testing.T) {
	tests := []struct {
		name  string
		from  int
		key   string
		want  int
		check func(t *testing.T, m Model)
	}{
		{name: "down", from: 0, key: "down", want: 1},
		{name: "down j", from: 3, key: "j", want: 4},
		{name: "down on the last row", from: 28, key: "down", want: 28},
		{name: "up", from: 5, key: "up", want: 4},
		{name: "up on the first row", from: 0, key: "up", want: 0},
		{name: "page down", from: 0, key: "pgdown", want: 11},
		{name: "page down past the end", from: 25, key: "ctrl+f", want: 28},
		{name: "page up", from: 20, key: "pgup", want: 9},
		{name: "page up past the start", from: 4, key: "ctrl+b", want: 0},
		{name: "half page down", from: 0, key: "ctrl+d", want: 5},
		{name: "half page up", from: 10, key: "ctrl+u", want: 5},
		{name: "top", from: 20, key: "g", want: 0},
		{name: "top home", from: 20, key: "home", want: 0},
		{name: "bottom", from: 3, key: "G", want: 28},
		{name: "bottom end", from: 3, key: "end", want: 28},
		{name: "next file", from: 0, key: "J", want: 16},
		{name: "next file from inside", from: 20, key: "J", want: 27},
		{name: "next file on the last file", from: 27, key: "J", want: 27},
		{name: "next file inside the last file", from: 28, key: "J", want: 28},
		{name: "prev file inside a file goes to its header", from: 20, key: "K", want: 16},
		{name: "prev file on a header", from: 16, key: "K", want: 0},
		{name: "prev file on the first header", from: 0, key: "K", want: 0},
		{name: "prev file inside the first file", from: 3, key: "K", want: 0},
		{name: "next hunk", from: 0, key: "}", want: 1},
		{name: "next hunk from a hunk header", from: 1, key: "}", want: 6},
		{name: "next hunk from inside a hunk", from: 8, key: "}", want: 11},
		{name: "next hunk goes to the next file's first", from: 13, key: "}", want: 17},
		{name: "next hunk past the last hunk", from: 22, key: "}", want: 22},
		{name: "next hunk inside the last hunk", from: 25, key: "}", want: 25},
		{name: "prev hunk goes to the hunk's header", from: 8, key: "{", want: 6},
		{name: "prev hunk from a hunk header", from: 6, key: "{", want: 1},
		{name: "prev hunk before the first", from: 1, key: "{", want: 1},
		{name: "prev hunk on the first header", from: 0, key: "{", want: 0},
		{name: "prev hunk crosses files", from: 17, key: "{", want: 11},
		{name: "prev hunk from a header crosses files", from: 16, key: "{", want: 11},
		{name: "fold on a header", from: 0, key: "enter", want: 0, check: func(t *testing.T, m Model) {
			t.Helper()
			if !m.layout.Collapsed(0) || m.Len() != 14 {
				t.Errorf("collapsed %v, %d rows; want folded, 14", m.layout.Collapsed(0), m.Len())
			}
		}},
		{name: "fold off a header does nothing", from: 3, key: "enter", want: 3, check: func(t *testing.T, m Model) {
			t.Helper()
			if m.layout.Collapsed(0) {
				t.Error("folded from inside the file")
			}
		}},
		{name: "fold again unfolds", from: 16, key: "enter", want: 16, check: func(t *testing.T, m Model) {
			t.Helper()
			if !m.layout.Collapsed(1) {
				t.Error("file not folded")
			}
			m = keys(t, m, "enter")
			if m.layout.Collapsed(1) || m.Len() != 29 {
				t.Errorf("still folded, or %d rows", m.Len())
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := load(t, threeFiles())
			m.cursor = tt.from
			m.scroll()
			m = keys(t, m, tt.key)
			if m.Cursor() != tt.want {
				t.Errorf("cursor = %d, want %d", m.Cursor(), tt.want)
			}
			if tt.check != nil {
				tt.check(t, m)
			}
			assertFits(t, m.View(), 80, 12)
		})
	}
}

func TestSidewaysScroll(t *testing.T) {
	// Line numbers of five digits make the gutter wider than its least.
	long := strings.Repeat("0123456789", 12) + "<END>"
	src := &source{size: 5, files: []File{{Path: "a.go", Status: StatusModified, Additions: 1, Patch: "@@ -99998,0 +99999 @@\n+" + long}}}
	m := keys(t, load(t, src, WithSize(40, 6)), "down", "down")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "+0123456789") {
		t.Fatalf("line does not start at its start:\n%s", v)
	}
	m = keys(t, m, "l")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "+89012345") {
		t.Fatalf("one step right did not scroll 8 cells:\n%s", v)
	}
	for range 40 {
		m = keys(t, m, "right")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "6789<END>") {
		t.Errorf("the end of the line cannot be scrolled into view:\n%s", v)
	}
	if want := len(long) - m.textWidth(m.numWidth()); m.left != want {
		t.Errorf("scrolled to %d, want exactly the end of the line (%d)", m.left, want)
	}
	for range 40 {
		m = keys(t, m, "left")
	}
	if m.left != 0 {
		t.Errorf("left = %d after scrolling back, want 0", m.left)
	}
	// Headers do not scroll.
	m = keys(t, m, "l", "l", "up")
	if v := m.View(); !strings.Contains(v, "@@ -99998,0 +99999 @@") {
		t.Errorf("the hunk header scrolled:\n%s", v)
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := load(t, threeFiles())
	m.Blur()
	m = keys(t, m, "down", "J", "enter")
	if m.Cursor() != 0 || m.layout.Collapsed(0) {
		t.Errorf("a blurred view moved to %d", m.Cursor())
	}
	if New((&source{}).fetch).Focused() {
		t.Error("a new view starts focused")
	}
}

func TestPagesLoadNearTheEnd(t *testing.T) {
	files := sizedFiles(20, 5)
	src := &source{size: 2, files: files}
	m := load(t, src)
	if m.Files() != 2 || m.Done() {
		t.Fatalf("%d files, done %v; want the first page", m.Files(), m.Done())
	}
	m = keys(t, m, "down", "down", "down")
	if len(src.calls) != 1 {
		t.Fatalf("fetched %q before the end was near", src.calls)
	}
	m = keys(t, m, "G")
	if m.Files() != 4 || len(src.calls) != 2 || src.calls[1] != "2" {
		t.Fatalf("%d files after calls %q, want a second page", m.Files(), src.calls)
	}
	for !m.Done() {
		m = keys(t, m, "G")
	}
	if m.Files() != 20 {
		t.Errorf("%d files, want 20", m.Files())
	}
	n := len(src.calls)
	keys(t, m, "G")
	if len(src.calls) != n {
		t.Error("fetched after the last page")
	}
}

func TestFilesMsg(t *testing.T) {
	src := &source{size: 2, files: []File{goFile("a.go", 1), goFile("b.go", 1), goFile("c.go", 1)}}
	m := New(src.fetch, WithKeyMap(testKeyMap), WithSize(80, 12))
	var msgs []tea.Msg
	var collect func(tea.Cmd)
	collect = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				collect(c)
			}
		case pageMsg:
			var next tea.Cmd
			m, next = m.Update(msg)
			collect(next)
		default:
			msgs = append(msgs, msg)
		}
	}
	collect(m.Init())
	var got []FilesMsg
	for _, msg := range msgs {
		if f, ok := msg.(FilesMsg); ok {
			got = append(got, f)
		}
	}
	if len(got) != 2 || got[0].ID != m.ID() || len(got[0].Files) != 2 || got[0].Done || len(got[1].Files) != 1 || !got[1].Done {
		t.Errorf("FilesMsgs = %+v, want a page of two, then the last of one", got)
	}
}

func TestMessagesAreScopedByID(t *testing.T) {
	src := &source{size: 5, files: []File{goFile("a.go", 1)}}
	a := New(src.fetch, WithKeyMap(testKeyMap), WithSize(80, 8))
	b := New(src.fetch, WithKeyMap(testKeyMap), WithSize(80, 8))
	if a.ID() == b.ID() {
		t.Fatal("two views share an ID")
	}
	msg := a.Init()()
	b, _ = b.Update(msg)
	if b.Files() != 0 {
		t.Error("a view took the page of another")
	}
	a, _ = a.Update(msg)
	if a.Files() != 1 {
		t.Error("a view dropped its own page")
	}
}

func TestEmpty(t *testing.T) {
	m := load(t, &source{size: 5})
	if !m.Done() || m.Len() != 0 {
		t.Fatalf("done %v, %d rows", m.Done(), m.Len())
	}
	if v := m.View(); !strings.Contains(v, "No files changed.") {
		t.Errorf("view lacks the empty text:\n%s", v)
	}
	m = keys(t, m, "down", "J", "}", "enter", "G", "K", "{", "pgdown")
	if m.Cursor() != 0 {
		t.Errorf("cursor = %d", m.Cursor())
	}
	if _, ok := m.CurrentFile(); ok {
		t.Error("a file is current with no files")
	}
	if m.Status() != "" {
		t.Errorf("status %q with no files", m.Status())
	}
}

func TestErrorAndRetry(t *testing.T) {
	src := threeFiles()
	src.failWith(errBoom)
	m := load(t, src, WithSize(120, 12))
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Couldn't load files: GET /repos/o/r/pulls/1/files: 502 Bad Gateway") || !strings.Contains(v, "r to retry") {
		t.Errorf("view lacks the error and the hint:\n%s", v)
	}
	if m.Err() == nil || m.Done() {
		t.Errorf("err %v, done %v", m.Err(), m.Done())
	}
	n := len(src.calls)
	m = keys(t, m, "down", "G")
	if len(src.calls) != n {
		t.Error("fetched again without a retry")
	}
	src.failWith(nil)
	m = keys(t, m, "r")
	if m.Err() != nil || m.Files() != 3 || strings.Contains(m.View(), "retry") {
		t.Errorf("err %v, %d files after retry:\n%s", m.Err(), m.Files(), m.View())
	}
	if keys(t, m, "r").Files() != 3 || len(src.calls) != n+1 {
		t.Error("retry fetched without a failure")
	}
}

func TestErrorAfterRows(t *testing.T) {
	files := sizedFiles(6, 3)
	src := &source{size: 2, files: files}
	m := load(t, src)
	src.failWith(errBoom)
	m = keys(t, m, "G")
	v := m.View()
	if !strings.Contains(v, "Couldn't load files") {
		t.Errorf("the error is not below the rows:\n%s", v)
	}
	if m.Cursor() != m.Len()-1 {
		t.Errorf("cursor = %d, want the last row", m.Cursor())
	}
}

func TestPositionAndFiles(t *testing.T) {
	m := load(t, threeFiles())
	if p, ok := m.Position(); ok {
		t.Errorf("a header has the position %+v", p)
	}
	m = keys(t, m, "}", "down", "down", "down")
	// Row 4 is the added line of the first hunk: new line 2.
	if p, ok := m.Position(); !ok || p != (Pos{"a.go", NewSide, 2}) {
		t.Errorf("Position() = %+v, %v", p, ok)
	}
	m = keys(t, m, "up")
	if p, ok := m.Position(); !ok || p != (Pos{"a.go", OldSide, 2}) {
		t.Errorf("Position() on a deleted line = %+v, %v", p, ok)
	}
	if f, ok := m.CurrentFile(); !ok || f.Path != "a.go" {
		t.Errorf("CurrentFile() = %+v, %v", f, ok)
	}
	m = keys(t, m, "J")
	if f, _ := m.CurrentFile(); f.Path != "b.go" {
		t.Errorf("CurrentFile() = %q after J", f.Path)
	}
}

func TestSeek(t *testing.T) {
	m := load(t, threeFiles())
	if !m.SeekFile("b.go") || m.Cursor() != 16 {
		t.Errorf("SeekFile(b.go) put the cursor on %d", m.Cursor())
	}
	if m.SeekFile("nope.go") || m.Cursor() != 16 {
		t.Error("SeekFile of an unknown file moved or succeeded")
	}
	if !m.Seek(Pos{"a.go", NewSide, 2}) || m.Cursor() != 4 {
		t.Errorf("Seek put the cursor on %d", m.Cursor())
	}
	if m.Seek(Pos{"a.go", NewSide, 900}) || m.Cursor() != 0 {
		t.Errorf("Seek of a missing line: cursor %d, want the header", m.Cursor())
	}
	if m.Seek(Pos{"nope.go", NewSide, 1}) {
		t.Error("Seek of an unknown file succeeded")
	}
	// A line of a folded file unfolds it.
	m.cursor = 0
	m = keys(t, m, "enter")
	if !m.layout.Collapsed(0) {
		t.Fatal("file not folded")
	}
	if !m.Seek(Pos{"a.go", NewSide, 22}) {
		t.Fatal("Seek into a folded file failed")
	}
	if m.layout.Collapsed(0) {
		t.Error("the file stayed folded")
	}
	if p, ok := m.Position(); !ok || p != (Pos{"a.go", NewSide, 22}) {
		t.Errorf("Position() = %+v, %v", p, ok)
	}
	assertFits(t, m.View(), 80, 12)
}

func TestSeekFetchesWhatTheWindowShows(t *testing.T) {
	files := sizedFiles(6, 3)
	src := &source{size: 2, files: files}
	m := load(t, src)
	m.SeekFile("pkg/f1.go")
	m = keys(t, m, "down")
	if m.Files() <= 2 {
		t.Errorf("%d files; a seek near the end fetched nothing", m.Files())
	}
}

func TestViewCollapseOver(t *testing.T) {
	src := &source{size: 5, files: []File{goFile("small.go", 1), goFile("big.go", 4)}}
	m := load(t, src, WithCollapseOver(10))
	if m.layout.Collapsed(0) || !m.layout.Collapsed(1) {
		t.Fatalf("collapsed = %v %v, want only the big file", m.layout.Collapsed(0), m.layout.Collapsed(1))
	}
	m = keys(t, m, "J", "enter")
	if m.layout.Collapsed(1) {
		t.Error("enter did not unfold the file")
	}
}

// A page that arrives keeps the rows before it where they are, and the
// folds the user made.
func TestPageKeepsFolds(t *testing.T) {
	files := sizedFiles(8, 1)
	src := &source{size: 2, files: files}
	m := load(t, src)
	m = keys(t, m, "enter")
	row := m.Cursor()
	m = keys(t, m, "G")
	if !m.layout.Collapsed(0) || m.Files() <= 2 {
		t.Errorf("fold lost, or no page arrived (%d files)", m.Files())
	}
	m.SeekFile("pkg/f0.go")
	if m.Cursor() != row {
		t.Errorf("first header moved to %d", m.Cursor())
	}
}

func TestResizeKeepsPosition(t *testing.T) {
	m := load(t, &source{size: 10, files: sizedFiles(6, 3)}, WithSize(80, 30))
	m = keys(t, m, "}", "}", "}", "}", "down", "down", "down")
	want, ok := m.Position()
	if !ok {
		t.Fatal("no position")
	}
	for _, size := range [][2]int{{20, 3}, {200, 40}, {5, 2}, {80, 6}, {1, 1}, {69, 9}, {70, 9}, {80, 30}, {30, 4}} {
		m.SetSize(size[0], size[1])
		m, _ = m.Update(nil)
		if got, _ := m.Position(); got != want {
			t.Errorf("at %v the position is %+v, want %+v", size, got, want)
		}
		v := m.View()
		assertFits(t, v, size[0], size[1])
		// The cursor stays in view whatever the window.
		if n := strings.Count(ansi.Strip(v), "▌"); n != 1 {
			t.Errorf("at %v the cursor is drawn %d times:\n%s", size, n, v)
		}
	}
	m.SetSize(0, 5)
	if m.View() != "" {
		t.Error("zero width should render nothing")
	}
}

func TestStatus(t *testing.T) {
	m := load(t, threeFiles())
	if got := m.Status(); got != "file 1/3" {
		t.Errorf("Status() on a header = %q", got)
	}
	m = keys(t, m, "}", "}", "down")
	if got := m.Status(); got != "hunk 2/3 · file 1/3" {
		t.Errorf("Status() = %q", got)
	}
	m = keys(t, m, "J", "J")
	if got := m.Status(); got != "file 3/3" {
		t.Errorf("Status() on a file without hunks = %q", got)
	}
	files := sizedFiles(5, 9)
	m = load(t, &source{size: 1, files: files})
	if got := m.Status(); got != "file 1/1+" {
		t.Errorf("Status() while more is to come = %q", got)
	}
}

func TestStickyHeader(t *testing.T) {
	src := &source{size: 5, files: []File{goFile("a.go", 6), goFile("b.go", 6)}}
	m := load(t, src, WithSize(60, 7))
	if m.sticky() {
		t.Fatal("sticky at the top")
	}
	m = keys(t, m, "ctrl+d", "ctrl+d", "ctrl+d")
	if !m.sticky() {
		t.Fatalf("not sticky inside a file (top %d, cursor %d)", m.top, m.cursor)
	}
	first, _, _ := strings.Cut(m.View(), "\n")
	if !strings.Contains(first, "a.go") {
		t.Errorf("first row is %q, want the header of a.go", first)
	}
	// The cursor is never under the header.
	for range 40 {
		m = keys(t, m, "down")
		if m.sticky() && m.cursor <= m.top {
			t.Fatalf("cursor %d hidden under the header at top %d", m.cursor, m.top)
		}
	}
	for range 60 {
		m = keys(t, m, "up")
		if m.sticky() && m.cursor <= m.top {
			t.Fatalf("cursor %d hidden under the header at top %d", m.cursor, m.top)
		}
		if strings.Count(m.View(), "▌") != 1 {
			t.Fatalf("the cursor is not drawn once at %d:\n%s", m.cursor, m.View())
		}
	}
}

// A seek to a file that is not fetched yet keeps paging until it arrives,
// and a key press gives it up.
func TestSeekPending(t *testing.T) {
	src := &source{size: 2, files: sizedFiles(10, 3)}
	m := load(t, src)
	if m.SeekFile("pkg/f7.go") {
		t.Fatal("SeekFile reported a file that is not fetched")
	}
	m = keys(t, m)
	m, cmd := m.Update(nil)
	m = run(t, m, cmd)
	if f, _ := m.CurrentFile(); f.Path != "pkg/f7.go" || m.isHeader(m.Cursor()) == false {
		t.Errorf("the cursor is in %q at %d, want the header of pkg/f7.go", f.Path, m.Cursor())
	}

	m = load(t, &source{size: 2, files: sizedFiles(10, 3)})
	if m.Seek(Pos{"pkg/f9.go", NewSide, 2}) {
		t.Fatal("Seek reported a line of a file that is not fetched")
	}
	m, cmd = m.Update(nil)
	m = run(t, m, cmd)
	if p, ok := m.Position(); !ok || p != (Pos{"pkg/f9.go", NewSide, 2}) {
		t.Errorf("Position() = %+v, %v after the pending Seek", p, ok)
	}

	// A file that never arrives stops at the last page.
	m = load(t, &source{size: 4, files: sizedFiles(10, 1)})
	m.SeekFile("nope.go")
	m, cmd = m.Update(nil)
	m = run(t, m, cmd)
	if !m.Done() || m.pg.seek != nil {
		t.Errorf("done %v, pending %v; want the last page and no seek", m.Done(), m.pg.seek)
	}

	// A key press cancels it.
	m = load(t, &source{size: 2, files: sizedFiles(10, 3)})
	m.SeekFile("pkg/f9.go")
	m = keys(t, m, "down")
	if m.Files() != 4 || m.pg.seek != nil {
		t.Errorf("%d files, pending %v after a key press", m.Files(), m.pg.seek)
	}
}

// A source that answers a page with no files and the cursor it was asked
// with is not asked again.
func TestSameCursorWithNoFiles(t *testing.T) {
	calls := 0
	fetch := func(context.Context, string) ([]File, string, error) {
		calls++
		return nil, "same", nil
	}
	m := New(fetch, WithKeyMap(testKeyMap), WithSize(80, 8))
	m = run(t, m, m.Init())
	if calls != 2 || !m.Done() {
		t.Errorf("%d fetches, done %v", calls, m.Done())
	}
}

// The copies of a model agree on what was fetched, so a page that arrives
// for one is not appended again by another.
func TestCopiesShareTheFetchedPages(t *testing.T) {
	src := &source{size: 5, files: []File{goFile("a.go", 1)}}
	a := New(src.fetch, WithKeyMap(testKeyMap), WithSize(80, 8))
	b := a
	msg := a.Init()()
	a, _ = a.Update(msg)
	b, _ = b.Update(msg)
	if a.Files() != 1 || b.Files() != 1 {
		t.Errorf("%d and %d files, want the page once", a.Files(), b.Files())
	}
}

// The gutter is as wide as the widest line number of the files in the
// window, so it does not change while the window scrolls inside a file.
func TestNumberWidthSteadyInAFile(t *testing.T) {
	var p strings.Builder
	p.WriteString("@@ -1,30 +1,30 @@\n")
	for range 29 {
		p.WriteString(" x\n")
	}
	p.WriteString(" x\n@@ -99990,3 +99990,3 @@\n a\n b\n c")
	src := &source{size: 5, files: []File{{Path: "a.go", Status: StatusModified, Additions: 1, Patch: p.String()}}}
	m := load(t, src, WithSize(80, 8))
	want := m.numWidth()
	if want != 5 {
		t.Fatalf("numWidth() = %d, want 5 from the last hunk", want)
	}
	for range 40 {
		m = keys(t, m, "down")
		if got := m.numWidth(); got != want {
			t.Fatalf("numWidth() = %d at row %d, want %d", got, m.Cursor(), want)
		}
	}
}

// With two rows, the loading row still shows below the last row.
func TestLoadingRowInATwoRowWindow(t *testing.T) {
	calls := 0
	fetch := func(context.Context, string) ([]File, string, error) {
		calls++
		if calls > 1 {
			return nil, "", errBoom
		}
		return []File{{Path: "a.png", Status: StatusAdded}}, "1", nil
	}
	m := New(fetch, WithKeyMap(testKeyMap), WithSize(60, 2), WithFocused(true))
	m = run(t, m, m.Init())
	m = keys(t, m, "down", "down")
	if v := m.View(); !strings.Contains(v, "Couldn't load") || m.Cursor() != 1 || strings.Count(ansi.Strip(v), "▌") != 1 {
		t.Errorf("cursor %d, view:\n%s", m.Cursor(), v)
	}
}
