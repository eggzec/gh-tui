package markdown

import (
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"
)

// settle waits a while for the goroutines to come down to n, and returns
// how many run.
func settle(n int) int {
	deadline := time.Now().Add(slowdown * 5 * time.Second)
	for runtime.NumGoroutine() > n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	return runtime.NumGoroutine()
}

// Code whose lexer would hand it to a lexer the code names, such as a
// fence in markdown, never reaches the lexers that never finish, at any
// width.
func TestDelegatedCodeRendersQuickly(t *testing.T) {
	for name, src := range map[string]string{
		"markdown":   "~~~markdown\n```jsonata\n\\\n```\n~~~",
		"postgresql": "```postgresql\nDO LANGUAGE jsonata $$\\$$;\n```",
		"postgres":   "```postgres\n$f$\\$f$ LANGUAGE jungle\n```",
		"http":       "```http\nHTTP/1.1 200 OK\nContent-Type: text/x-jungle\n\n\\\n```",
	} {
		t.Run(name, func(t *testing.T) {
			before := runtime.NumGoroutine()
			r := New(DefaultStyle(true))
			for w := 80; w > 72; w-- {
				start := time.Now()
				out := r.Render(src, w)
				if d := time.Since(start); d > hostileLimit {
					t.Errorf("took %v at %d cells, more than %v", d, w, hostileLimit)
				}
				if !strings.Contains(ansi.Strip(out), `\`) {
					t.Errorf("the code doesn't show at %d cells:\n%s", w, out)
				}
			}
			if n := settle(before); n > before {
				t.Errorf("%d goroutines run after the renders, %d before", n, before)
			}
		})
	}
}

// shapes returns the fenced block of code in lang in each container
// markdown has, and code indented and in <pre>.
func shapes(lang, code string) map[string]string {
	block := "```" + lang + "\n" + code + "\n```"
	in := func(first, rest string) string {
		return first + strings.ReplaceAll(block, "\n", "\n"+rest)
	}
	return map[string]string{
		"top":           block,
		"quote":         in("> ", "> "),
		"dash":          in("- ", "  "),
		"ordered":       in("1. ", "   "),
		"plus":          in("+ ", "  "),
		"star tilde":    "* ~~~" + lang + "\n  " + code + "\n  ~~~",
		"quoted list":   in("> - ", ">   "),
		"tab quote":     in(">\t", ">\t"),
		"nested":        in("> > > > ", "> > > > "),
		"deep list":     in("- - - ", "      "),
		"indented list": "- a\n\n  - b\n\n        " + strings.ReplaceAll(block, "\n", "\n        "),
		"indented":      "text\n\n    " + strings.ReplaceAll(code, "\n", "\n    "),
		"pre":           "<pre>\n" + block + "\n</pre>",
	}
}

// spy is a lexer that counts what it is asked, and would be chroma's
// guess for any code.
type spy struct {
	tokenised, analysed atomic.Int32
}

func (s *spy) Config() *chroma.Config {
	return &chroma.Config{Name: "Spy", Aliases: []string{"spy"}}
}

func (s *spy) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	s.tokenised.Add(1)
	return chroma.Literator(), nil
}

func (s *spy) SetRegistry(*chroma.LexerRegistry) chroma.Lexer { return s }
func (s *spy) SetAnalyser(func(string) float32) chroma.Lexer  { return s }

func (s *spy) AnalyseText(string) float32 {
	s.analysed.Add(1)
	return 1
}

// Code reaches chroma only through the gate, in whatever container: no
// lexer it doesn't highlight runs, and none is guessed.
func TestCodeReachesChromaOnlyThroughTheGate(t *testing.T) {
	s := &spy{}
	lexers.GlobalLexerRegistry.Register(s)
	r := New(DefaultStyle(true))
	for name, src := range shapes("spy", "x := 1") {
		for _, w := range []int{80, 40} {
			r.Render(src, w)
		}
		if n, m := s.tokenised.Load(), s.analysed.Load(); n != 0 || m != 0 {
			t.Fatalf("%s: chroma ran the lexer %d times and guessed %d times", name, n, m)
		}
	}
}

// Code shows in every container, highlighted or not, and what stands for
// highlighted code while glamour renders never shows.
func TestCodeShowsInEveryContainer(t *testing.T) {
	r := New(DefaultStyle(true))
	cases := shapes("go", "x := 1")
	cases["html block"] = "<table>\n```go\nx := 1\n```\n</table>"
	cases["paragraph"] = "text\n    ```go\n    x := 1\n    ```"
	for name, src := range cases {
		out := r.Render(src, 40)
		if strings.Contains(out, r.nonce) {
			t.Errorf("%s shows a mark:\n%s", name, out)
		}
		if !strings.Contains(ansi.Strip(out), "x := 1") {
			t.Errorf("%s doesn't show the code:\n%s", name, out)
		}
	}
	if out := r.Render(cases["top"], 40); !strings.Contains(out, "\x1b[38;2;") {
		t.Errorf("code at the top isn't highlighted: %q", out)
	}
}

// A highlighted block in a list item ends where its fence does: what
// follows it still renders as markdown.
func TestHighlightedBlockInAListEnds(t *testing.T) {
	after := "\n\nafter **bold**\n\n# Heading"
	for name, src := range map[string]string{
		"continuation": "- item\n\n  ```go\n  x := 1\n  ```" + after,
		"ordered":      "1. first\n\n   ```go\n   x := 1\n   ```\n2. next" + after,
		"tight":        "- item\n  ```go\n  x := 1\n  ```\n- other" + after,
		"nested":       "- a\n  - b\n\n    ```go\n    x := 1\n    ```" + after,
	} {
		r := New(DefaultStyle(true))
		out := r.Render(src, 60)
		text := ansi.Strip(out)
		if !strings.Contains(text, "after bold") || strings.Contains(text, "**") {
			t.Errorf("%s: the text after the block isn't markdown:\n%s", name, text)
		}
		if strings.Contains(text, "# Heading") || !strings.Contains(out, "Heading") {
			t.Errorf("%s: the heading after the block isn't one:\n%s", name, text)
		}
		if name == "ordered" && !strings.Contains(text, "2. next") {
			t.Errorf("%s: the next item isn't one:\n%s", name, text)
		}
		if name != "nested" && !strings.Contains(out, "\x1b[38;2;") {
			t.Errorf("%s: the block isn't highlighted: %q", name, out)
		}
	}
}

// Highlighted code is cut at the width, however little room is left,
// wide characters too.
func TestHighlightedCodeFitsTheWidth(t *testing.T) {
	r := New(DefaultStyle(true))
	// A short mark, which glamour doesn't wrap even at a few cells.
	r.nonce = "m"
	spliced := false
	for w := 1; w <= 8; w++ {
		for i, l := range strings.Split(r.Render("```go\n世界 := 1\n```", w), "\n") {
			if !strings.Contains(l, "\x1b[38;2;") {
				continue
			}
			spliced = true
			if n := ansi.StringWidth(l); n > w {
				t.Errorf("line %d at %d cells is %d wide: %q", i+1, w, n, l)
			}
		}
	}
	if !spliced {
		t.Error("the code is never highlighted")
	}
}

// Code a lexer never finishes on renders quickly in any container, at
// any width, and leaves nothing running.
func TestContainedCodeRendersQuickly(t *testing.T) {
	for _, lang := range []string{"jsonata", "jungle"} {
		for name, src := range shapes(lang, `\`) {
			t.Run(lang+"/"+name, func(t *testing.T) {
				before := runtime.NumGoroutine()
				r := New(DefaultStyle(true))
				for w := 80; w > 76; w-- {
					start := time.Now()
					r.Render(src, w)
					if d := time.Since(start); d > hostileLimit {
						t.Errorf("took %v at %d cells, more than %v", d, w, hostileLimit)
					}
				}
				if n := settle(before); n > before {
					t.Errorf("%d goroutines run after the renders, %d before", n, before)
				}
			})
		}
	}
}

// stuck is a lexer that runs until release is closed, as one that never
// finishes does, and counts its runs.
type stuck struct {
	chroma.Lexer
	release chan struct{}
	runs    atomic.Int32
}

func (l *stuck) Tokenise(*chroma.TokeniseOptions, string) (chroma.Iterator, error) {
	l.runs.Add(1)
	<-l.release
	return chroma.Literator(), nil
}

// A lexer that overruns goes on alone: while it runs, code shows plain
// without another starting, and the code it overran on stays plain.
func TestOverrunLexerRunsAlone(t *testing.T) {
	idle(t)
	before := runtime.NumGoroutine()
	l := &stuck{Lexer: lexers.Get("go"), release: make(chan struct{})}
	for _, code := range []string{"overran", "other", "overran"} {
		if _, ok := tokens(l, code, newBudget()); ok {
			t.Errorf("%q highlights while the lexer overruns", code)
		}
	}
	if n := l.runs.Load(); n != 1 {
		t.Errorf("the lexer ran %d times, want once", n)
	}
	if n := runtime.NumGoroutine(); n != before+1 {
		t.Errorf("%d goroutines run, want the lexer's beside the %d before", n, before)
	}
	close(l.release)
	if n := settle(before); n > before {
		t.Fatalf("%d goroutines run after the lexer ended, %d before", n, before)
	}
	// Another goroutine may have ended first, while the lexer's still
	// held its token.
	idle(t)
	if _, ok := tokens(l, "overran", newBudget()); ok {
		t.Error("the code the lexer overran on highlights")
	}
	if _, ok := tokens(l, "other", newBudget()); !ok {
		t.Error("other code doesn't highlight once the lexer ended")
	}
	if n := l.runs.Load(); n != 2 {
		t.Errorf("the lexer ran %d times, want once more for the other code", n)
	}
}

// A render highlights at most maxHighlightedBlocks blocks, lexing for at
// most renderLexLimit, and the blocks after show plain.
func TestHighlightingIsBounded(t *testing.T) {
	idle(t)
	golang := lexer("go", "x := 1")
	b := &budget{time: time.Hour, blocks: maxHighlightedBlocks}
	n := 0
	for range 2 * maxHighlightedBlocks {
		if _, ok := tokens(golang, "x := 1", b); ok {
			n++
		}
	}
	if n != maxHighlightedBlocks {
		t.Errorf("%d blocks are highlighted, want %d", n, maxHighlightedBlocks)
	}
	if _, ok := tokens(golang, "x := 1", &budget{blocks: 1}); ok {
		t.Error("a block past the time a render has is highlighted")
	}
}
