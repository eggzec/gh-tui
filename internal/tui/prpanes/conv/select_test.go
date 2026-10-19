package conv

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestCurrent(t *testing.T) {
	t.Parallel()
	d := newDriver(t, conversation())
	d.all()
	for _, tc := range []struct {
		row, kind     string
		item          string
		thread, reply string
	}{
		{row: "c:1", kind: KindComment, item: "1"},
		{row: "t:T1", kind: KindThread, thread: "T1", reply: "T1-a"},
		{row: "p:T1-b", kind: KindReply, thread: "T1", reply: "T1-b"},
	} {
		d.m.views[""].tree.Reveal(tc.row)
		s, ok := d.m.Current()
		if !ok || s.ID != tc.row || s.Kind != tc.kind {
			t.Fatalf("Current at %s = %q %q %v", tc.row, s.ID, s.Kind, ok)
		}
		if tc.item != "" && (s.Item == nil || s.Item.ID != tc.item) {
			t.Errorf("%s: item %+v, want %q", tc.row, s.Item, tc.item)
		}
		if tc.thread != "" && (s.Thread == nil || s.Thread.ID != tc.thread) {
			t.Errorf("%s: thread %+v, want %q", tc.row, s.Thread, tc.thread)
		}
		if tc.reply != "" && (s.Comment == nil || s.Comment.ID != tc.reply) {
			t.Errorf("%s: comment %+v, want %q", tc.row, s.Comment, tc.reply)
		}
	}

	if _, ok := New(Source{}).Current(); ok {
		t.Error("a model with no rows has a current row")
	}
}

func TestRecacheShowsTheKeptPagesWithoutReading(t *testing.T) {
	t.Parallel()
	f := conversation()
	f.cached = true
	d := newDriver(t, f)
	d.all()
	if rows := d.rowIDs(); !slices.Contains(rows, "c:2") || !slices.Contains(rows, "t:T2") {
		t.Fatalf("rows %v lack c:2 or t:T2 before the change", rows)
	}
	reads := f.fetches

	// The service changes what it keeps: a comment and a thread go.
	f.tl = [][]core.TimelineItem{f.tl[0], {review("R2", core.ReviewStateCommented)}, f.tl[2]}
	f.th = [][]core.Thread{f.th[0], nil}
	m, cmd := d.m.Recache()
	d.m, d.q = m, []tea.Cmd{cmd}
	d.all()

	rows := d.rowIDs()
	for _, gone := range []string{"c:2", "t:T2"} {
		if slices.Contains(rows, gone) {
			t.Errorf("rows %v still show %s", rows, gone)
		}
	}
	for _, kept := range []string{"c:1", "t:T1", "r:R2", "c:3"} {
		if !slices.Contains(rows, kept) {
			t.Errorf("rows %v lack %s", rows, kept)
		}
	}
	if f.fetches != reads {
		t.Errorf("%d reads from the network after Recache, want none", f.fetches-reads)
	}
	if !d.m.feeds[timeline].done || !d.m.feeds[threads].done {
		t.Error("Recache restarted the reads")
	}
}

func TestRecacheWithoutKeptPagesKeepsTheRows(t *testing.T) {
	t.Parallel()
	f := conversation()
	d := newDriver(t, f)
	d.all()
	before, reads := d.rowIDs(), f.fetches
	m, cmd := d.m.Recache()
	d.m, d.q = m, []tea.Cmd{cmd}
	d.all()
	if got := d.rowIDs(); !slices.Equal(got, before) {
		t.Errorf("rows %v after Recache, want %v", got, before)
	}
	if f.fetches != reads {
		t.Errorf("%d reads after Recache, want none", f.fetches-reads)
	}
}
