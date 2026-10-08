package diff

import (
	"fmt"
	"strings"
	"testing"
)

type want struct {
	kind     Kind
	old, new int
	text     string
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		patch   string
		hunks   []Hunk
		rows    []want
		wantErr bool
	}{
		{
			name:  "mixed hunk with section",
			patch: "@@ -10,4 +10,5 @@ func f() {\n a\n-b\n+c\n+d\n e\n",
			hunks: []Hunk{{10, 4, 10, 5, "func f() {"}},
			rows: []want{
				{KindHunkHeader, 0, 0, "@@ -10,4 +10,5 @@ func f() {"},
				{KindContext, 10, 10, "a"},
				{KindDeleted, 11, 0, "b"},
				{KindAdded, 0, 11, "c"},
				{KindAdded, 0, 12, "d"},
				{KindContext, 12, 13, "e"},
			},
		},
		{
			name:  "counts left out mean one",
			patch: "@@ -1 +1 @@\n-a\n+b",
			hunks: []Hunk{{1, 1, 1, 1, ""}},
			rows: []want{
				{KindHunkHeader, 0, 0, "@@ -1 +1 @@"},
				{KindDeleted, 1, 0, "a"},
				{KindAdded, 0, 1, "b"},
			},
		},
		{
			name:  "new file",
			patch: "@@ -0,0 +1,2 @@\n+a\n+b\n",
			hunks: []Hunk{{0, 0, 1, 2, ""}},
			rows: []want{
				{KindHunkHeader, 0, 0, "@@ -0,0 +1,2 @@"},
				{KindAdded, 0, 1, "a"},
				{KindAdded, 0, 2, "b"},
			},
		},
		{
			name:  "pure insertion between context",
			patch: "@@ -5,0 +6,1 @@\n+x\n@@ -9,2 +10,2 @@\n a\n b\n",
			hunks: []Hunk{{5, 0, 6, 1, ""}, {9, 2, 10, 2, ""}},
			rows: []want{
				{KindHunkHeader, 0, 0, "@@ -5,0 +6,1 @@"},
				{KindAdded, 0, 6, "x"},
				{KindHunkHeader, 0, 0, "@@ -9,2 +10,2 @@"},
				{KindContext, 9, 10, "a"},
				{KindContext, 10, 11, "b"},
			},
		},
		{
			name:  "no newline marker has no numbers",
			patch: "@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+a\n",
			hunks: []Hunk{{1, 1, 1, 1, ""}},
			rows: []want{
				{KindHunkHeader, 0, 0, "@@ -1 +1 @@"},
				{KindDeleted, 1, 0, "a"},
				{KindNoNewline, 1, 0, "\\ No newline at end of file"},
				{KindAdded, 0, 1, "a"},
			},
		},
		{
			name:  "crlf",
			patch: "@@ -1,2 +1,2 @@\r\n a\r\n-b\r\n+c\r\n",
			hunks: []Hunk{{1, 2, 1, 2, ""}},
			rows: []want{
				{KindHunkHeader, 0, 0, "@@ -1,2 +1,2 @@"},
				{KindContext, 1, 1, "a"},
				{KindDeleted, 2, 0, "b"},
				{KindAdded, 0, 2, "c"},
			},
		},
		{
			name:  "blank context line with its space trimmed",
			patch: "@@ -1,3 +1,3 @@\n a\n\n c\n",
			hunks: []Hunk{{1, 3, 1, 3, ""}},
			rows: []want{
				{KindHunkHeader, 0, 0, "@@ -1,3 +1,3 @@"},
				{KindContext, 1, 1, "a"},
				{KindContext, 2, 2, ""},
				{KindContext, 3, 3, "c"},
			},
		},
		{name: "empty", patch: ""},
		{
			name:    "bad header",
			patch:   "@@ -1,x +1 @@\n a\n",
			rows:    []want{{KindRaw, 0, 0, "@@ -1,x +1 @@"}, {KindRaw, 0, 0, " a"}},
			wantErr: true,
		},
		{
			name:    "text before the first hunk",
			patch:   "diff --git a/x b/x\n@@ -1 +1 @@\n a\n",
			rows:    []want{{KindRaw, 0, 0, "diff --git a/x b/x"}, {KindRaw, 0, 0, "@@ -1 +1 @@"}, {KindRaw, 0, 0, " a"}},
			wantErr: true,
		},
		{
			name:    "unknown line prefix",
			patch:   "@@ -1 +1 @@\n!a\n",
			rows:    []want{{KindRaw, 0, 0, "@@ -1 +1 @@"}, {KindRaw, 0, 0, "!a"}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hunks, rows, err := Parse(tt.patch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if len(rows) != patchLines(tt.patch) {
				t.Errorf("%d rows for %d lines", len(rows), patchLines(tt.patch))
			}
			if fmt.Sprint(hunks) != fmt.Sprint(tt.hunks) {
				t.Errorf("hunks = %v, want %v", hunks, tt.hunks)
			}
			if len(rows) != len(tt.rows) {
				t.Fatalf("rows = %v, want %d rows", rows, len(tt.rows))
			}
			for i, w := range tt.rows {
				r := rows[i]
				if r.Kind != w.kind || r.Old != w.old || r.New != w.new || r.Text != w.text {
					t.Errorf("row %d = {%v %d %d %q}, want %+v", i, r.Kind, r.Old, r.New, r.Text, w)
				}
				if tt.wantErr && (r.Err == "" || r.Hunk != -1) {
					t.Errorf("raw row %d: Err %q Hunk %d", i, r.Err, r.Hunk)
				}
			}
		})
	}
}

func TestHunkIndexAndRange(t *testing.T) {
	_, rows, _ := Parse("@@ -1 +1 @@\n-a\n+a\n@@ -9 +9 @@\n-b\n+b\n")
	got := []int{rows[0].Hunk, rows[1].Hunk, rows[2].Hunk, rows[3].Hunk, rows[4].Hunk, rows[5].Hunk}
	if fmt.Sprint(got) != "[0 0 0 1 1 1]" {
		t.Errorf("hunk indexes = %v", got)
	}
}

const patchA = "@@ -1,3 +1,3 @@\n a\n-b\n+c\n d\n@@ -20 +20,2 @@ tail\n x\n+y\n"

func sampleFiles() []File {
	return []File{
		{Path: "a.go", Status: StatusModified, Additions: 2, Deletions: 1, Patch: patchA},
		{Path: "img.png", Status: StatusAdded},
		{Path: "new/name.go", OldPath: "old/name.go", Status: StatusRenamed},
		{Path: "big.txt", Status: StatusModified, Additions: 9000, Truncated: true},
		{Path: "cut.go", Status: StatusModified, Additions: 1, Patch: "@@ -0,0 +1 @@\n+a\n", Truncated: true},
		{Path: "bad.go", Status: StatusModified, Additions: 1, Patch: "garbage\n"},
		{Path: "empty.txt", Status: StatusUnchanged},
	}
}

func TestLayoutRows(t *testing.T) {
	l := NewLayout(sampleFiles(), WithFirstFilesNote(7))
	if l.Len() != 23 {
		t.Fatalf("Rows = %d, want 23", l.Len())
	}
	type exp struct {
		row  int
		kind Kind
		file int
		note NoteKind
	}
	for _, e := range []exp{
		{0, KindFileHeader, 0, NoteNone},
		{1, KindHunkHeader, 0, NoteNone},
		{6, KindHunkHeader, 0, NoteNone},
		{8, KindAdded, 0, NoteNone},
		{9, KindFileHeader, 1, NoteNone},
		{10, KindNote, 1, NoteBinary},
		{12, KindNote, 2, NoteRenamed},
		{14, KindNote, 3, NoteTruncated},
		{16, KindHunkHeader, 4, NoteNone},
		{18, KindNote, 4, NoteTruncated},
		{20, KindRaw, 5, NoteNone},
		{21, KindFileHeader, 6, NoteNone},
		{22, KindNote, -1, NoteFirstFiles},
	} {
		r := rowAt(t, l, e.row)
		if r.Kind != e.kind || r.File != e.file || r.Note != e.note {
			t.Errorf("row %d = %+v, want kind %v file %d note %v", e.row, r, e.kind, e.file, e.note)
		}
	}
	if err := l.ParseErr(5); err == nil || rowAt(t, l, 20).Err == "" {
		t.Errorf("bad.go: err %v, row err %q", err, rowAt(t, l, 20).Err)
	}
	if got := rowAt(t, l, 22).Text; got != "showing the first 7 files" {
		t.Errorf("note = %q", got)
	}
	if f, ok := l.FileAt(22); ok {
		t.Errorf("FileAt(note) = %d, true", f)
	}
	for i, want := range []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 4, 4, 5, 5, 6} {
		if f, ok := l.FileAt(i); !ok || f != want {
			t.Errorf("FileAt(%d) = %d, %v, want %d", i, f, ok, want)
		}
	}
	if h := l.Hunks(0); len(h) != 2 || h[1].Section != "tail" {
		t.Errorf("Hunks = %v", h)
	}
}

func TestPosRoundTrip(t *testing.T) {
	l := NewLayout(sampleFiles())
	lines := 0
	for i := range l.Len() {
		p, ok := l.PosAt(i)
		if !ok {
			continue
		}
		lines++
		row, found := l.Find(p)
		if !found || row != i {
			t.Errorf("Find(%+v) = %d, %v; want %d", p, row, found, i)
		}
		if back, _ := l.PosAt(row); back != p {
			t.Errorf("round trip: %+v -> %d -> %+v", p, row, back)
		}
	}
	if lines != 7 {
		t.Errorf("%d positioned rows, want 7", lines)
	}
	// A context line is found on the old side too, to the same row.
	if row, ok := l.Find(Pos{"a.go", Old, 1}); !ok || row != 2 {
		t.Errorf("old side of a context line: %d, %v", row, ok)
	}
	if row, ok := l.Find(Pos{"a.go", Old, 2}); !ok || row != 3 {
		t.Errorf("deleted line: %d, %v", row, ok)
	}
	if row, ok := l.Find(Pos{"a.go", New, 500}); ok || row != 0 {
		t.Errorf("line outside the hunks: %d, %v", row, ok)
	}
	if row, ok := l.Find(Pos{"nope.go", New, 1}); ok || row != -1 {
		t.Errorf("unknown file: %d, %v", row, ok)
	}
}

func TestCollapse(t *testing.T) {
	l := NewLayout(sampleFiles())
	before := l.Len()
	l.SetCollapsed(0, true)
	if l.Len() != before-8 || !l.Collapsed(0) {
		t.Fatalf("Rows = %d after collapsing, want %d", l.Len(), before-8)
	}
	if r := rowAt(t, l, 1); r.Kind != KindFileHeader || r.File != 1 {
		t.Errorf("row after a collapsed file = %+v", r)
	}
	if _, ok := l.Find(Pos{"a.go", New, 2}); ok {
		t.Error("Find in a collapsed file succeeded")
	}
	l.SetCollapsed(0, false)
	if l.Len() != before {
		t.Errorf("Rows = %d after expanding, want %d", l.Len(), before)
	}
	if r, ok := l.FileRow(1); !ok || r != 9 {
		t.Errorf("FileRow(1) = %d, %v", r, ok)
	}
}

func TestCollapseOver(t *testing.T) {
	l := NewLayout(sampleFiles(), WithCollapseOver(7))
	if !l.Collapsed(0) || l.Collapsed(4) {
		t.Errorf("collapsed: a.go %v (8 lines), cut.go %v (2 lines)", l.Collapsed(0), l.Collapsed(4))
	}
	l = NewLayout(sampleFiles(), WithCollapseOver(8))
	if l.Collapsed(0) {
		t.Error("a file at the threshold collapsed")
	}
}

func TestLaziness(t *testing.T) {
	var parsed []string
	l := NewLayout(sampleFiles())
	l.onParse = func(p string) { parsed = append(parsed, p) }
	rowAt(t, l, 0) // a header
	rowAt(t, l, 9)
	rowAt(t, l, 10) // a note: nothing to parse
	if len(parsed) != 0 {
		t.Fatalf("parsed %v before any line was asked for", parsed)
	}
	rowAt(t, l, 3)
	rowAt(t, l, 4)
	if fmt.Sprint(parsed) != "[a.go]" {
		t.Errorf("parsed %v after reading a.go", parsed)
	}
	l.Find(Pos{"bad.go", New, 1})
	if fmt.Sprint(parsed) != "[a.go bad.go]" {
		t.Errorf("parsed %v", parsed)
	}
	l.SetCollapsed(0, true) // collapsing never parses
	l.SetCollapsed(4, true)
	if len(parsed) != 2 {
		t.Errorf("parsed %v", parsed)
	}
}

func TestNoteKinds(t *testing.T) {
	tests := []struct {
		f    File
		want NoteKind
	}{
		{File{Status: StatusAdded}, NoteBinary},
		{File{Status: StatusModified}, NoteBinary},
		{File{Status: StatusRenamed}, NoteRenamed},
		{File{Status: StatusCopied}, NoteRenamed},
		{File{Status: StatusRenamed, Additions: 1}, NoteTruncated},
		{File{Status: StatusModified, Truncated: true}, NoteTruncated},
		{File{Status: StatusUnchanged}, NoteNone},
	}
	for _, tt := range tests {
		s := state{file: tt.f}
		if got := s.noteKind(); got != tt.want {
			t.Errorf("%+v: note %v, want %v", tt.f, got, tt.want)
		}
	}
}

func TestInputIsCopied(t *testing.T) {
	files := sampleFiles()
	l := NewLayout(files)
	files[0].Path = "changed.go"
	if mustFile(t, l, 0).Path != "a.go" || l.FileIndex("a.go") != 0 || l.FileIndex("changed.go") != -1 {
		t.Error("the layout follows the caller's slice")
	}
	h := l.Hunks(0)
	h[0].OldStart = 99
	if l.Hunks(0)[0].OldStart == 99 {
		t.Error("Hunks shares its slice")
	}
}

func bigPatch(lines int) string {
	var body strings.Builder
	oldN, newN := 0, 0
	for i := 1; i < lines; i++ {
		if i%3 == 0 {
			body.WriteString("-old line\n+new line\n")
			oldN++
			newN++
			i++
		} else {
			body.WriteString(" context line\n")
			oldN++
			newN++
		}
	}
	return fmt.Sprintf("@@ -1,%d +1,%d @@\n", oldN, newN) + body.String()
}

func TestHugePatch(t *testing.T) {
	p := bigPatch(100_000)
	n := patchLines(p)
	l := NewLayout([]File{{Path: "big.go", Status: StatusModified, Patch: p}})
	if l.Len() != n+1 {
		t.Fatalf("Rows = %d, want %d", l.Len(), n+1)
	}
	last := rowAt(t, l, l.Len()-1)
	if last.File != 0 || last.Text == "" {
		t.Errorf("last row = %+v", last)
	}
	pos, ok := l.PosAt(l.Len() - 1)
	if !ok {
		t.Fatal("last row has no position")
	}
	if row, found := l.Find(pos); !found || row != l.Len()-1 {
		t.Errorf("Find(%+v) = %d, %v", pos, row, found)
	}
}

func manyFiles(n int) []File {
	files := make([]File, n)
	patch := "@@ -1,3 +1,3 @@\n a\n-b\n+c\n d\n@@ -20 +20,2 @@ tail\n x\n+y\n"
	for i := range files {
		files[i] = File{Path: fmt.Sprintf("dir/file%04d.go", i), Status: StatusModified, Additions: 2, Deletions: 1, Patch: patch}
	}
	return files
}

func TestManyFiles(t *testing.T) {
	l := NewLayout(manyFiles(3000), WithFirstFilesNote(3000))
	if want := 3000*9 + 1; l.Len() != want {
		t.Fatalf("Rows = %d, want %d", l.Len(), want)
	}
	if f, _ := l.FileAt(2999*9 + 4); f != 2999 {
		t.Errorf("FileAt = %d", f)
	}
}

func BenchmarkLayout(b *testing.B) {
	files := manyFiles(3000)
	b.Run("new", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			NewLayout(files)
		}
	})
	b.Run("scan-all-rows", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			l := NewLayout(files)
			for i := range l.Len() {
				l.RowAt(i)
			}
		}
	})
	l := NewLayout(files)
	for i := range l.Len() {
		l.RowAt(i)
	}
	b.Run("rowat-parsed", func(b *testing.B) {
		b.ReportAllocs()
		i := 0
		for b.Loop() {
			l.RowAt(i % l.Len())
			i += 7
		}
	})
	b.Run("find-parsed", func(b *testing.B) {
		b.ReportAllocs()
		p := Pos{Path: files[2500].Path, Side: New, Line: 20}
		for b.Loop() {
			l.Find(p)
		}
	})
	b.Run("collapse-first", func(b *testing.B) {
		b.ReportAllocs()
		on := false
		for b.Loop() {
			on = !on
			l.SetCollapsed(0, on)
		}
	})
	huge := bigPatch(100_000)
	b.Run("parse-100k-lines", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _, _ = Parse(huge)
		}
	})
}

func rowAt(tb testing.TB, l *Layout, i int) Row {
	tb.Helper()
	r, ok := l.RowAt(i)
	if !ok {
		tb.Fatalf("RowAt(%d) not a row of %d", i, l.Len())
	}
	return r
}

func mustFile(tb testing.TB, l *Layout, i int) File {
	tb.Helper()
	f, ok := l.File(i)
	if !ok {
		tb.Fatalf("File(%d) missing", i)
	}
	return f
}

func TestParseCounts(t *testing.T) {
	tests := []struct {
		name    string
		patch   string
		wantErr bool
	}{
		{"exact", "@@ -1,2 +1,2 @@\n a\n-b\n+c\n", false},
		{"extra added line", "@@ -1 +1 @@\n+a\n+b\n", true},
		{"extra deleted line", "@@ -1 +0,0 @@\n-a\n-b\n", true},
		{"extra context line", "@@ -1 +1 @@\n a\n b\n", true},
		{"short at the next header", "@@ -1,3 +1,3 @@\n a\n@@ -9 +9 @@\n x\n", true},
		{"short at the end, no newline", "@@ -1,2 +1,2 @@\n a", true},
		{"short by one blank context line, newline ended", "@@ -1,2 +1,2 @@\n a\n", false},
		{"short by two lines, newline ended", "@@ -1,3 +1,3 @@\n a\n", true},
		{"short by one added line", "@@ -1 +1,2 @@\n a\n", true},
		{"blank context line past the counts", "@@ -1 +1 @@\n a\n\n", true},
		{"no newline marker past the counts", "@@ -1 +1 @@\n-a\n+a\n\\ No newline at end of file\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, rows, err := Parse(tt.patch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if len(rows) != patchLines(tt.patch) {
				t.Errorf("%d rows for %d lines", len(rows), patchLines(tt.patch))
			}
			for _, r := range rows {
				if tt.wantErr != (r.Kind == KindRaw) {
					t.Errorf("row kind %v, error expected %v", r.Kind, tt.wantErr)
				}
			}
		})
	}
}

func TestParseMore(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		hunks []Hunk
		rows  []want
	}{
		{
			name:  "deleted file",
			patch: "@@ -1,2 +0,0 @@\n-a\n-b",
			hunks: []Hunk{{1, 2, 0, 0, ""}},
			rows:  []want{{KindHunkHeader, 0, 0, "@@ -1,2 +0,0 @@"}, {KindDeleted, 1, 0, "a"}, {KindDeleted, 2, 0, "b"}},
		},
		{
			name:  "no newline on the new side",
			patch: "@@ -1 +1 @@\n-a\n+a\n\\ No newline at end of file",
			hunks: []Hunk{{1, 1, 1, 1, ""}},
			rows: []want{{KindHunkHeader, 0, 0, "@@ -1 +1 @@"}, {KindDeleted, 1, 0, "a"}, {KindAdded, 0, 1, "a"},
				{KindNoNewline, 0, 1, "\\ No newline at end of file"}},
		},
		{
			name:  "no newline on both sides",
			patch: "@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n\\ No newline at end of file",
			hunks: []Hunk{{1, 1, 1, 1, ""}},
			rows: []want{{KindHunkHeader, 0, 0, "@@ -1 +1 @@"}, {KindDeleted, 1, 0, "a"}, {KindNoNewline, 1, 0, "\\ No newline at end of file"},
				{KindAdded, 0, 1, "b"}, {KindNoNewline, 0, 1, "\\ No newline at end of file"}},
		},
		{
			name:  "no newline after a context line is on both",
			patch: "@@ -3 +3 @@\n a\n\\ No newline at end of file",
			hunks: []Hunk{{3, 1, 3, 1, ""}},
			rows: []want{{KindHunkHeader, 0, 0, "@@ -3 +3 @@"}, {KindContext, 3, 3, "a"},
				{KindNoNewline, 3, 3, "\\ No newline at end of file"}},
		},
		{
			name:  "section containing @@",
			patch: "@@ -1 +1 @@ if a @@ b\n x",
			hunks: []Hunk{{1, 1, 1, 1, "if a @@ b"}},
			rows:  []want{{KindHunkHeader, 0, 0, "@@ -1 +1 @@ if a @@ b"}, {KindContext, 1, 1, "x"}},
		},
		{
			name:  "tabs are kept",
			patch: "@@ -1 +1 @@ f\t(x)\n-\ta\n+\t\tb",
			hunks: []Hunk{{1, 1, 1, 1, "f\t(x)"}},
			rows:  []want{{KindHunkHeader, 0, 0, "@@ -1 +1 @@ f\t(x)"}, {KindDeleted, 1, 0, "\ta"}, {KindAdded, 0, 1, "\t\tb"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hunks, rows, err := Parse(tt.patch)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(hunks) != fmt.Sprint(tt.hunks) {
				t.Errorf("hunks = %v, want %v", hunks, tt.hunks)
			}
			if len(rows) != len(tt.rows) {
				t.Fatalf("rows = %v", rows)
			}
			for i, w := range tt.rows {
				r := rows[i]
				if r.Kind != w.kind || r.Old != w.old || r.New != w.new || r.Text != w.text {
					t.Errorf("row %d = {%v %d %d %q}, want %+v", i, r.Kind, r.Old, r.New, r.Text, w)
				}
			}
		})
	}
}

func TestRemovedWithoutPatch(t *testing.T) {
	l := NewLayout([]File{{Path: "gone.go", Status: StatusRemoved, Deletions: 5000}})
	if r := rowAt(t, l, 1); r.Kind != KindNote || r.Note != NoteTruncated {
		t.Errorf("row = %+v, want the truncated note", r)
	}
}

func TestOutOfRange(t *testing.T) {
	l := NewLayout(sampleFiles())
	n := l.Files()
	if _, ok := l.RowAt(-1); ok {
		t.Error("RowAt(-1)")
	}
	if _, ok := l.RowAt(l.Len()); ok {
		t.Error("RowAt(Len)")
	}
	if _, ok := l.File(n); ok {
		t.Error("File(n)")
	}
	if _, ok := l.File(-1); ok {
		t.Error("File(-1)")
	}
	if l.Collapsed(n) || l.Hunks(n) != nil || l.ParseErr(n) != nil || l.Collapsed(-1) {
		t.Error("zero values expected")
	}
	l.SetCollapsed(n, true)
	l.SetCollapsed(-1, true)
	if r, ok := l.FileRow(n); !ok || r != l.Len() {
		t.Errorf("FileRow(Files()) = %d, %v, want %d", r, ok, l.Len())
	}
	if _, ok := l.FileRow(n + 1); ok {
		t.Error("FileRow(n+1)")
	}
}
