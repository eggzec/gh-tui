package diff

import (
	"image/color"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// openComment is a hunk whose old side opens a comment that a context line
// closes, so that the deleted line is a comment, and the added line is code
// only when the sides are lexed apart.
const openComment = "@@ -1,3 +1,3 @@\n package p\n-/* old\n+var x = 1\n */ var y = 2"

func fileOf(path, patch string) File {
	return File{Path: path, Status: StatusModified, Additions: 1, Deletions: 1, Patch: patch}
}

// spansOfRow returns the spans of body row i of file 0.
func spansOfRow(m Model, i int) []syntax.Span {
	return m.hl.spans[0].row(i)
}

// lexed reports whether file f has tokens.
func lexed(m Model, f int) bool { return m.hl.spans[f] != nil }

func highlighted(t *testing.T, f File, opts ...Option) Model {
	t.Helper()
	syntaxtest.Use(t, syntaxtest.Stopped{})
	return load(t, &source{size: 5, files: []File{f}}, opts...)
}

func firstType(spans []syntax.Span) chroma.TokenType {
	if len(spans) == 0 {
		return chroma.Error
	}
	return spans[0].Type
}

func TestEachSideIsLexedOnItsOwn(t *testing.T) {
	m := highlighted(t, fileOf("p.go", openComment))
	// Body rows: hunk header, context, deleted, added, context.
	if got := firstType(spansOfRow(m, 2)); !got.InCategory(chroma.Comment) {
		t.Errorf("the deleted line starts with %v, want a comment from the old side", got)
	}
	if got := firstType(spansOfRow(m, 3)); got != chroma.Keyword && got != chroma.KeywordDeclaration {
		t.Errorf("the added line starts with %v, want a keyword from the new side", got)
	}
	// The old side reads " */ var y" as the end of a comment, the new one as
	// code, and a context line takes the tokens of the new.
	ctx := spansOfRow(m, 4)
	if len(ctx) == 0 || ctx[0].Type.InCategory(chroma.Comment) {
		t.Errorf("the context line has the spans %v, want code from the new side", ctx)
	}
	if !slices.ContainsFunc(ctx, func(s syntax.Span) bool { return s.Type.InCategory(chroma.Keyword) }) {
		t.Errorf("the context line has the spans %v, want a keyword", ctx)
	}
}

func TestColoursShowInTheView(t *testing.T) {
	m := highlighted(t, fileOf("p.go", openComment), WithSize(60, 8))
	v := m.View()
	code := m.wrap.code[codeAdded]
	if want := code.token(chroma.KeywordDeclaration).on("var"); !strings.Contains(v, want) {
		t.Errorf("the added line lacks %q:\n%q", want, v)
	}
	assertFits(t, v, 60, 8)
}

func TestWhichPathDecidesTheLanguage(t *testing.T) {
	tests := []struct {
		name string
		file File
		want bool
	}{
		{"modified", fileOf("a.go", openComment), true},
		{"unknown language", fileOf("a.nosuchlanguage", openComment), false},
		{"renamed takes the new path", File{Path: "new.go", OldPath: "old.nosuchlanguage", Status: StatusRenamed, Patch: openComment}, true},
		{"renamed away from code", File{Path: "new.nosuchlanguage", OldPath: "old.go", Status: StatusRenamed, Patch: openComment}, false},
		{"deleted takes the old path", File{Path: "x.nosuchlanguage", OldPath: "x.go", Status: StatusRemoved, Patch: openComment}, true},
		{"deleted by its path", File{Path: "x.go", Status: StatusRemoved, Patch: openComment}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := highlighted(t, tt.file)
			if got := lexed(m, 0); got != tt.want {
				t.Errorf("highlighted = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStaleHighlightIsDropped(t *testing.T) {
	m := highlighted(t, fileOf("p.go", openComment))
	got := m.hl.spans[0]
	if got == nil {
		t.Fatal("the file was not highlighted")
	}
	forged := newFileSpans([][]syntax.Span{{{End: 3, Type: chroma.Keyword}}})
	stale := highlightMsg{id: m.id, gen: m.hl.gen, file: 0, spans: forged}
	m.SetTabWidth(2)
	if len(m.hl.spans) != 0 {
		t.Fatal("a new tab width kept the tokens of the old one")
	}
	// The view lexes again for the new width.
	m = run(t, m, m.sync())
	if len(m.hl.spans[0].all) != len(got.all) {
		t.Fatal("the file was not lexed again")
	}
	m, _ = m.Update(stale)
	if len(m.hl.spans[0].all) != len(got.all) {
		t.Error("the tokens of an older generation were taken")
	}
	other := highlightMsg{id: m.id + 1, gen: m.hl.gen, file: 0, spans: forged}
	m, _ = m.Update(other)
	if len(m.hl.spans[0].all) != len(got.all) {
		t.Error("the tokens for another view were taken")
	}
	cur := highlightMsg{id: m.id, gen: m.hl.gen, file: 0, spans: forged}
	m, _ = m.Update(cur)
	if got := m.hl.spans[0]; len(got.all) != 1 {
		t.Errorf("the tokens of the current generation were not taken: %v", got.all)
	}
}

func TestOverALimitIsPlain(t *testing.T) {
	t.Run("a file over the byte limit", func(t *testing.T) {
		var b strings.Builder
		n := 100
		b.WriteString("@@ -0,0 +1,100 @@\n")
		for range n {
			b.WriteString("+var x = 12345678\n")
		}
		patch := strings.TrimSuffix(b.String(), "\n")
		if m := highlighted(t, fileOf("big.go", patch), WithHighlightLimit(n*16-1)); len(m.hl.spans) != 0 {
			t.Error("a file over the limit was lexed")
		}
		if m := highlighted(t, fileOf("big.go", patch), WithHighlightLimit(n*16)); len(m.hl.spans) != 1 {
			t.Error("a file within the limit was not lexed")
		}
		if m := highlighted(t, fileOf("big.go", patch), WithHighlightLimit(0)); len(m.hl.spans) != 0 {
			t.Error("a view with highlighting off lexed")
		}
	})
	t.Run("a line over the line limit", func(t *testing.T) {
		long := "+var s = \"" + strings.Repeat("x", syntax.MaxLexedLine) + "\""
		m := highlighted(t, fileOf("p.go", "@@ -0,0 +1,2 @@\n+var a = 1\n"+long))
		if len(spansOfRow(m, 1)) == 0 {
			t.Error("a short line next to a long one was not lexed")
		}
		if got := spansOfRow(m, 2); len(got) != 0 {
			t.Errorf("a line over the limit has spans: %v", got)
		}
	})
	t.Run("no syntax style", func(t *testing.T) {
		st := DefaultStyles(true)
		st.Syntax = nil
		m := highlighted(t, fileOf("p.go", openComment), WithStyles(st))
		if len(m.hl.spans) != 0 || len(m.hl.asked) != 0 {
			t.Error("a view without a syntax style lexed")
		}
	})
}

// Colours never change what shows: the text is the same cut, scrolled and
// cleaned as when plain.
func TestColouredTextIsTheSameText(t *testing.T) {
	patch := "@@ -1,3 +1,3 @@\n \tx := \"tab\"\n-var a = \"\x1b[31mred\x1b[0m\" // old\n+var b = \"wide 你好\" // a longer comment than the room there is\n func f() {}"
	f := fileOf("p.go", patch)
	plain := DefaultStyles(true)
	plain.Syntax = nil
	for _, width := range []int{80, 30, 12} {
		on := highlighted(t, f, WithSize(width, 8), WithFocused(true))
		off := highlighted(t, f, WithSize(width, 8), WithFocused(true), WithStyles(plain))
		if len(on.hl.spans) == 0 {
			t.Fatal("not highlighted")
		}
		for i := range 6 {
			if got, want := ansi.Strip(on.View()), ansi.Strip(off.View()); got != want {
				t.Errorf("width %d, scrolled %d: coloured text differs from plain:\n%s\nwant\n%s", width, i, got, want)
			}
			assertFits(t, on.View(), width, 8)
			on, off = keys(t, on, "l"), keys(t, off, "l")
		}
	}
}

func TestViewHighlight(t *testing.T) {
	m := highlighted(t, File{Path: "main.go", Status: StatusModified, Additions: 4, Deletions: 3, Patch: strings.Join([]string{
		"@@ -1,8 +1,9 @@ package main",
		" import \"fmt\"",
		" ",
		"-// hello greets the world.",
		"-func hello(name string) {",
		"-\tfmt.Println(\"hello\", name)",
		"+/* hello greets the world,",
		"+   and says so. */",
		"+func hello(name string, n int) {",
		"+\tfmt.Println(\"hello\", name, n+1)",
		" }",
		" ",
		" var _ = 3.14",
	}, "\n")}, WithSize(64, 16), WithStyles(tintedStyles()))
	v := m.View()
	assertFits(t, v, 64, 16)
	golden.RequireEqual(t, v)
}

// tintedStyles are the dark styles with a background behind the added and
// deleted code, as a theme gives.
func tintedStyles() Styles {
	st := DefaultStyles(true)
	st.AddedText = st.AddedText.Background(lipglossColor("#16301f"))
	st.DeletedText = st.DeletedText.Background(lipglossColor("#3a1c1f"))
	return st
}

func lipglossColor(s string) color.Color { return lipgloss.Color(s) }

func TestBusyLexerIsAskedAgain(t *testing.T) {
	m := highlighted(t, fileOf("p.go", openComment))
	m.SetTabWidth(2) // forget the tokens
	m.sync()
	if !m.hl.asked[0] {
		t.Fatal("the file was not asked for")
	}
	m, _ = m.Update(highlightMsg{id: m.id, gen: m.hl.gen, file: 0, busy: true})
	if m.hl.asked[0] || lexed(m, 0) {
		t.Fatal("a busy lexer left the file asked for")
	}
	m = run(t, m, m.sync())
	if !lexed(m, 0) {
		t.Error("the file was not lexed once the lexer was free")
	}
}

func TestFilesFarFromTheWindowAreForgotten(t *testing.T) {
	syntaxtest.Use(t, syntaxtest.Stopped{})
	m := load(t, &source{size: 40, files: sizedFiles(40, 3)})
	if !lexed(m, 0) {
		t.Fatal("the first file was not lexed")
	}
	m = keys(t, m, "G")
	if lexed(m, 0) || m.hl.asked[0] {
		t.Error("the tokens of a file far from the window were kept")
	}
	if !lexed(m, 39) {
		t.Error("the last file was not lexed")
	}
	m = keys(t, m, "g")
	if !lexed(m, 0) || lexed(m, 39) {
		t.Error("the first file was not lexed again, or the last one kept")
	}
}

func TestLexedWhenItComesIntoView(t *testing.T) {
	syntaxtest.Use(t, syntaxtest.Stopped{})
	t.Run("unfolded", func(t *testing.T) {
		m := load(t, &source{size: 5, files: sizedFiles(1, 2)}, WithCollapseOver(1))
		if lexed(m, 0) {
			t.Fatal("a folded file was lexed")
		}
		m.layout.SetCollapsed(0, false)
		m = run(t, m, m.sync())
		if !lexed(m, 0) {
			t.Error("an unfolded file was not lexed")
		}
	})
	t.Run("a new page", func(t *testing.T) {
		m := load(t, &source{size: 2, files: sizedFiles(6, 3)}, WithSize(80, 8))
		if lexed(m, 5) {
			t.Fatal("a file of a page not yet fetched was lexed")
		}
		m = keys(t, m, "G", "G", "G")
		if !lexed(m, 5) {
			t.Error("a file of a new page was not lexed")
		}
	})
	t.Run("a resize", func(t *testing.T) {
		m := load(t, &source{size: 10, files: sizedFiles(4, 1)}, WithSize(80, 6))
		if lexed(m, 3) {
			t.Fatal("a file out of the window was lexed")
		}
		m.SetSize(80, 40)
		m = run(t, m, m.sync())
		if !lexed(m, 3) {
			t.Error("a file that the new size shows was not lexed")
		}
	})
}
