package diff

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func asciiStyles() Styles {
	st := DefaultStyles(true)
	st.CursorGlyph, st.ErrorGlyph = ">", "x"
	st.FoldOpen, st.FoldClosed, st.RenameArrow = "v", ">", "->"
	st.Separator, st.Ellipsis = " - ", "..."
	return st
}

// The views are compared without their styles, to read the goldens.
func TestView(t *testing.T) {
	failing := &source{size: 5, files: []File{goFile("a.go", 1)}, fail: errBoom}
	tests := []struct {
		name  string
		model func(t *testing.T) Model
	}{
		{"basic", func(t *testing.T) Model {
			t.Helper()
			return load(t, &source{size: 5, files: []File{goFile("a.go", 2), {
				Path: "README.md", Status: StatusModified, Additions: 1,
				Patch: "@@ -1 +1,2 @@\n title\n+a line without a newline\n\\ No newline at end of file",
			}}}, WithSize(80, 16))
		}},
		{"collapsed", func(t *testing.T) Model {
			t.Helper()
			return load(t, &source{size: 5, files: []File{goFile("small.go", 1), goFile("big.go", 4), goFile("tail.go", 1)}},
				WithSize(60, 12), WithCollapseOver(10))
		}},
		{"sticky", func(t *testing.T) Model {
			t.Helper()
			m := load(t, &source{size: 5, files: []File{goFile("a.go", 6), goFile("b.go", 2)}}, WithSize(60, 8))
			return keys(t, m, "ctrl+d", "ctrl+d", "ctrl+d")
		}},
		{"narrow", func(t *testing.T) Model {
			t.Helper()
			return load(t, &source{size: 5, files: []File{goFile("a/very/long/path/name.go", 2)}}, WithSize(24, 9))
		}},
		{"binary", func(t *testing.T) Model {
			t.Helper()
			return load(t, &source{size: 5, files: []File{{Path: "logo.png", Status: StatusAdded}, {Path: "empty", Status: StatusAdded}}}, WithSize(50, 6))
		}},
		{"truncated", func(t *testing.T) Model {
			t.Helper()
			return load(t, &source{size: 5, files: []File{
				{Path: "huge.json", Status: StatusModified, Additions: 90000, Deletions: 4},
				{Path: "cut.go", Status: StatusModified, Additions: 5, Deletions: 1, Truncated: true, Patch: patch(1)},
			}}, WithSize(60, 12))
		}},
		{"rename", func(t *testing.T) Model {
			t.Helper()
			return load(t, &source{size: 5, files: []File{
				{Path: "new/name.go", OldPath: "old/name.go", Status: StatusRenamed},
				{Path: "moved.go", OldPath: "was.go", Status: StatusRenamed, Additions: 1, Deletions: 1, Patch: patch(1)},
			}}, WithSize(60, 12))
		}},
		{"empty", func(t *testing.T) Model {
			t.Helper()
			return load(t, &source{size: 5}, WithSize(40, 4))
		}},
		{"error", func(t *testing.T) Model {
			t.Helper()
			return load(t, failing, WithSize(70, 4))
		}},
		{"ascii", func(t *testing.T) Model {
			t.Helper()
			m := load(t, &source{size: 5, files: []File{
				{Path: "new.go", OldPath: "old.go", Status: StatusRenamed, Additions: 1, Deletions: 1, Patch: patch(1)},
				goFile("b.go", 1),
			}}, WithSize(50, 10), WithStyles(asciiStyles()), WithCollapseOver(5))
			return keys(t, m, "down", "down")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model(t)
			v := m.View()
			assertFits(t, v, m.Width(), m.Height())
			golden.RequireEqual(t, ansi.Strip(v))
		})
	}
}

func TestViewASCII(t *testing.T) {
	m := load(t, &source{size: 5, files: []File{
		{Path: "new.go", OldPath: "old.go", Status: StatusRenamed, Additions: 1, Deletions: 1, Patch: patch(1)},
		goFile("b.go", 1),
	}}, WithSize(50, 10), WithStyles(asciiStyles()), WithCollapseOver(5))
	v := ansi.Strip(m.View())
	if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
		t.Errorf("view has glyphs beyond ASCII:\n%s", v)
	}
	for _, want := range []string{"> ", "old.go -> new.go", "v "} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
}

func TestViewStyles(t *testing.T) {
	m := load(t, threeFiles(), WithSize(80, 12))
	m = keys(t, m, "}", "down", "down", "down")
	v := m.View()
	st := m.Styles()
	for name, want := range map[string]string{
		"added":   st.Added.Render("+new b0"),
		"deleted": st.Deleted.Render("-old b0"),
		"hunk":    st.HunkHeader.Render("@@"),
		"cursor":  st.Cursor.Render("▌"),
	} {
		_, prefix, _ := strings.Cut(want, "\x1b[")
		if prefix == "" {
			continue
		}
		if !strings.Contains(v, "\x1b["+strings.SplitN(prefix, "m", 2)[0]+"m") {
			t.Errorf("view lacks the %s style", name)
		}
	}
	m.Blur()
	if strings.Contains(m.View(), st.Cursor.Render("▌")) {
		t.Error("a blurred view draws the focused cursor")
	}
}

func TestViewGutter(t *testing.T) {
	src := func() *source { return &source{size: 5, files: []File{goFile("a.go", 1)}} }
	wide := ansi.Strip(load(t, src(), WithSize(80, 8)).View())
	if !strings.Contains(wide, "  1   1  ctx a0") || !strings.Contains(wide, "  2     -old b0") || !strings.Contains(wide, "      2 +new b0") {
		t.Errorf("wide view lacks the two numbers:\n%s", wide)
	}
	narrow := ansi.Strip(load(t, src(), WithSize(69, 8)).View())
	if !strings.Contains(narrow, "  1  ctx a0") || !strings.Contains(narrow, "  2 -old b0") || !strings.Contains(narrow, "  2 +new b0") {
		t.Errorf("narrow view lacks the one number:\n%s", narrow)
	}
	// The numbers are as wide as the largest in view.
	big := &source{size: 5, files: []File{{Path: "a.go", Status: StatusModified, Additions: 1, Patch: "@@ -99999,1 +99999,2 @@\n x\n+y"}}}
	if v := ansi.Strip(load(t, big, WithSize(80, 6)).View()); !strings.Contains(v, " 99999  99999  x") {
		t.Errorf("view lacks the wide numbers:\n%s", v)
	}
}

func TestViewTabsAndWideText(t *testing.T) {
	patch := "@@ -1,3 +1,3 @@\n \tindented\n-日本語のテキスト\n+tab\there 日本"
	src := &source{size: 5, files: []File{{Path: "a.go", Status: StatusModified, Additions: 1, Deletions: 1, Patch: patch}}}
	for _, w := range []int{20, 30, 40, 80} {
		m := load(t, src, WithSize(w, 7))
		v := m.View()
		assertFits(t, v, w, 7)
		if strings.Contains(v, "\t") {
			t.Errorf("view keeps a tab at %d", w)
		}
		for range 6 {
			m = keys(t, m, "right")
			assertFits(t, m.View(), w, 7)
		}
	}
	m := load(t, src, WithSize(80, 7))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "+tab    here 日本") || !strings.Contains(v, "     indented") {
		t.Errorf("tabs are not expanded to stops of four:\n%s", v)
	}
}

func TestViewFitsAnySize(t *testing.T) {
	m := load(t, threeFiles())
	for w := 1; w <= 90; w += 7 {
		for _, h := range []int{1, 2, 3, 12} {
			m.SetSize(w, h)
			for _, k := range []string{"", "G", "J", "enter", "}", "right", "g"} {
				if k != "" {
					m = keys(t, m, k)
				}
				assertFits(t, m.View(), w, h)
			}
		}
	}
}

func TestViewUnderTwentyColumns(t *testing.T) {
	m := load(t, &source{size: 5, files: []File{goFile("a/b/c.go", 2), {Path: "x.png", Status: StatusAdded}}}, WithSize(12, 10))
	for range 30 {
		m = keys(t, m, "down")
		assertFits(t, m.View(), 12, 10)
	}
}

func TestViewCleansHostileText(t *testing.T) {
	h := termtexttest.Hostile
	hostile := strings.ReplaceAll(h, "\n", " ")
	patch := fmt.Sprintf("@@ -1,2 +1,2 @@ %s\n %s\n-%s\n+%s", hostile, hostile, hostile, hostile)
	src := &source{size: 5, files: []File{
		{Path: h, OldPath: h + "\nold", Status: StatusRenamed, Additions: 1, Deletions: 1, Patch: patch},
		{Path: "raw.go", Status: StatusModified, Additions: 1, Patch: "junk " + hostile + "\n+x"},
	}}
	for _, w := range []int{20, 60, 100} {
		m := load(t, src, WithSize(w, 12))
		termtexttest.AssertClean(t, m.View(), w)
		for range 12 {
			m = keys(t, m, "down", "right")
			termtexttest.AssertClean(t, m.View(), w)
		}
	}
	failing := &source{size: 5, fail: fmt.Errorf("%s", h)}
	termtexttest.AssertClean(t, load(t, failing, WithSize(60, 4)).View(), 60)
}
