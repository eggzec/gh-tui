package markdown

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

const diagram = "Before.\n\n```mermaid\nsequenceDiagram\n    participant C as Client\n    participant A as API\n    C->>A: POST /licenses\n    A-->>C: 201 Created\n```\n\nAfter."

// A diagram shows as one line that names its kind and size and offers
// its link, at 80 columns on a light and a dark terminal.
func TestDiagramHead(t *testing.T) {
	for _, dark := range []bool{true, false} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			out := New(DefaultStyle(dark)).Render(diagram, 80)
			want := "Before.\n\n  ◆ sequence diagram · 5 lines · View diagram ↗\n\nAfter."
			if got := ansi.Strip(out); got != want {
				t.Errorf("render\n got %q\nwant %q", got, want)
			}
			golden.RequireEqual(t, out)
		})
	}
}

// Opening a diagram shows its code, plain, under its head, and the heads
// say where they and the code are.
func TestDiagramOpens(t *testing.T) {
	r := New(DefaultStyle(true))
	out, heads := r.RenderHeads(diagram, 80)
	lines := strings.Split(ansi.Strip(out), "\n")
	if len(heads) != 1 {
		t.Fatalf("got %d heads, want 1", len(heads))
	}
	h := heads[0]
	if h.Block != 0 || h.End != h.Line+1 || !strings.HasPrefix(lines[h.Line], "  ◆ sequence diagram") ||
		!strings.HasPrefix(h.URL, "https://mermaid.live/view#pako:") {
		t.Errorf("collapsed head %+v on %q", h, lines[h.Line])
	}

	out, heads = r.RenderHeads(diagram, 80, 0)
	lines = strings.Split(ansi.Strip(out), "\n")
	h = heads[0]
	if !strings.HasPrefix(lines[h.Line], "  ◆ sequence diagram") {
		t.Fatalf("open head %+v on %q", h, lines[h.Line])
	}
	code := strings.Join(lines[h.Line+1:h.End], "\n")
	for _, want := range []string{"sequenceDiagram", "C->>A: POST /licenses", "A-->>C: 201 Created"} {
		if !strings.Contains(code, want) {
			t.Errorf("the open diagram lacks %q between its head and its end:\n%s", want, code)
		}
	}
	if !strings.HasSuffix(lines[h.End-1], "A-->>C: 201 Created") || lines[len(lines)-1] != "After." {
		t.Errorf("the code ends at %d of:\n%s", h.End, strings.Join(lines, "\n"))
	}
	if strings.Contains(out, "\x1b[38;2;") {
		t.Errorf("the diagram's code is highlighted:\n%q", out)
	}
}

// A diagram at the top or on lines of its own in a list item has a head.
// One whose fence starts a list item or a quote line, which glamour
// renders on its own, shows as plain code, and one indented as deep as
// code as its collapsed line or as code. None shows a mark, and none
// reaches chroma.
func TestDiagramInContainers(t *testing.T) {
	r := New(DefaultStyle(true))
	if lexer("mermaid", "graph TD") != nil {
		t.Fatal("mermaid has a lexer")
	}
	cases := shapes("mermaid", "graph TD\n  A-->B")
	cases["in an item"] = "- a\n\n  ```mermaid\n  graph TD\n    A-->B\n  ```\n- b"
	cases["in an ordered item"] = "1. a\n   ```mermaid\n   graph TD\n     A-->B\n   ```\n\n   more"
	headed := map[string]bool{"top": true, "in an item": true, "in an ordered item": true}
	for name, src := range cases {
		for _, open := range [][]int{nil, {0}} {
			out, heads := r.RenderHeads(src, 60, open...)
			plain := ansi.Strip(out)
			if strings.Contains(out, r.nonce) {
				t.Errorf("%s shows a mark:\n%s", name, out)
			}
			if !strings.Contains(plain, "◆ flowchart") && !strings.Contains(plain, "A-->B") {
				t.Errorf("%s shows neither the head nor the code:\n%s", name, plain)
			}
			if strings.Contains(out, "\x1b[38;2;") {
				t.Errorf("%s highlights the diagram:\n%q", name, out)
			}
			if headed[name] && (len(heads) != 1 || !strings.Contains(strings.Split(plain, "\n")[heads[0].Line], "◆ flowchart")) {
				t.Errorf("%s: heads %+v in:\n%s", name, heads, plain)
			}
			if headed[name] && len(open) > 0 && !strings.Contains(plain, "A-->B") {
				t.Errorf("%s didn't open:\n%s", name, plain)
			}
		}
	}
}

// A diagram whose link would be too long says so instead of offering it.
func TestDiagramTooLarge(t *testing.T) {
	rnd := rand.New(rand.NewPCG(1, 2))
	var b strings.Builder
	b.WriteString("```mermaid\ngraph TD\n")
	for range 900 {
		fmt.Fprintf(&b, "  n%x --> n%x\n", rnd.Uint64(), rnd.Uint64())
	}
	b.WriteString("```")
	out, heads := New(DefaultStyle(true)).RenderHeads(b.String(), 80)
	if got, want := ansi.Strip(out), "  ◆ flowchart · 901 lines · too large to view"; got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
	if len(heads) != 1 || heads[0].URL != "" {
		t.Errorf("heads = %+v, want one without a link", heads)
	}
}

var pakoURL = regexp.MustCompile(`^https://mermaid\.live/view#pako:[A-Za-z0-9_-]+$`)

// What a diagram holds, however hostile, never reaches the terminal as
// anything but text, nor the head's label, nor the link as it is.
func TestDiagramHostile(t *testing.T) {
	for _, code := range []string{
		"\x1b]8;;https://evil.test\x07graph TD\x1b]8;;\x07\n  A-->B",
		"sequenceDiagram\x1b[2J\n  A->>B: \x1b]0;title\x07hi",
		"\u202egraph TD\u202c\n  A[\u2066x\u2069]-->B",
		"graph TD\n  A[\u009b31mx]-->B &#27;[2J &#x202e;",
		"%%{init: {\"theme\": \"\x1b[31m\"}}%%\n\x00flowchart\n  A-->B",
	} {
		src := "```mermaid\n" + code + "\n```"
		r := New(DefaultStyle(true))
		for _, open := range [][]int{nil, {0}} {
			out, heads := r.RenderHeads(src, 80, open...)
			if why := unsafe(out); why != "" {
				t.Errorf("%q: %s", code, why)
			}
			for _, h := range heads {
				if !pakoURL.MatchString(h.URL) {
					t.Errorf("%q: link %q", code, h.URL)
				}
			}
		}
		b := Blocks(src)[0]
		if b.kind != "diagram" && b.kind != "flowchart" && b.kind != "sequence diagram" {
			t.Errorf("%q: kind %q", code, b.kind)
		}
		for _, bad := range []string{"\x1b", "\u202e", "evil", "title", "\x00"} {
			if strings.Contains(b.Collapsed, bad) {
				t.Errorf("%q: collapsed line %q holds %q", code, b.Collapsed, bad)
			}
		}
	}
}

// The offer of a head is a hyperlink to the diagram's view, opened and
// closed once, which takes no cells.
func TestDiagramLink(t *testing.T) {
	r := New(DefaultStyle(true))
	url := Blocks(diagram)[0].URL
	out, heads := r.RenderHeads(diagram, 80)
	line := strings.Split(out, "\n")[heads[0].Line]
	open, end := "\x1b]8;;"+url+"\x1b\\", "\x1b]8;;\x1b\\"
	if strings.Count(line, open) != 1 || strings.Count(line, "\x1b]8;;") != 2 || strings.Count(line, end) != 1 {
		t.Fatalf("head %q doesn't link to %q once", line, url)
	}
	if got := line[strings.Index(line, open)+len(open) : strings.Index(line, end)]; ansi.Strip(got) != "View diagram ↗" {
		t.Errorf("the link holds %q", ansi.Strip(got))
	}
	if w, want := ansi.StringWidth(line), ansi.StringWidth(ansi.Strip(line)); w != want || w != ansi.StringWidth("  ◆ sequence diagram · 5 lines · View diagram ↗") {
		t.Errorf("head is %d cells, %d without the link", w, want)
	}
	for other := range strings.SplitSeq(out, "\n") {
		if other != line && strings.Contains(other, "\x1b]") {
			t.Errorf("another line links: %q", other)
		}
	}
}

// Cut at any width, by the render or by a caller, a head never leaves a
// link open, and links only while its whole offer shows.
func TestDiagramLinkCut(t *testing.T) {
	r := New(DefaultStyle(true))
	for w := 1; w <= 60; w++ {
		out, heads := r.RenderHeads(diagram, w)
		for _, h := range heads {
			line := strings.Split(out, "\n")[h.Line]
			if strings.Contains(line, "\x1b]8;;https") && !strings.Contains(ansi.Strip(line), "View diagram ↗") {
				t.Errorf("width %d: links a cut offer: %q", w, line)
			}
			for cut := range ansi.StringWidth(line) + 1 {
				c := ansi.Truncate(line, cut, "…")
				if n, m := strings.Count(c, "\x1b]8;;https"), strings.Count(c, "\x1b]8;;\x1b\\"); n != m || n > 1 {
					t.Errorf("width %d cut at %d: %d opened, %d closed: %q", w, cut, n, m, c)
				}
				if ansi.StringWidth(c) > cut {
					t.Errorf("width %d cut at %d is %d wide", w, cut, ansi.StringWidth(c))
				}
			}
		}
	}
}

// A comment can't make a link of its own, not even one that looks like a
// head's, and a diagram too large to link has none.
func TestDiagramLinkForged(t *testing.T) {
	url := Blocks(diagram)[0].URL
	for _, src := range []string{
		"\x1b]8;;" + url + "\x1b\\View diagram ↗\x1b]8;;\x1b\\",
		"\x1b]8;;" + url + "\aView diagram ↗\x1b]8;;\a",
		"◆ sequence diagram · 5 lines · View diagram ↗",
		"```\n◆ sequence diagram · 5 lines · View diagram ↗\n```",
		"[View diagram ↗](" + url + ")",
		"<a href=\"" + url + "\">View diagram ↗</a>",
	} {
		out := New(DefaultStyle(true)).Render(src, 80)
		if strings.Contains(out, "\x1b]") {
			t.Errorf("%q links: %q", src, out)
		}
	}
	rnd := rand.New(rand.NewPCG(3, 4))
	var b strings.Builder
	b.WriteString("```mermaid\ngraph TD\n")
	for range 900 {
		fmt.Fprintf(&b, "  n%x --> n%x\n", rnd.Uint64(), rnd.Uint64())
	}
	b.WriteString("```")
	if out := New(DefaultStyle(true)).Render(b.String(), 80); strings.Contains(out, "\x1b]") {
		t.Errorf("a diagram too large links: %q", out)
	}
	if got := linked("  View diagram ↗", "https://evil.test/view#pako:abc", Glyphs{}.orDefault()); got != "  View diagram ↗" {
		t.Errorf("linked to another page: %q", got)
	}
}

// Every head that offers its diagram links to it once, at every size up
// to the longest link and past it, where the head says it's too large.
func TestDiagramOfferIsLinked(t *testing.T) {
	rnd := rand.New(rand.NewPCG(7, 8))
	r := New(DefaultStyle(true))
	var b strings.Builder
	large := false
	for n := 1; n <= 200 && !large; n++ {
		fmt.Fprintf(&b, "  n%x --> n%x\n", rnd.Uint32(), rnd.Uint32())
		out, heads := r.RenderHeads("```mermaid\ngraph TD\n"+b.String()+"```", 80)
		line := strings.Split(out, "\n")[heads[0].Line]
		links := strings.Count(line, "\x1b]8;;https://mermaid.live/view#pako:")
		switch plain := ansi.Strip(line); {
		case strings.Contains(plain, "View diagram ↗"):
			if links != 1 || strings.Count(line, "\x1b]8;;\x1b\\") != 1 {
				t.Fatalf("%d lines: the offer has %d links: %q", n, links, line)
			}
		case strings.Contains(plain, "too large to view"):
			large = true
			if links != 0 || heads[0].URL != "" {
				t.Fatalf("%d lines: a head too large links: %q", n, line)
			}
		default:
			t.Fatalf("%d lines: head %q", n, plain)
		}
	}
	if !large {
		t.Error("no diagram was too large to link")
	}
}
