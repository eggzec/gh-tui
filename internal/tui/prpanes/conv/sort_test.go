package conv

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// aged is it, written h hours after the base time.
func aged(it core.TimelineItem, h int) core.TimelineItem {
	it.At = at.Add(time.Duration(h) * time.Hour)
	return it
}

// activity is a conversation whose old review has the newest reply, whose
// last two items were written at the same time, and which has a pending
// review of the viewer. Oldest first it reads c1 R1 [c2 c3] R2 c4.
func activity() *fake {
	t1 := thread("T1", "R1", 2)
	t1.Comments[0].At = at.Add(10 * time.Hour)
	t1.Comments[1].At = at.Add(11 * time.Hour)
	draft := thread("T9", "", 1)
	draft.Pending = true
	draft.Comments[0].Pending = true
	return newFake(
		[][]core.TimelineItem{{
			aged(comment("1"), 1),
			aged(review("R1", core.ReviewStateCommented), 2),
			aged(comment("2"), 3),
			aged(comment("3"), 4),
			aged(review("R2", core.ReviewStateApproved), 5),
			aged(comment("4"), 5),
		}},
		[][]core.Thread{{t1, draft}},
	)
}

// top returns the IDs of the rows of the top level.
func (d *driver) top() []string {
	var out []string
	for _, id := range d.rowIDs() {
		if strings.HasPrefix(id, "c:") || strings.HasPrefix(id, "r:") || strings.HasPrefix(id, "run:") {
			out = append(out, id)
		}
	}
	return out
}

func TestOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		order Order
		want  []string
	}{
		{"oldest keeps the timeline", OrderOldest, []string{"c:1", "r:R1", "run:2", "r:R2", "c:4", "r:pending"}},
		{"newest reverses it and keeps the pending review last", OrderNewest, []string{"c:4", "r:R2", "run:2", "r:R1", "c:1", "r:pending"}},
		{"activity puts the review with the newest reply first", OrderActivity, []string{"r:R1", "r:R2", "c:4", "run:2", "c:1", "r:pending"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newDriver(t, activity())
			d.all()
			d.m.SetOrder(tc.order)
			if got := d.top(); !slices.Equal(got, tc.want) {
				t.Errorf("top level = %v, want %v", got, tc.want)
			}
			if got := d.m.Order(); got != tc.order {
				t.Errorf("Order() = %v, want %v", got, tc.order)
			}
			d.m.SetOrder(OrderOldest)
			if got, want := d.top(), []string{"c:1", "r:R1", "run:2", "r:R2", "c:4", "r:pending"}; !slices.Equal(got, want) {
				t.Errorf("back to oldest = %v, want %v", got, want)
			}
		})
	}
}

func TestOrderKeepsInsides(t *testing.T) {
	t.Parallel()
	for _, o := range []Order{OrderNewest, OrderActivity} {
		d := newDriver(t, activity())
		d.all()
		d.m.SetOrder(o)
		rows := d.rowIDs()
		if i := slices.Index(rows, "run:2"); i < 0 || rows[i+1] != "rc:2" || rows[i+2] != "rc:3" {
			t.Errorf("order %d: the comments of the run are not in the timeline's order: %v", o, rows)
		}
	}
}

func TestOrderEditedComment(t *testing.T) {
	t.Parallel()
	edited := aged(comment("1"), 1)
	edited.UpdatedAt = at.Add(9 * time.Hour)
	d := newDriver(t, newFake([][]core.TimelineItem{{edited, aged(review("R1", core.ReviewStateCommented), 2), aged(comment("2"), 3)}}, [][]core.Thread{{}}))
	d.all()
	d.m.SetOrder(OrderActivity)
	if got, want := d.top(), []string{"c:1", "c:2", "r:R1"}; !slices.Equal(got, want) {
		t.Errorf("top level = %v, want %v", got, want)
	}
}

func TestOrderKeepsCursor(t *testing.T) {
	t.Parallel()
	d := newDriver(t, newFake([][]core.TimelineItem{{aged(comment("1"), 1), aged(comment("2"), 2), aged(review("R1", core.ReviewStateCommented), 3)}}, [][]core.Thread{{}}))
	d.all()
	d.m.views[""].tree.Reveal("rc:2")
	d.m.SetOrder(OrderNewest)
	if id, _, _ := d.m.Selected(); id != "rc:2" {
		t.Errorf("the cursor is on %q, want rc:2 (rows %v)", id, d.rowIDs())
	}
}

func TestOrderWithFilter(t *testing.T) {
	t.Parallel()
	d := newDriver(t, activity())
	d.all()
	d.m.SetShown(allBut(GroupReviews))
	d.m.SetOrder(OrderNewest)
	if got, want := d.top(), []string{"run:1"}; !slices.Equal(got, want) {
		t.Errorf("top level = %v, want %v", got, want)
	}
}

func TestPreviewIgnoresOrder(t *testing.T) {
	t.Parallel()
	d := newDriver(t, conversation())
	d.all()
	d.m.SetOrder(OrderNewest)
	d.m.SetView("p", Narrow{Item: func(it core.TimelineItem) bool { return it.Kind == core.TimelineComment }})
	if got, want := d.top(), []string{"c:1", "c:2", "c:3"}; !slices.Equal(got, want) {
		t.Errorf("preview rows = %v, want %v", got, want)
	}
}

func TestOrderGolden(t *testing.T) {
	t.Parallel()
	d := fixture()
	for _, o := range []struct {
		name  string
		order Order
	}{{"newest", OrderNewest}, {"activity", OrderActivity}} {
		for _, w := range []int{80, 120, 160} {
			t.Run(o.name+"-"+strconv.Itoa(w), func(t *testing.T) {
				t.Parallel()
				op := opts(config.IconsNerd, w)
				op.order = o.order
				golden.RequireEqual(t, text(Build(d, op), w, false))
			})
		}
	}
}

func BenchmarkSetOrder(b *testing.B) {
	d := newDriver(b, bigFake(),
		WithSize(160, 50), WithOptions(opts(config.IconsNerd, 160)), WithTreeKeys(treeKeys()))
	d.all()
	d.m.Focus()
	b.ReportAllocs()
	for b.Loop() {
		d.m.SetOrder(OrderNewest)
		d.m.SetOrder(OrderOldest)
	}
}
