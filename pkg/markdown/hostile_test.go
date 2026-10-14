package markdown

import (
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// hostile are comments made to take a long time to render.
var hostile = map[string]string{
	"jsonata backslash":   "```jsonata\n\\\n```",
	"jsonata accent":      "```jsonata\né\n```",
	"jungle backslash":    "```jungle\n\\\n```",
	"jungle accent":       "```jungle\né\n```",
	"quotes":              strings.Repeat("> ", 80) + "x",
	"quotes over lines":   strings.Repeat(">", 200) + " x\n" + strings.Repeat(">", 200) + " y",
	"lists":               strings.Repeat("- ", 500) + "x",
	"lists over lines":    listLines(150),
	"quoted lists":        strings.Repeat("> - ", 200) + "x",
	"brackets":            strings.Repeat("[", 4000),
	"links":               strings.Repeat("[a](", 1000),
	"emphasis":            strings.Repeat("*a", 2000),
	"emphasis mixed":      strings.Repeat("*_", 2000),
	"backticks":           strings.Repeat("`a", 2000),
	"angles":              strings.Repeat("<a ", 1300),
	"references":          strings.Repeat("&#27;[1m", 500),
	"table columns":       strings.Repeat("|a", 3000) + "\n" + strings.Repeat("|-", 3000) + "\n" + strings.Repeat("|b", 3000),
	"joined emoji":        strings.Repeat("\U0001F468\u200d", 500),
	"table rows":          "|a|\n|-|\n" + strings.Repeat("|"+strings.Repeat("x|", 100)+"\n", 60),
	"details":             strings.Repeat("<details><summary>s</summary>\n", 125),
	"comments":            strings.Repeat("<!--", 1000),
	"emoji":               strings.Repeat(":smile:", 570),
	"combining":           "a" + strings.Repeat("́", 2000),
	"long word":           strings.Repeat("x", 4000),
	"fences":              strings.Repeat("```\n", 1000),
	"images":              strings.Repeat("<img src=x alt=y>", 235),
	"autolinks":           strings.Repeat("https://a.b/", 333),
	"long code":           "```go\n" + strings.Repeat("x := `\\\n", 3000) + "```",
	"indented code guess": "    " + strings.Repeat("#include <a>\n    ", 300),
	"java":                "```java\n" + lines(7, strings.Repeat("a ", 255)) + "\n```",
	"html":                "```html\n" + lines(7, strings.Repeat("<?", 255)) + "\n```",
	"groovy":              "```groovy\n" + lines(97, strings.Repeat("a", 40)) + "\n```",
	"quoted java":         "> ```java\n" + lines(31, "> "+strings.Repeat("a ", 255)) + "\n> ```",
	"indented templ":      "text\n\n    templ x() {\n" + lines(31, "    "+strings.Repeat("<?", 240)),
	"css":                 "```css\n" + lines(7, strings.Repeat("{a:", 170)) + "\n```",
}

// long are hostile comments as long as they come.
var long = map[string]string{
	"many blocks":        strings.Repeat("```go\n"+strings.Repeat("x := 1; ", 20)+"\n```\n", 333),
	"nested lines":       lines(999, strings.Repeat("> ", 8)+strings.Repeat("- ", 10)+"x"),
	"lists on each line": lines(999, strings.Repeat("- ", 10)+"x"),
	"long links":         strings.Repeat("[a](", 32<<10),
	"long emphasis":      strings.Repeat("*a", 32<<10),
	"long autolinks":     strings.Repeat("https://a.b/", 5<<10),
	"long backticks":     strings.Repeat("`a", 32<<10),
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

// plainText returns plain text of n bytes, words and spaces.
func plainText(n int) string {
	const words = "lorem ipsum dolor sit amet, "
	return strings.Repeat(words, n/len(words)+1)[:n]
}

func TestHostileRendersSafely(t *testing.T) {
	wallClock(t)
	t.Cleanup(func() { idle(t) })
	for _, set := range []map[string]string{hostile, long} {
		for name, src := range set {
			t.Run(name, func(t *testing.T) {
				out := New(DefaultStyle(true)).Render(src, 76)
				if why := unsafe(out); why != "" {
					t.Error(why)
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
	for _, set := range []map[string]string{hostile, long} {
		for name, src := range set {
			b.Run(name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					New(DefaultStyle(true)).Render(src, 76)
				}
			})
			b.Run(name+"/plain", func(b *testing.B) {
				base := plainText(len(src))
				b.ReportAllocs()
				for b.Loop() {
					New(DefaultStyle(true)).Render(base, 76)
				}
			})
		}
	}
}

// idle waits for a lexer that overran to finish, so the tests after it
// highlight.
func idle(t *testing.T) {
	t.Helper()
	select {
	case lexing <- struct{}{}:
		<-lexing
	case <-time.After(slowdown * 10 * time.Second):
		t.Fatal("a lexer still runs")
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
