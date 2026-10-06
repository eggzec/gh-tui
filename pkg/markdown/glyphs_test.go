package markdown

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// everything is markdown in ASCII alone that holds every construct the
// renderer draws glyphs of its own for.
var everything = "# A title long enough to wrap at the narrowest width\n\n" +
	"## Lists\n\n" +
	"- one\n  - nested\n    - deeper\n- two\n\n" +
	"1. first\n2. second\n\n" +
	"- [x] done\n- [ ] to do\n- [X](https://x.test/share)\n\n" +
	"Term\n: what the term means\n\n" +
	"| Name | Link | Badge |\n|---|---|---|\n" +
	"| a | [docs](https://example.com/" + strings.Repeat("long/", 30) + "end) | ![b](https://x.test/b.png) |\n" +
	"| b | <https://example.com/x> | plain |\n\n" +
	"> A quote\n> > nested in it\n\n" +
	"> [!NOTE]\n> a note\n\n> [!TIP]\n> a tip\n\n> [!IMPORTANT]\n> important\n\n" +
	"> [!WARNING]\n> a warning\n\n> [!CAUTION]\n> a caution\n\n" +
	"![screenshot](https://example.com/shot.png)\n\n" +
	"<img alt=\"logo\" src=\"https://example.com/logo.png\">\n\n" +
	"[![build](https://x.test/b.svg)](https://x.test/ci) and [![](https://x.test/c.svg)](https://x.test/c)\n\n" +
	"<details><summary>Stack trace</summary>\n\n```go\nfunc main() {\n\tpanic(\"boom\")\n}\n```\n\n</details>\n\n" +
	"<details>\n<summary>\nOn lines\n</summary>\n\nbody\n\n</details>\n\n" +
	"```mermaid\ngraph TD\n  A-->B\n```\n\n" +
	"```mermaid\ngraph TD\n" + strings.Repeat("  A-->B\n", 900) + "```\n\n" +
	"> ```mermaid\n> graph LR\n>   C-->D\n> ```\n\n" +
	"A note[^1] and another[^two], :tada: and ~~struck~~ text.\n\n" +
	"[^1]: The first.\n[^two]: The second.\n\n" +
	"---\n\n" +
	"End with https://example.com/auto and `code`."

// asciiOnly fails t for each rune of out, plain, past ASCII.
func asciiOnly(t *testing.T, out string) {
	t.Helper()
	for i, l := range strings.Split(ansi.Strip(out), "\n") {
		for _, r := range l {
			if r >= utf8.RuneSelf || r < ' ' {
				t.Errorf("line %d holds %q (%U): %q", i+1, r, r, l)
				break
			}
		}
	}
}

// With the ASCII glyphs, a source in ASCII renders in ASCII alone, at any
// width, on a dark or a light terminal, with its diagrams shut or open,
// and when it is cut.
func TestASCIIGlyphsRenderASCII(t *testing.T) {
	long := everything + "\n\n" + strings.Repeat("- item\n", maxLines)
	for _, dark := range []bool{true, false} {
		for _, width := range []int{12, 40, 80} {
			t.Run(strconv.FormatBool(dark)+"/"+strconv.Itoa(width), func(t *testing.T) {
				r := New(DefaultStyle(dark))
				r.SetGlyphs(ASCIIGlyphs())
				r.SetHint("o to open")
				for _, open := range [][]int{nil, {0, 1, 2}} {
					out := r.Render(everything, width, open...)
					asciiOnly(t, out)
					if width == 80 && open == nil {
						for _, want := range []string{
							"* one", "[x] done", "-> what the term means", "| A quote", "| | nested in it",
							"i Note", "+ Tip", "! Important", "! Warning", "x Caution",
							"Image: screenshot", "Image: build", "+ Stack trace", "+\nOn lines",
							"* flowchart - 2 lines - View diagram ->",
							"note[1] and another[2]", ":tada:",
						} {
							if !strings.Contains(ansi.Strip(out), want) {
								t.Errorf("the render lacks %q:\n%s", want, ansi.Strip(out))
							}
						}
						if !strings.Contains(out, "\x1b]8;;https://mermaid.live/") {
							t.Errorf("the diagram's offer has no link:\n%q", out)
						}
					}
				}
				out := r.Render(long, width)
				asciiOnly(t, out)
				if !strings.HasSuffix(strings.Join(strings.Fields(ansi.Strip(out)), " "), "... The rest is too long to show here - o to open") {
					t.Errorf("the cut note is %q", out[max(len(out)-80, 0):])
				}
			})
		}
	}
}

// The ASCII glyphs render the sample as the goldens hold it.
func TestRenderASCII(t *testing.T) {
	for _, dark := range []bool{true, false} {
		name := "light"
		if dark {
			name = "dark"
		}
		for _, width := range []int{40, 76} {
			t.Run(name+"/"+strconv.Itoa(width), func(t *testing.T) {
				r := New(DefaultStyle(dark))
				r.SetGlyphs(ASCIIGlyphs())
				out := r.Render(sample, width)
				asciiOnly(t, out)
				golden.RequireEqual(t, out)
			})
		}
	}
}

// Setting glyphs renders again in them, and setting the same ones again,
// or the zero Glyphs on a new renderer, keeps what was rendered.
func TestSetGlyphs(t *testing.T) {
	r := New(DefaultStyle(true))
	def := r.Render(sample, 76)
	r.SetGlyphs(Glyphs{})
	r.Render(sample, 76)
	if r.Renders() != 1 {
		t.Errorf("the zero glyphs rendered again: %d renders", r.Renders())
	}
	r.SetGlyphs(ASCIIGlyphs())
	ascii := r.Render(sample, 76)
	if ascii == def || r.Renders() != 2 {
		t.Errorf("the ASCII glyphs didn't render again: %d renders", r.Renders())
	}
	r.SetGlyphs(ASCIIGlyphs())
	r.Render(sample, 76)
	r.SetGlyphs(Glyphs{})
	if got := r.Render(sample, 76); got != def || r.Renders() != 3 {
		t.Errorf("back to the default glyphs, %d renders:\n%s", r.Renders(), got)
	}
}

// With the ASCII glyphs only the "…" that lipgloss ends a cut header cell
// with becomes "~": the text's own, in code or within a header cell,
// stays.
func TestASCIIKeepsTheTextsEllipses(t *testing.T) {
	r := New(DefaultStyle(true))
	r.SetGlyphs(ASCIIGlyphs())
	for name, tt := range map[string]struct {
		src, want string
		width     int
	}{
		"plain code over a rule":        {"```\nLoading…\n--------\n```", "Loading…\n  --------", 40},
		"highlighted code like a table": {"```go\nA… | b\n---+--\n```", "A… | b", 40},
		"a header's own ellipsis":       {"| Wait… more | b |\n|---|---|\n| 1 | 2 |", "Wait… more", 40},
		"a header cut to its column":    {"| " + strings.Repeat("long ", 6) + "| b |\n|---|---|\n| 1 | 2 |", "~", 14},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ansi.Strip(r.Render(tt.src, tt.width)); !strings.Contains(got, tt.want) {
				t.Errorf("the render lacks %q:\n%s", tt.want, got)
			}
		})
	}
}
