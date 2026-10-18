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
}

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
	for _, id := range []string{"rc:c1/d0", "rc:c1/d0/end", "rc:c1/d0/t0", "rc:c2/d0", "rc:c2/d0.0", "rc:c2/d0.0/t0", "rc:c2/d0/t2", "rc:c2/t2"} {
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
