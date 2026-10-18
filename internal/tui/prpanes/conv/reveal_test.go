package conv

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// revealed loads the timeline and the threads into a driver.
func revealed(t *testing.T, items []core.TimelineItem, ths []core.Thread) *driver {
	t.Helper()
	d := newDriver(t, newFake([][]core.TimelineItem{items}, [][]core.Thread{ths}))
	d.all()
	return d
}

func selected(t *testing.T, d *driver) string {
	t.Helper()
	id, _, ok := d.m.Selected()
	if !ok {
		t.Fatal("no row under the cursor")
	}
	return id
}

func TestRevealOpensAncestors(t *testing.T) {
	t.Parallel()
	d := revealed(t, []core.TimelineItem{review("R1", core.ReviewStateApproved), comment("1")}, []core.Thread{thread("T1", "R1", 2)})
	tree := &d.m.views[""].tree
	if !tree.Folded("r:R1") {
		t.Fatal("the review starts open, so nothing shows that reveal opens it")
	}
	if !d.m.Reveal("T1-b") {
		t.Fatal("Reveal found no row for a reply")
	}
	if tree.Folded("r:R1") {
		t.Error("the review above the reply is still closed")
	}
	if got := selected(t, d); got != "p:T1-b" {
		t.Errorf("cursor on %q, want p:T1-b", got)
	}
	if !d.m.Reveal("1") || selected(t, d) != "c:1" {
		t.Errorf("a comment alone: cursor on %q", selected(t, d))
	}
}

func TestRevealClosedRunLandsOnRun(t *testing.T) {
	t.Parallel()
	d := revealed(t, []core.TimelineItem{comment("1"), comment("2"), comment("3")}, nil)
	tree := &d.m.views[""].tree
	tree.SetFolded("run:1", true)
	if !d.m.Reveal("2") {
		t.Fatal("Reveal found no row for a comment of a run")
	}
	if got := selected(t, d); got != "run:1" || !tree.Folded("run:1") {
		t.Errorf("cursor on %q, run closed = %v; want the closed run row", got, tree.Folded("run:1"))
	}
	tree.SetFolded("run:1", false)
	if !d.m.Reveal("2") || selected(t, d) != "rc:2" {
		t.Errorf("with the run open, cursor on %q, want rc:2", selected(t, d))
	}
}

func TestRevealHidden(t *testing.T) {
	t.Parallel()
	d := revealed(t, []core.TimelineItem{comment("1"), review("R1", core.ReviewStateApproved)}, nil)
	d.m.SetShown(allBut(GroupComments))
	if d.m.Reveal("1") {
		t.Error("Reveal found a row for a comment the filter hides")
	}
	if !d.m.Holds("1") {
		t.Error("Holds says the timeline lacks a comment that the filter hides")
	}
	if d.m.Reveal("nope") || d.m.Holds("nope") {
		t.Error("an unknown item was found")
	}
	if !d.m.Reveal("R1") {
		t.Error("Reveal found no row for a review the filter shows")
	}
}

func TestLatestReviewBy(t *testing.T) {
	t.Parallel()
	d := revealed(t, []core.TimelineItem{
		review("R1", core.ReviewStateChangesRequested), comment("1"), review("R2", core.ReviewStateApproved), review("R3", core.ReviewStatePending),
	}, nil)
	d.m.items[1].Actor.Login = "bob"
	if got, _ := d.m.LatestBy("bob"); got == "R1" {
		t.Fatalf("LatestBy(bob) = %q, the case needs a later item by bob", got)
	}
	for _, tc := range []struct {
		login string
		state core.ReviewState
		want  string
		ok    bool
	}{
		{"bob", core.ReviewStateChangesRequested, "R1", true}, // not the later comment or review
		{"bob", core.ReviewStateApproved, "R2", true},
		{"bob", core.ReviewStatePending, "", false}, // never submitted
		{"bob", core.ReviewStateCommented, "", false},
		{"ann", core.ReviewStateChangesRequested, "", false},
	} {
		got, ok := d.m.LatestReviewBy(tc.login, tc.state)
		if got != tc.want || ok != tc.ok {
			t.Errorf("LatestReviewBy(%q, %v) = %q, %v; want %q, %v", tc.login, tc.state, got, ok, tc.want, tc.ok)
		}
	}
}

func TestReady(t *testing.T) {
	t.Parallel()
	d := newDriver(t, newFake([][]core.TimelineItem{{comment("1")}}, [][]core.Thread{nil}))
	if d.m.Ready() {
		t.Error("Ready before any page")
	}
	d.all()
	if !d.m.Ready() {
		t.Error("not Ready with every page read")
	}
}

func TestLatestBy(t *testing.T) {
	t.Parallel()
	d := revealed(t, []core.TimelineItem{
		comment("1"), review("R1", core.ReviewStateApproved), comment("2"), review("R2", core.ReviewStateCommented),
	}, []core.Thread{thread("T1", "R1", 2)})
	for _, tc := range []struct {
		login, want string
		ok          bool
	}{
		{"ann", "2", true},    // the later comment, not the first
		{"bob", "R2", true},   // the later review
		{"dee", "T1-b", true}, // only in a thread: its last comment
		{"nobody", "", false},
	} {
		got, ok := d.m.LatestBy(tc.login)
		if got != tc.want || ok != tc.ok {
			t.Errorf("LatestBy(%q) = %q, %v; want %q, %v", tc.login, got, ok, tc.want, tc.ok)
		}
	}
	// A comment that comes after a review by the same person is the latest.
	d = revealed(t, []core.TimelineItem{review("R1", core.ReviewStateApproved), comment("1")}, nil)
	d.m.items[1].Actor.Login = "bob"
	if got, _ := d.m.LatestBy("bob"); got != "1" {
		t.Errorf("LatestBy(bob) = %q, want the later comment 1", got)
	}
}
