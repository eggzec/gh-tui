package conv

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// segmentBodies are bodies with details blocks, by the ID of the comment that
// holds each.
var segmentBodies = []struct{ id, body string }{
	{"c1", "Before.\n\n<details>\n<summary>Logs</summary>\n\nline one\n\nline two\n\n</details>"},
	{"c2", "Intro.\n\n<details><summary>Outer</summary>\n\nouter text\n\n<details><summary>Inner</summary>\n\ninner text\n\n</details>\n\nouter end\n\n</details>\n\nAfter the blocks."},
	{"c3", "<details>\n\nno summary\n\n</details>\n\nTrailing."},
	{"c4", "Never closed.\n\n<details><summary>Open ended</summary>\n\nruns to the end"},
	{"c5", "Flow:\n\n```mermaid\ngraph TD\n  A[Start] --> B[Stop]\n  B --> C[Again]\n```\n\nAfter the diagram.\n\n```mermaid\nsequenceDiagram\n  Ann->>Bob: Hi\n```"},
	{"c6", "<details><summary>Diagram</summary>\n\nInside.\n\n```mermaid\npie title Pets\n  \"Dogs\" : 3\n  \"Cats\" : 2\n```\n\nBelow.\n\n</details>"},
}

// diagramBody is a body whose first text holds a diagram.
const diagramBody = "Intro.\n\n```mermaid\ngraph TD\n  A --> B\n```\n\nOutro."

// segmentData is a conversation of comments with details blocks, a review and
// a thread with one.
func segmentData() Data {
	var d Data
	for _, s := range segmentBodies {
		d.Items = append(d.Items, core.TimelineItem{
			Kind: core.TimelineComment, ID: s.id, Actor: core.User{Login: "ann"}, At: at, Body: s.body,
		})
	}
	rev := core.TimelineItem{
		Kind: core.TimelineReview, ID: "R1", Actor: core.User{Login: "bob"}, At: at, Review: core.ReviewStateCommented,
		Body: "Review.\n\n<details><summary>Notes</summary>\n\nin a review\n\n</details>",
	}
	d.Items = append(d.Items, rev)
	d.Threads = []core.Thread{{
		ID: "T1", ReviewID: "R1", Path: "a.go", Line: 3, TotalComments: 2,
		Comments: []core.ThreadComment{
			{ID: "T1-0", Author: core.User{Login: "bob"}, At: at, Body: "Thread.\n\n<details><summary>More</summary>\n\nin a thread\n\n</details>"},
			{ID: "T1-1", Author: core.User{Login: "ann"}, At: at, Body: "Reply.\n\n<details><summary>Reply notes</summary>\n\nin a reply\n\n</details>"},
		},
	}}
	return d
}

func TestSegmentsGolden(t *testing.T) {
	t.Parallel()
	d := segmentData()
	for _, set := range []string{config.IconsNerd, config.IconsASCII} {
		for _, w := range []int{80, 120} {
			for _, all := range []bool{false, true} {
				name := fmt.Sprintf("%s-%d", set, w)
				if all {
					name += "-open"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					out := text(buildOf(d, opts(set, w)), w, all)
					for i, l := range strings.Split(out, "\n") {
						if cells := ansi.StringWidth(l); cells > w {
							t.Errorf("line %d is %d cells wide:\n%s", i, cells, out)
						}
					}
					golden.RequireEqual(t, out)
				})
			}
		}
	}
}

func TestSegmentIDs(t *testing.T) {
	t.Parallel()
	d := segmentData()
	var want []string
	for i, w := range []int{80, 60, 120} {
		rows := buildOf(d, opts(config.IconsNerd, w))
		got := ids(rows)
		if i == 0 {
			want = got
			continue
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("IDs at %d cells differ from those at 80:\n%v\n%v", w, got, want)
		}
	}
	rows := byID(buildOf(d, opts(config.IconsNerd, 80)))
	for _, id := range []string{"rc:c1/d0", "rc:c1/d0/end", "rc:c1/d0/t0", "rc:c2/d0", "rc:c2/d0.0", "rc:c2/d0.0/t0", "rc:c2/d0/t2", "rc:c2/t2", "rc:c5/t0.m0", "rc:c5/t0.s1", "rc:c5/t0.m1", "rc:c6/d0/t0.s0", "rc:c6/d0/t0.m0", "rc:c6/d0/t0.s1"} {
		if _, ok := rows[id]; !ok {
			t.Errorf("no row %q in %v", id, ids(buildOf(d, opts(config.IconsNerd, 80))))
		}
	}
}

func TestSegmentRows(t *testing.T) {
	t.Parallel()
	rows := buildOf(segmentData(), opts(config.IconsNerd, 80))
	by := byID(rows)
	d0 := by["rc:c2/d0"]
	if !d0.Inline || !d0.Fold || !d0.Closed || d0.Kind != KindSegment {
		t.Errorf("a block is not an inline closed fold of the segment kind: %+v", d0)
	}
	// Its text, its nested block with its head, line and end, and its last text.
	if !strings.HasSuffix(d0.Marker, " 5 lines") {
		t.Errorf("marker %q, want it to hide 5 lines", d0.Marker)
	}
	if got, want := by["rc:c2/d0.0"].Depth, d0.Depth+1; got != want {
		t.Errorf("nested block at depth %d, want %d", got, want)
	}
	if end := by["rc:c2/d0/end"]; !end.Skip || !end.Inline {
		t.Errorf("the end of a block is not a skipped inline row: %+v", end)
	}
	if c := by["rc:c2"]; c.Fold {
		t.Errorf("a comment with blocks folds itself")
	}
}

func TestSegmentDiagrams(t *testing.T) {
	t.Parallel()
	for _, w := range []int{80, 60} {
		by := byID(buildOf(segmentData(), opts(config.IconsNerd, w)))
		m0 := by["rc:c5/t0.m0"]
		if !m0.Inline || !m0.Fold || !m0.Closed || m0.Kind != KindSegment {
			t.Errorf("at %d cells a diagram is not an inline closed fold of the segment kind: %+v", w, m0)
		}
		if len(m0.Lines) != 4 || !strings.Contains(ansi.Strip(m0.Lines[0]), "3 lines · View diagram") {
			t.Errorf("at %d cells a diagram has the lines %q, want its head and 3 of code", w, m0.Lines)
		}
		for _, l := range m0.Lines {
			if cells := ansi.StringWidth(l); cells > w {
				t.Errorf("at %d cells a diagram line is %d cells wide: %q", w, cells, l)
			}
		}
		// Text sits beside a diagram without a blank line between them.
		if s1 := by["rc:c5/t0.s1"]; len(s1.Lines) != 1 || !strings.Contains(s1.Lines[0], "After the diagram.") {
			t.Errorf("text after a diagram is %q, want one line", s1.Lines)
		}
		if by["rc:c5/t0.s0"].ID != "" {
			t.Errorf("a body that opens with a diagram has text before it")
		}
		if got, want := by["rc:c6/d0/t0.m0"].Depth, by["rc:c6/d0"].Depth+1; got != want {
			t.Errorf("a diagram in a block is at depth %d, want %d", got, want)
		}
	}
}

func TestSplitDiagram(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		src, head, first string
		base             int
	}{
		"cut":      {diagramBody, "Intro.", "```mermaid\ngraph TD\n  A --> B\n```\n\nOutro.", 0},
		"first":    {"```mermaid\ngraph TD\n  A --> B\n```", "", "```mermaid\ngraph TD\n  A --> B\n```", 0},
		"none":     {"Plain.\n\n```go\nx := 1\n```", "Plain.\n\n```go\nx := 1\n```", "", 1},
		"in code":  {"````md\n```mermaid\ngraph TD\n```\n````", "````md\n```mermaid\ngraph TD\n```\n````", "", 1},
		"indented": {"- item\n\n  ```mermaid\n  graph TD\n  ```", "- item\n\n  ```mermaid\n  graph TD\n  ```", "", 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			head, rest, base := split(tc.src)
			if head != tc.head || base != tc.base {
				t.Errorf("head %q, base %d; want %q, %d", head, base, tc.head, tc.base)
			}
			if tc.first == "" {
				return
			}
			if len(rest) == 0 || rest[0].Block != nil || rest[0].Text != tc.first {
				t.Errorf("rest %+v, want text %q first", rest, tc.first)
			}
		})
	}
}

func TestOwner(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]string{
		"c:1":            "c:1",
		"c:1/d0":         "c:1",
		"c:1/d0.2.1":     "c:1",
		"c:1/d0/end":     "c:1",
		"c:1/d0.1/t3":    "c:1",
		"c:1/t0":         "c:1",
		"c:1/t0.m1":      "c:1",
		"c:1/d0/t2.s3":   "c:1",
		"c:1/t0.m0":      "c:1",
		"rc:c6/d0/t0.m0": "rc:c6",
		"c:1/d0.1/t0.s0": "c:1",
		"rc:9/d1":        "rc:9",
		"r:R1/d0":        "r:R1",
		"t:T1/d0/end":    "t:T1",
		"p:P2/t1":        "p:P2",
		"b:c:1/d0":       "b:c:1/d0",
		"other/d0":       "other/d0",
		"c:1/d0/x":       "c:1/d0/x",
		"c:1/t0.m1/more": "c:1/t0.m1/more",
		"d0":             "d0",
		"c:1/d0/end/d1":  "c:1/d0/end",
	} {
		if got := Owner(id); got != want {
			t.Errorf("Owner(%q) = %q, want %q", id, got, want)
		}
	}
}
