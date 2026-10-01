package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
)

// pullsKinds returns the kinds details and comments of prefetch.pulls,
// read through d and c.
func pullsKinds(d, c *reader) []AheadKind[int] {
	return []AheadKind[int]{
		{Name: "details", Log: "row", Read: d.readRow, Current: d.current},
		{Name: "comments", Log: "row_comments", Read: c.readRow, Current: c.current},
	}
}

func TestAheadsKindsApart(t *testing.T) {
	d, c := newReader(), newReader()
	a := NewAheads(t.Context(), "pulls", pullsKinds(d, c)...)
	if a.On() || a.Window(rowsOf(30), 0) != nil {
		t.Fatal("Aheads read ahead before it had the settings")
	}

	p := config.Default().Prefetch
	p.Window = config.Window{Before: 0, After: 2}
	p.Pulls.Comments.Enabled = new(false)
	a.Configure(p)
	run(a.Window(rowsOf(30), 0))
	if got, want := d.reads(), []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Errorf("details read %v, want %v", got, want)
	}
	if got := c.reads(); len(got) != 0 {
		t.Errorf("comments read %v, want none while they are off", got)
	}

	// Comments turn on with a window of their own, from the next move.
	p.Pulls.Comments.Enabled = new(true)
	p.Pulls.Comments.Window.After = new(0)
	p.Pulls.Comments.Rest = new(time.Duration(0))
	a.Configure(p)
	if !a.On() {
		t.Error("On = false with both kinds on")
	}
	run(a.Window(rowsOf(30), 0))
	if got, want := c.reads(), []int{1}; !slices.Equal(got, want) {
		t.Errorf("comments read %v, want the row under the cursor, %v", got, want)
	}
}

func TestAheadsUnknownKindReadsNothing(t *testing.T) {
	r := newReader()
	a := NewAheads(t.Context(), "pulls", AheadKind[int]{Name: "nope", Log: "row", Read: r.readRow, Current: r.current})
	a.Configure(config.Default().Prefetch)
	if a.On() || a.Window(rowsOf(30), 0) != nil {
		t.Error("a kind the settings don't have read ahead")
	}
}

func TestNilAheads(t *testing.T) {
	var a *Aheads[int]
	a.Configure(config.Default().Prefetch)
	a.Reset(t.Context())
	a.Resume()
	a.Opened(1)
	a.Pause()()
	if a.On() || a.Window(rowsOf(3), 0) != nil || a.Rested(AheadMsg{}) != nil {
		t.Error("a nil Aheads read ahead")
	}
}
