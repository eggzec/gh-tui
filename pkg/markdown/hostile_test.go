package markdown

import (
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// shape is a comment made to take a long time to render, as long as k times
// its base length.
type shape func(k int) string

// fixed is a shape that has one length.
func fixed(s string) shape { return func(int) string { return s } }

// hostile are comments made to take a long time to render.
var hostile = map[string]shape{
	"jsonata backslash":   fixed("```jsonata\n\\\n```"),
	"jsonata accent":      fixed("```jsonata\né\n```"),
	"jungle backslash":    fixed("```jungle\n\\\n```"),
	"jungle accent":       fixed("```jungle\né\n```"),
	"quotes":              func(k int) string { return strings.Repeat("> ", 80*k) + "x" },
	"quotes over lines":   func(k int) string { return strings.Repeat(">", 200*k) + " x\n" + strings.Repeat(">", 200*k) + " y" },
	"lists":               func(k int) string { return strings.Repeat("- ", 500*k) + "x" },
	"lists over lines":    func(k int) string { return listLines(150 * k) },
	"quoted lists":        func(k int) string { return strings.Repeat("> - ", 200*k) + "x" },
	"brackets":            func(k int) string { return strings.Repeat("[", 4000*k) },
	"links":               func(k int) string { return strings.Repeat("[a](", 1000*k) },
	"emphasis":            func(k int) string { return strings.Repeat("*a", 2000*k) },
	"emphasis mixed":      func(k int) string { return strings.Repeat("*_", 2000*k) },
	"backticks":           func(k int) string { return strings.Repeat("`a", 2000*k) },
	"angles":              func(k int) string { return strings.Repeat("<a ", 1300*k) },
	"references":          func(k int) string { return strings.Repeat("&#27;[1m", 500*k) },
	"table columns":       func(k int) string { return tableColumns(3000 * k) },
	"joined emoji":        func(k int) string { return strings.Repeat("\U0001F468\u200D", 500*k) },
	"table rows":          func(k int) string { return "|a|\n|-|\n" + strings.Repeat("|"+strings.Repeat("x|", 100)+"\n", 60*k) },
	"details":             func(k int) string { return strings.Repeat("<details><summary>s</summary>\n", 125*k) },
	"comments":            func(k int) string { return strings.Repeat("<!--", 1000*k) },
	"emoji":               func(k int) string { return strings.Repeat(":smile:", 570*k) },
	"combining":           func(k int) string { return "a" + strings.Repeat("́", 2000*k) },
	"long word":           func(k int) string { return strings.Repeat("x", 4000*k) },
	"fences":              func(k int) string { return strings.Repeat("```\n", 1000*k) },
	"images":              func(k int) string { return strings.Repeat("<img src=x alt=y>", 235*k) },
	"autolinks":           func(k int) string { return strings.Repeat("https://a.b/", 333*k) },
	"long code":           func(k int) string { return "```go\n" + strings.Repeat("x := `\\\n", 3000*k) + "```" },
	"indented code guess": func(k int) string { return "    " + strings.Repeat("#include <a>\n    ", 300*k) },
	"java":                func(k int) string { return "```java\n" + lines(7*k, strings.Repeat("a ", 255)) + "\n```" },
	"html":                func(k int) string { return "```html\n" + lines(7*k, strings.Repeat("<?", 255)) + "\n```" },
	"groovy":              func(k int) string { return "```groovy\n" + lines(97*k, strings.Repeat("a", 40)) + "\n```" },
	"quoted java": func(k int) string {
		return "> ```java\n" + lines(31*k, "> "+strings.Repeat("a ", 255)) + "\n> ```"
	},
	"indented templ": func(k int) string {
		return "text\n\n    templ x() {\n" + lines(31*k, "    "+strings.Repeat("<?", 240))
	},
	"css": func(k int) string { return "```css\n" + lines(7*k, strings.Repeat("{a:", 170)) + "\n```" },
}

// long are hostile comments as long as they come.
var long = map[string]shape{
	"many blocks":        func(k int) string { return strings.Repeat("```go\n"+strings.Repeat("x := 1; ", 20)+"\n```\n", 333*k) },
	"nested lines":       func(k int) string { return lines(999*k, strings.Repeat("> ", 8)+strings.Repeat("- ", 10)+"x") },
	"lists on each line": func(k int) string { return lines(999*k, strings.Repeat("- ", 10)+"x") },
	"long links":         func(k int) string { return strings.Repeat("[a](", 32<<10*k) },
	"long emphasis":      func(k int) string { return strings.Repeat("*a", 32<<10*k) },
	"long autolinks":     func(k int) string { return strings.Repeat("https://a.b/", 5<<10*k) },
	"long backticks":     func(k int) string { return strings.Repeat("`a", 32<<10*k) },
}

// lines returns n lines of l.
func lines(n int, l string) string {
	return strings.TrimSuffix(strings.Repeat(l+"\n", n), "\n")
}

// listLines nests a list n deep, a level a line.
func listLines(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString(strings.Repeat("  ", i) + "- x\n")
	}
	return b.String()
}

// tableColumns returns a table of n columns and a body row.
func tableColumns(n int) string {
	return strings.Repeat("|a", n) + "\n" + strings.Repeat("|-", n) + "\n" + strings.Repeat("|b", n)
}

// plainText returns plain text of n bytes, words and spaces.
func plainText(n int) string {
	const words = "lorem ipsum dolor sit amet, "
	return strings.Repeat(words, n/len(words)+1)[:n]
}

// Lexers that never finish are cut short by the real clock, so the render of
// a comment that reaches one still ends; nothing in the test is timed.
func TestHostileRendersSafely(t *testing.T) {
	wallClock(t)
	t.Cleanup(func() { idle(t) })
	for _, set := range []map[string]shape{hostile, long} {
		for name, src := range set {
			t.Run(name, func(t *testing.T) {
				out := New(DefaultStyle(true)).Render(src(1), 76)
				if why := unsafe(out); why != "" {
					t.Error(why)
				}
			})
		}
	}
}

// scaleLimit is how much more a render of four times the input may
// allocate: linear work gives about 4, and quadratic 16.
const scaleLimit = 8

// The work to render a comment grows with its length and not with the
// square of it. Allocations stand for the work, as they don't depend on how
// busy the machine is, as a time does.
func TestHostileRenderScales(t *testing.T) {
	wallClock(t)
	t.Cleanup(func() { idle(t) })
	for _, set := range []map[string]shape{hostile, long} {
		for name, src := range set {
			t.Run(name, func(t *testing.T) {
				allocs := func(k int) float64 {
					in := src(k)
					return testing.AllocsPerRun(1, func() {
						New(DefaultStyle(true)).Render(in, 76)
					})
				}
				small, large := allocs(1), allocs(4)
				if large > scaleLimit*small {
					t.Errorf("rendering 4 times the input allocates %.1f times as much (%.0f to %.0f), want at most %d times",
						large/small, small, large, scaleLimit)
				}
			})
		}
	}
}

// BenchmarkHostileRender times the render of each hostile comment, and of
// plain text of its length, which its time is to grow with and not with the
// square of its length. The render runs in Update, so the app waits for it.
func BenchmarkHostileRender(b *testing.B) {
	syntaxtest.Use(b, nil)
	// A lexer cut short that never finishes would keep the rest plain.
	b.Cleanup(func() { idle(b) })
	for _, set := range []map[string]shape{hostile, long} {
		for name, build := range set {
			src := build(1)
			b.Run(name, func(b *testing.B) {
				b.Run("hostile", func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						New(DefaultStyle(true)).Render(src, 76)
					}
				})
				b.Run("plain", func(b *testing.B) {
					base := plainText(len(src))
					b.ReportAllocs()
					for b.Loop() {
						New(DefaultStyle(true)).Render(base, 76)
					}
				})
			})
		}
	}
}

// idle waits for a lexer that was cut short to finish, so the tests after it
// highlight. One that never finishes holds the gate past any bound, so the
// wait fails the test when one was reached.
func idle(tb testing.TB) {
	tb.Helper()
	select {
	case lexing <- struct{}{}:
		<-lexing
	case <-time.After(slowdown * 10 * time.Second):
		tb.Fatal("a lexer that never finishes was reached: the gate is still held")
	}
}

// hostileCode is code that has made lexers loop or backtrack.
var hostileCode = []string{
	"\\", "é", "\\é", "'", "\"", "`", "/*", "<!--", "#", "$(", "${", "\\\\\\",
	strings.Repeat("(", 200), strings.Repeat("\"\\", 200), strings.Repeat("a", 5000),
	"x := \"&#27;[2J \x1b[5m\" // <script>\n#include <a>\n$var `cmd` 'str' 0x1F 1e9 /* c */ -- c\n@decorator\n\\x1b[31m\n\t\ttabs\n" +
		strings.Repeat("é́", 20),
}

// Every language that is highlighted finishes quickly on hostile code.
func TestHighlightedLexersFinish(t *testing.T) {
	wallClock(t)
	t.Cleanup(func() { idle(t) })
	r := New(DefaultStyle(true))
	for _, l := range lexers.GlobalLexerRegistry.Lexers {
		name := l.Config().Name
		if !highlighted[name] {
			continue
		}
		alias := name
		if a := l.Config().Aliases; len(a) > 0 {
			alias = a[0]
		}
		for _, code := range hostileCode {
			out := r.Render("```"+alias+"\n"+code+"\n```", 60)
			if why := unsafe(out); why != "" {
				t.Errorf("%s: %s", name, why)
			}
		}
	}
}
