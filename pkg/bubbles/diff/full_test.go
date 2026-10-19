package diff

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// numbered is n lines "l1" ... "ln", with those of repl in place of their own.
func numbered(n int, repl map[int]string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if s, ok := repl[i]; ok {
			b.WriteString(s)
		} else {
			fmt.Fprintf(&b, "l%d", i)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// rowStrings is the body rows of the file at path, one string each: the kind,
// the hunk, the numbers on both sides and the text.
func rowStrings(t *testing.T, l *Layout, path string) []string {
	t.Helper()
	f := l.FileIndex(path)
	start, _ := l.FileRow(f)
	end, _ := l.FileRow(f + 1)
	var out []string
	for i := start + 1; i < end; i++ {
		r, ok := l.RowAt(i)
		if !ok {
			t.Fatalf("no row %d", i)
		}
		switch r.Kind {
		case KindHunkHeader:
			out = append(out, fmt.Sprintf("@@ %d", r.Hunk))
		case KindContext:
			out = append(out, fmt.Sprintf("  h%d %d/%d %s", r.Hunk, r.Old, r.New, r.Text))
		case KindAdded:
			out = append(out, fmt.Sprintf("+ h%d -/%d %s", r.Hunk, r.New, r.Text))
		case KindDeleted:
			out = append(out, fmt.Sprintf("- h%d %d/- %s", r.Hunk, r.Old, r.Text))
		default:
			out = append(out, fmt.Sprintf("kind %d", r.Kind))
		}
	}
	return out
}

func patchedFile(path, patch string) File {
	return File{Path: path, Status: StatusModified, Additions: 1, Deletions: 1, Patch: patch}
}

func TestFullFileGaps(t *testing.T) {
	tests := []struct {
		name    string
		patch   string
		content string
		want    []string
	}{
		{
			name: "first, middle and last gaps with added and deleted lines",
			patch: "@@ -3,3 +3,4 @@ f\n c3\n+added\n c4\n c5\n" +
				"@@ -10,3 +11,2 @@ g\n c10\n-del\n c12",
			content: numbered(14, map[int]string{3: "c3", 4: "added", 5: "c4", 6: "c5", 11: "c10", 12: "c12"}),
			want: []string{
				"  h-1 1/1 l1", "  h-1 2/2 l2",
				"@@ 0",
				"  h0 3/3 c3", "+ h0 -/4 added", "  h0 4/5 c4", "  h0 5/6 c5",
				"  h-1 6/7 l7", "  h-1 7/8 l8", "  h-1 8/9 l9", "  h-1 9/10 l10",
				"@@ 1",
				"  h1 10/11 c10", "- h1 11/- del", "  h1 12/12 c12",
				"  h-1 13/13 l13", "  h-1 14/14 l14",
			},
		},
		{
			name:    "a hunk at the start and the end of the file has no gaps",
			patch:   "@@ -1,3 +1,3 @@\n a\n-b\n+c\n d",
			content: "a\nc\nd\n",
			want:    []string{"@@ 0", "  h0 1/1 a", "- h0 2/- b", "+ h0 -/2 c", "  h0 3/3 d"},
		},
		{
			name:    "a hunk that only deletes names the line before it",
			patch:   "@@ -2,2 +1,0 @@\n-l2\n-l3",
			content: "l1\nl4\n",
			want:    []string{"  h-1 1/1 l1", "@@ 0", "- h0 2/- l2", "- h0 3/- l3", "  h-1 4/2 l4"},
		},
		{
			name:    "a hunk that only adds names the line before it",
			patch:   "@@ -1,0 +2,2 @@\n+x\n+y",
			content: "l1\nx\ny\nl2\n",
			want:    []string{"  h-1 1/1 l1", "@@ 0", "+ h0 -/2 x", "+ h0 -/3 y", "  h-1 2/4 l2"},
		},
		{
			name:    "windows line ends and a missing last newline",
			patch:   "@@ -2 +2 @@\n-b\n+c",
			content: "a\r\nc\r\nd",
			want:    []string{"  h-1 1/1 a", "@@ 0", "- h0 2/- b", "+ h0 -/2 c", "  h-1 3/3 d"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLayout([]File{patchedFile("a.go", tt.patch)})
			if !l.SetFull("a.go", tt.content) {
				t.Fatal("SetFull refused the file")
			}
			if got := rowStrings(t, l, "a.go"); !slices.Equal(got, tt.want) {
				t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			if !l.Full("a.go") {
				t.Error("Full is false after SetFull")
			}
			if want := 1 + len(tt.want); l.Len() != want {
				t.Errorf("Len = %d, want %d", l.Len(), want)
			}
		})
	}
}

func TestFullFileGutterFitsTheLinesOfTheGaps(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		lines int
	}{
		{"new numbers past the hunks", "@@ -1,2 +1,2 @@\n l1\n-x\n+l2", 1000},
		{"old numbers of a file that shrank", "@@ -1,2 +1,1 @@\n l1\n-x", 999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := patchedFile("a.go", tt.patch)
			m := load(t, &source{size: 5, files: []File{f}}, WithSize(80, 12))
			if got := m.numWidth(); got != minNumberWidth {
				t.Fatalf("gutter of the hunks = %d, want %d", got, minNumberWidth)
			}
			if !m.SetFull("a.go", numbered(tt.lines, nil)) {
				t.Fatal("SetFull refused the file")
			}
			if got := m.numWidth(); got != 4 {
				t.Errorf("gutter of the whole file = %d, want 4", got)
			}
			m.ClearFull("a.go")
			if got := m.numWidth(); got != minNumberWidth {
				t.Errorf("gutter after ClearFull = %d, want %d", got, minNumberWidth)
			}
		})
	}
}

func TestFullFileClearRestoresHunks(t *testing.T) {
	l := NewLayout([]File{goFile("a.go", 2), goFile("b.go", 1)})
	before, total := rowStrings(t, l, "a.go"), l.Len()
	if !l.SetFull("a.go", numbered(60, nil)) {
		t.Fatal("SetFull refused the file")
	}
	if l.Len() <= total {
		t.Fatalf("Len = %d, not more than %d", l.Len(), total)
	}
	b, _ := l.FileRow(1)
	if row, _ := l.RowAt(b); row.Kind != KindFileHeader || row.Text != "b.go" {
		t.Errorf("row %d is %+v, want the header of the next file", b, row)
	}
	l.ClearFull("a.go")
	if l.Full("a.go") || l.Len() != total {
		t.Errorf("Full = %v, Len = %d; want false, %d", l.Full("a.go"), l.Len(), total)
	}
	if got := rowStrings(t, l, "a.go"); !slices.Equal(got, before) {
		t.Errorf("rows after ClearFull:\n%v\nwant:\n%v", got, before)
	}
}

func TestFullFileRefused(t *testing.T) {
	for name, f := range map[string]File{
		"no patch":  {Path: "a.go", Status: StatusAdded},
		"truncated": {Path: "a.go", Status: StatusModified, Truncated: true, Patch: patch(1)},
		"malformed": {Path: "a.go", Status: StatusModified, Patch: "not a patch"},
	} {
		t.Run(name, func(t *testing.T) {
			l := NewLayout([]File{f})
			n := l.Len()
			if l.SetFull("a.go", "a\nb\n") || l.Full("a.go") || l.Len() != n {
				t.Errorf("SetFull took the file: Full = %v, Len = %d, was %d", l.Full("a.go"), l.Len(), n)
			}
		})
	}
	if NewLayout([]File{goFile("a.go", 1)}).SetFull("missing.go", "a\n") {
		t.Error("SetFull took a file that is not in the layout")
	}
}

// midFile is a file whose one hunk starts at line 10, with 30 lines of content.
func midFile() (file File, content string) {
	f := patchedFile("a.go", "@@ -10,3 +10,3 @@\n ctx a\n-old\n+new\n ctx c")
	return f, numbered(30, map[int]string{10: "ctx a", 11: "new", 12: "ctx c"})
}

func TestFullFileKeepsTheCursorRow(t *testing.T) {
	f, content := midFile()
	m := load(t, &source{size: 5, files: []File{f}}, WithSize(80, 12))
	m = keys(t, m, "}", "down", "down", "down") // the added line
	want, ok := m.Position()
	if !ok || want.Line != 11 || want.Side != NewSide {
		t.Fatalf("position = %+v, %v; want line 11 of the new side", want, ok)
	}
	row := m.Cursor() - m.top
	if !m.SetFull("a.go", content) {
		t.Fatal("SetFull refused the file")
	}
	if got, _ := m.Position(); got != want {
		t.Errorf("position after SetFull = %+v, want %+v", got, want)
	}
	if got := m.Cursor() - m.top; got != row {
		t.Errorf("the cursor is on window row %d, was on %d", got, row)
	}
	m.ClearFull("a.go")
	if got, _ := m.Position(); got != want {
		t.Errorf("position after ClearFull = %+v, want %+v", got, want)
	}
	if got := m.Cursor() - m.top; got != row {
		t.Errorf("the cursor is on window row %d after ClearFull, was on %d", got, row)
	}
}

func TestFullFileClearGoesToTheNearestHunkLine(t *testing.T) {
	f, content := midFile()
	tests := []struct {
		name string
		keys []string
		want Pos
	}{
		{"above the hunk", []string{"down", "down"}, Pos{Path: "a.go", Side: NewSide, Line: 10}},
		{"below the hunk", []string{"G"}, Pos{Path: "a.go", Side: NewSide, Line: 12}},
		{"on the hunk", []string{"}", "down"}, Pos{Path: "a.go", Side: NewSide, Line: 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := load(t, &source{size: 5, files: []File{f}}, WithSize(80, 12))
			if !m.SetFull("a.go", content) {
				t.Fatal("SetFull refused the file")
			}
			m = keys(t, m, tt.keys...)
			m.ClearFull("a.go")
			if got, ok := m.Position(); !ok || got != tt.want {
				t.Errorf("position after ClearFull = %+v, %v; want %+v", got, ok, tt.want)
			}
		})
	}
}

func TestFullFileKeepsTheHunkHeader(t *testing.T) {
	f, content := midFile()
	m := load(t, &source{size: 5, files: []File{f}}, WithSize(80, 12))
	m = keys(t, m, "}")
	m.SetFull("a.go", content)
	if r, _ := m.layout.RowAt(m.Cursor()); r.Kind != KindHunkHeader {
		t.Errorf("the cursor is on a %v row, want the hunk header", r.Kind)
	}
	m = keys(t, m, "g", "}")
	if r, _ := m.layout.RowAt(m.Cursor()); r.Kind != KindHunkHeader {
		t.Errorf("} went to a %v row, want the hunk header", r.Kind)
	}
	m = keys(t, m, "G", "{")
	if r, _ := m.layout.RowAt(m.Cursor()); r.Kind != KindHunkHeader {
		t.Errorf("{ went to a %v row, want the hunk header", r.Kind)
	}
}

func TestFullFileMovesTheFilesAfter(t *testing.T) {
	f, content := midFile()
	m := load(t, &source{size: 5, files: []File{f, goFile("b.go", 1)}}, WithSize(80, 12))
	m.SeekFile("b.go")
	was := m.Cursor()
	m.SetFull("a.go", content)
	if r, _ := m.layout.RowAt(m.Cursor()); r.Kind != KindFileHeader || r.Text != "b.go" {
		t.Errorf("the cursor is on %+v, want the header of b.go", r)
	}
	if m.Cursor() <= was {
		t.Errorf("cursor = %d, want it past %d", m.Cursor(), was)
	}
}
