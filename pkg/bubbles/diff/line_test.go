package diff

import (
	"strings"
	"testing"
)

func TestLine(t *testing.T) {
	t.Parallel()
	m := load(t, &source{files: []File{
		goFile("pkg/a.go", 1),
		{Path: "pkg/cut.go", Status: StatusModified, Truncated: true},
	}, size: 10})
	tests := []struct {
		name      string
		pos       Pos
		text      string
		shown     bool
		known     bool
		wantInTxt string
	}{
		{"context line on the new side", Pos{Path: "pkg/a.go", Side: NewSide, Line: 1}, "", true, true, "ctx a0"},
		{"context line on the old side", Pos{Path: "pkg/a.go", Side: OldSide, Line: 1}, "", true, true, "ctx a0"},
		{"added line", Pos{Path: "pkg/a.go", Side: NewSide, Line: 2}, "", true, true, "new b0"},
		{"deleted line", Pos{Path: "pkg/a.go", Side: OldSide, Line: 2}, "", true, true, "old b0"},
		{"a deleted line is not on the new side", Pos{Path: "pkg/a.go", Side: NewSide, Line: 20}, "", false, true, ""},
		{"a line the patch lacks", Pos{Path: "pkg/a.go", Side: NewSide, Line: 99}, "", false, true, ""},
		{"a file that is not in the diff, all pages read", Pos{Path: "pkg/none.go", Side: NewSide, Line: 1}, "", false, true, ""},
		{"a file whose diff is cut short", Pos{Path: "pkg/cut.go", Side: NewSide, Line: 1}, "", false, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			text, shown, known := m.Line(tt.pos)
			if shown != tt.shown || known != tt.known || !strings.Contains(text, tt.wantInTxt) {
				t.Errorf("Line(%+v) = %q, %v, %v; want text with %q, shown %v, known %v", tt.pos, text, shown, known, tt.wantInTxt, tt.shown, tt.known)
			}
			if !tt.shown && text != "" {
				t.Errorf("Line(%+v) = %q for a line that is not shown, want no text", tt.pos, text)
			}
		})
	}
}

// A file that comes with a later page is not known yet, so that nothing
// settles a draft on it too early.
func TestLineUnknownBeforeItsPage(t *testing.T) {
	t.Parallel()
	src := &source{files: sizedFiles(3, 1), size: 1}
	m := New(src.fetch, WithKeyMap(testKeyMap), WithSize(80, 12), WithFocused(true))
	if _, shown, known := m.Line(Pos{Path: "pkg/f2.go", Side: NewSide, Line: 1}); shown || known {
		t.Errorf("Line before any page = shown %v, known %v, want neither", shown, known)
	}
}
