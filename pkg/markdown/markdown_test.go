package markdown

import (
	"strconv"
	"strings"
	"testing"

	glamourstyles "charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// sample is a comment with what GitHub bodies hold: a template's comments
// and headings, lines broken by hand, a task list, a table, a quote,
// links, images, a collapsed section with code, and mentions.
const sample = "<!-- Thanks for the report! Fill in what you can. -->\r\n" +
	"### Describe the bug\r\n" +
	"Opening a pull request\r\n" +
	"with a long body crashes.\r\n\r\n" +
	"- [x] searched the issues\r\n" +
	"- [ ] tried main\r\n\r\n" +
	"| OS | Version |\r\n|---|---|\r\n| linux | 0.3.1 |\r\n\r\n" +
	"> It only happens\r\n> on narrow terminals.\r\n\r\n" +
	"See [the docs](https://example.com/docs) and #12, cc @mona.\r\n\r\n" +
	"![screenshot](https://example.com/shot.png)\r\n" +
	"<img width=\"300\" alt=\"logo\" src=\"https://example.com/logo.png\">\r\n\r\n" +
	"<details><summary>Stack trace</summary>\r\n\r\n" +
	"```go\r\nfunc main() {\r\n\tpanic(\"boom\")\r\n}\r\n```\r\n\r\n" +
	"</details>\r\n\r\n" +
	"```mermaid\r\ngraph TD\r\n  A-->B\r\n```\r\n"

func TestRender(t *testing.T) {
	for _, dark := range []bool{true, false} {
		name := "light"
		if dark {
			name = "dark"
		}
		for _, width := range []int{40, 76} {
			t.Run(name+"/"+strconv.Itoa(width), func(t *testing.T) {
				out := New(DefaultStyle(dark)).Render(sample, width)
				for i, l := range strings.Split(out, "\n") {
					if w := ansi.StringWidth(l); w > width {
						t.Errorf("line %d is %d cells wide, more than %d: %q", i+1, w, width, l)
					}
				}
				golden.RequireEqual(t, out)
			})
		}
	}
}

func TestRenderShows(t *testing.T) {
	out := ansi.Strip(New(DefaultStyle(true)).Render(sample, 76))
	for _, want := range []string{
		"Describe the bug\n",
		"Opening a pull request\nwith a long body crashes.",
		"[✓] searched the issues",
		"│ on narrow terminals.",
		"the docs https://example.com/docs",
		"#12, cc @mona.",
		"🖼 screenshot (https://example.com/shot.png)",
		"🖼 logo (https://example.com/logo.png)",
		"▸ Stack trace",
		"panic(\"boom\")",
		"◆ flowchart · 2 lines · View diagram ↗",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the render lacks %q:\n%s", want, out)
		}
	}
	for _, hidden := range []string{"Thanks for the report", "###", "<details>", "<img", "\x1b]8;", "graph TD"} {
		if strings.Contains(out, hidden) {
			t.Errorf("the render shows %q:\n%s", hidden, out)
		}
	}
}

func TestRenderNeutralisesEscapes(t *testing.T) {
	out := New(DefaultStyle(true)).Render("title \x1b]0;pwned\x07 and \x1b[2J clear", 40)
	if strings.Contains(out, "\x07") || strings.Contains(out, "\x1b]") || strings.Contains(out, "\x1b[2J") {
		t.Errorf("the render passes escape sequences through: %q", out)
	}
}

// A control in the name of a link, which glamour decodes from a character
// reference, can't break the link's hyperlink and show its address.
func TestRenderKeepsLinksWhole(t *testing.T) {
	out := ansi.Strip(New(DefaultStyle(true)).Render("[x&#27;y&#x1b;z](https://x.test/a)", 60))
	if strings.Contains(out, "]8;") || strings.Contains(out, "id=") {
		t.Errorf("the link's hyperlink shows: %q", out)
	}
}

// A source too long to show in full ends with a note that offers the
// hint, and a new hint renders it again.
func TestRenderCutOffersHint(t *testing.T) {
	src := strings.Repeat("line\n\n", 600)
	r := New(DefaultStyle(true))
	if out := ansi.Strip(r.Render(src, 76)); !strings.HasSuffix(out, "⋯ The rest is too long to show here") {
		t.Errorf("the cut note ends %q", out[max(len(out)-60, 0):])
	}
	r.SetHint("o to open on GitHub")
	if out := ansi.Strip(r.Render(src, 76)); !strings.HasSuffix(out, "⋯ The rest is too long to show here · o to open on GitHub") {
		t.Errorf("the cut note ends %q", out[max(len(out)-60, 0):])
	}
	if r.Renders() != 2 {
		t.Errorf("rendered %d times, want again for the new hint", r.Renders())
	}
	// A hint shows as it is, markdown or not.
	r.SetHint("ctrl+_ to open *[it](x)*")
	if out := ansi.Strip(r.Render(src, 76)); !strings.HasSuffix(out, "· ctrl+_ to open *[it](x)*") {
		t.Errorf("the cut note ends %q", out[max(len(out)-60, 0):])
	}
}

func TestRenderEmpty(t *testing.T) {
	r := New(DefaultStyle(true))
	for _, tt := range []struct {
		src   string
		width int
	}{{"", 40}, {" \n\t\n", 40}, {"text", 0}, {"text", -1}} {
		if out := r.Render(tt.src, tt.width); out != "" {
			t.Errorf("Render(%q, %d) = %q, want nothing", tt.src, tt.width, out)
		}
	}
	if r.Renders() != 0 {
		t.Errorf("rendering nothing ran glamour %d times", r.Renders())
	}
}

func TestRenderCache(t *testing.T) {
	r := New(DefaultStyle(true))
	first := r.Render(sample, 60)
	if r.Render(sample, 60) != first || r.Renders() != 1 {
		t.Fatalf("the same source at the same width rendered %d times", r.Renders())
	}
	r.Render(sample, 50)
	if r.Renders() != 2 {
		t.Fatalf("a new width rendered %d times in all, want 2", r.Renders())
	}
	r.Render(sample, 60)
	if r.Renders() != 2 {
		t.Fatalf("going back to a width rendered again")
	}
	r.SetStyle(DefaultStyle(false))
	if r.Render(sample, 60) == first || r.Renders() != 3 {
		t.Fatalf("a new style didn't render again")
	}
}

func TestRenderCacheKeepsRecent(t *testing.T) {
	r := New(glamourstyles.ASCIIStyleConfig)
	for i := range 3 * keep {
		r.Render("comment "+strconv.Itoa(i), 40)
		// The first comment is read on every frame, so it stays.
		r.Render("comment 0", 40)
	}
	if r.Renders() != 3*keep {
		t.Errorf("rendered %d times, want %d", r.Renders(), 3*keep)
	}
	if n := len(r.recent) + len(r.older); n > 2*keep {
		t.Errorf("the cache holds %d renders, more than %d", n, 2*keep)
	}
}

func TestIndent(t *testing.T) {
	if got := Indent("a\nb", "  "); got != "  a\n  b" {
		t.Errorf("Indent = %q", got)
	}
	if got := Indent("", "  "); got != "" {
		t.Errorf("Indent of nothing = %q", got)
	}
}

func BenchmarkRender(b *testing.B) {
	r := New(DefaultStyle(true))
	b.ReportAllocs()
	for b.Loop() {
		r.SetStyle(DefaultStyle(true))
		r.Render(sample, 76)
	}
}

func BenchmarkRenderCached(b *testing.B) {
	r := New(DefaultStyle(true))
	r.Render(sample, 76)
	b.ReportAllocs()
	for b.Loop() {
		r.Render(sample, 76)
	}
}

// Single newlines inside a paragraph stay line breaks, as GitHub shows
// them in issue and pull request bodies, which templates rely on; stock
// markdown would join the lines into one.
func TestSoftLineBreaksKept(t *testing.T) {
	src := "**OS:** linux\nVersion: 0.3.1\nShell: bash\n\nSecond paragraph\nwith two lines."
	got := ansi.Strip(New(DefaultStyle(true)).Render(src, 76))
	var lines []string
	for l := range strings.SplitSeq(got, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	want := []string{"OS: linux", "Version: 0.3.1", "Shell: bash", "Second paragraph", "with two lines."}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("rendered lines %q, want %q", lines, want)
	}
}
