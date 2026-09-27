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
		if check(l, code, newBudget()) {
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
	if check(l, "overran", newBudget()) {
		t.Error("the code the lexer overran on highlights")
	}
	if !check(l, "other", newBudget()) {
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
	b := &budget{time: time.Hour, blocks: maxHighlightedBlocks}
	n := 0
	for range 2 * maxHighlightedBlocks {
		if highlightAs("go", "x := 1", b) == "go" {
			n++
		}
	}
	if n != maxHighlightedBlocks {
		t.Errorf("%d blocks are highlighted, want %d", n, maxHighlightedBlocks)
	}
	if got := highlightAs("go", "x := 1", &budget{blocks: 1}); got != plain {
		t.Errorf("a block past the time a render has shows as %q", got)
	}
	n = 0
	for _, b := range Blocks(strings.Repeat("```go\nx := 1\n```\n\n", 2*maxHighlightedBlocks)) {
		if strings.HasPrefix(b.Full, "```go") {
			n++
		}
	}
	if n == 0 || n > maxHighlightedBlocks {
		t.Errorf("%d blocks of a source are highlighted, want 1 to %d", n, maxHighlightedBlocks)
	}
}
