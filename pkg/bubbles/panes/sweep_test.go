package panes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestCoverage checks, for every size from 0x0 to 200x60 and every focus and
// zoom, that the boxes of Compute stay in the area and that no cell is
// covered twice. Every cell is covered exactly once as well: Compute cuts the
// boxes to the area, and the columns and the right column's boxes follow one
// another with no gap, so a box that does not fit costs the ones after it
// their cells but leaves no hole.
func TestCoverage(t *testing.T) {
	cover := make([]int, 200*60)
	for w := range 201 {
		for h := range 61 {
			for focus := Pane(0); focus <= Pane(len(testPanes)); focus++ {
				spec := Spec{Width: w, Height: h, Panes: testPanes, Focus: focus, Zoom: true}
				l := Compute(spec)
				cover = cover[:w*h]
				clear(cover)
				for _, b := range l.Boxes {
					r := b.Rect
					if r.X < 0 || r.Y < 0 || r.W < 0 || r.H < 0 || r.X+r.W > w || r.Y+r.H > h {
						t.Fatalf("%+v: %s %v box %+v leaves the area", spec, l.Title(b.Pane), b.Role, r)
					}
					for y := r.Y; y < r.Y+r.H; y++ {
						for x := r.X; x < r.X+r.W; x++ {
							cover[y*w+x]++
						}
					}
				}
				for i, n := range cover {
					if n != 1 {
						t.Fatalf("%+v: cell %d,%d covered %d times, boxes %+v", spec, i%w, i/w, n, l.Boxes)
					}
				}
			}
		}
	}
}

// Content that is awkward at any size: wide characters in the title and the
// info forms, styles that are never closed, hyperlinks and tabs.
const sweepBody = "\x1b[1;31m漢字 bold red text that runs on and on\x1b[m\n" +
	"\tindented\ttabbed\n" +
	"\x1b]8;;https://example.com/a-long-url\x1b\\a linked 漢字 text that runs on\x1b]8;;\x1b\\ end\n" +
	"🙂🙂🙂🙂🙂🙂🙂🙂\n" +
	"\x1b[4munderlined and never closed, and long enough to be cut\n" +
	"\x1b]8;;https://example.com/never-closed\x1b\\a link left open in a long line"

func sweepContent() Content {
	return Content{
		TopLeft:     []string{"漢字のタイトル日本語", "漢字のタイトル", "漢字"},
		TopRight:    []string{"日本語日本語日本語日本語", "日本語日本語", "漢", "ab"},
		BottomLeft:  []string{"日本語日本語日本語", "漢字", "a"},
		BottomRight: []string{"日本語日本語", "漢"},
		Body:        sweepBody,
	}
}

// checkLines fails when out is not h lines of w cells, or a line leaves a
// hyperlink open.
func checkLines(t *testing.T, what, out string, w, h int) []string {
	t.Helper()
	lines := strings.Split(out, "\n")
	if h == 0 {
		if out != "" {
			t.Fatalf("%s: %q for no rows", what, out)
		}
		return nil
	}
	if len(lines) != h {
		t.Fatalf("%s: %d lines, want %d", what, len(lines), h)
	}
	for i, ln := range lines {
		if got := ansi.StringWidth(ln); got != w {
			t.Fatalf("%s: line %d is %d cells, want %d: %q", what, i, got, w, ansi.Strip(ln))
		}
		if linkOpen(ln) {
			t.Fatalf("%s: line %d leaves a hyperlink open: %q", what, i, visible(ln))
		}
		if sgrOpen(ln) {
			t.Fatalf("%s: line %d ends in an open style: %q", what, i, visible(ln))
		}
	}
	return lines
}

// TestFrameSweep draws awkward content in every box size from 0x0 to 40x40,
// focused or not. No row's content may bleed a style into the border.
func TestFrameSweep(t *testing.T) {
	st := DefaultStyles(true)
	for _, focus := range []bool{false, true} {
		c := sweepContent()
		c.Focus = focus
		right := st.edge(focus).Render(st.Border.Right)
		for w := range 41 {
			for h := range 41 {
				lines := checkLines(t, "Frame", st.Frame(c, w, h), w, h)
				if w < 2 || h < 2 {
					continue
				}
				for i := 1; i < h-1; i++ {
					body, ok := strings.CutSuffix(lines[i], right)
					if !ok || sgrOpen(body) {
						t.Fatalf("%dx%d: row %d bleeds into the border: %q", w, h, i, visible(lines[i]))
					}
				}
			}
		}
	}
}

// TestDrawSweep draws awkward content over the layouts of Compute, at sizes
// from 0x0 to 40x40 and at the widths around the zoom and split thresholds.
func TestDrawSweep(t *testing.T) {
	st := DefaultStyles(true)
	content := func(Box) Content { return sweepContent() }
	widths := make([]int, 0, 50)
	for w := range 41 {
		widths = append(widths, w)
	}
	widths = append(widths, 99, 100, 101, 119, 120, 121, 140, 160, 200)
	for _, w := range widths {
		for h := range 41 {
			for focus := Pane(1); focus <= Pane(len(testPanes)); focus++ {
				l := Compute(Spec{Width: w, Height: h, Panes: testPanes, Focus: focus, Zoom: true})
				checkLines(t, "Draw", st.Draw(l, content), w, h)
			}
		}
	}
}
