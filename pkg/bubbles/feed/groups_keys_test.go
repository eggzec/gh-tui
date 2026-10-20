package feed

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// under returns the label or id of the line under the cursor.
func under(t *testing.T, m Model[item]) string {
	t.Helper()
	l := m.lines[m.sel]
	switch {
	case l.more:
		return "more"
	case l.head:
		return l.label
	}
	it, _ := m.item(l.item)
	return it.id
}

func TestGroupKeysOnHeadersAndItems(t *testing.T) {
	m := load(t, newSource(6, 6), WithSize(40, 20))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})

	// h on an open header closes it, and l opens it again.
	m = keys(t, m, "home", "h")
	wantOutline(t, m, "even(3)-", "odd(3)", ".1", ".3", ".5")
	m = keys(t, m, "l")
	if got := len(m.lines); got != 8 {
		t.Fatalf("%d lines after l, want 8", got)
	}
	// l on an open header moves to its first child, and on an item does nothing.
	m = keys(t, m, "l")
	if got := under(t, m); got != "0" {
		t.Fatalf("on %q after l on an open header, want its first item", got)
	}
	m = keys(t, m, "l")
	if got := under(t, m); got != "0" {
		t.Fatalf("on %q after l on an item, want it to stay", got)
	}
	// h on an item moves to the header, without closing it.
	m = keys(t, m, "down", "h")
	if got := under(t, m); got != "even" || !m.lines[m.sel].open {
		t.Fatalf("on %q (open %v) after h on an item, want the open even header", got, m.lines[m.sel].open)
	}
	// h on a closed header with no parent does nothing.
	m = keys(t, m, "h", "h")
	if got := under(t, m); got != "even" || m.lines[m.sel].open {
		t.Fatalf("on %q after h on a closed top header, want it closed and still there", got)
	}
}

func TestGroupKeysToParentInTwoLevels(t *testing.T) {
	m := load(t, newSource(12, 12), WithSize(40, 30))
	m = applyGroups(t, m, []Group[item]{{Key: parity}, {Key: third}})
	m = keys(t, m, "home", "j", "j", "h")
	if got := under(t, m); got != "m0" {
		t.Fatalf("on %q after h from an item, want m0", got)
	}
	m = keys(t, m, "h")
	if got := under(t, m); got != "m0" || m.lines[m.sel].open {
		t.Fatalf("on %q after h on an open header, want it closed", got)
	}
	m = keys(t, m, "h")
	if got := under(t, m); got != "even" {
		t.Fatalf("on %q after h on a closed header, want its parent", got)
	}
}

func TestGroupKeysToggleAll(t *testing.T) {
	m := load(t, newSource(6, 6), WithSize(40, 20))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	m = keys(t, m, "*")
	wantOutline(t, m, "even(3)-", "odd(3)-")
	m = keys(t, m, "l", "*")
	wantOutline(t, m, "even(3)", ".0", ".2", ".4", "odd(3)", ".1", ".3", ".5")
	m = keys(t, m, "*")
	wantOutline(t, m, "even(3)-", "odd(3)-")
}

func TestGroupKeysOnlyWhileGrouped(t *testing.T) {
	m := load(t, newSource(6, 6), WithSize(40, 20))
	if m.KeyMap().Expand.Enabled() {
		t.Fatal("expand is enabled in a flat feed")
	}
	m = keys(t, m, "j", "l", "h", "*")
	if m.Index() != 1 || m.Grouped() {
		t.Fatalf("flat feed changed by the group keys: index %d", m.Index())
	}
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	if !m.KeyMap().Expand.Enabled() || !m.KeyMap().ToggleAll.Enabled() {
		t.Fatal("expand is disabled in a grouped feed")
	}
}

func TestGroupMoreRow(t *testing.T) {
	src := newSource(20, 5)
	m := load(t, src, WithSize(40, 3), WithPrefetch(1))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	wantOutline(t, m, "even(3)", ".0", ".2", ".4", "odd(2)", ".1", ".3", "more")

	// The row reads how many are left when the total is known.
	if got := m.moreText(); got != "more not loaded" {
		t.Fatalf("text = %q", got)
	}
	m.SetTotal(20)
	if got := m.moreText(); got != "15 more not loaded" {
		t.Fatalf("text = %q", got)
	}

	// Reaching the row fetches the next chunk, and it is not an item.
	calls := src.callCount()
	m = keys(t, m, "end")
	if got := src.callCount(); got != calls+1 {
		t.Fatalf("%d fetches on reaching the row, want %d", got, calls+1)
	}
	if m.Len() != 10 {
		t.Fatalf("%d items after the fetch, want 10", m.Len())
	}
	// The cursor stays on the row, which the new items pushed down.
	if got := under(t, m); got != "more" {
		t.Fatalf("cursor on %q after the fetch, want the row of the items not loaded", got)
	}

	// Enter on it fetches too, and on another row it is not taken.
	m.sel = len(m.lines) - 1
	m.scroll()
	if _, ok := m.Selected(); ok {
		t.Fatal("Selected() is true on the row of the items not loaded")
	}
	if !strings.Contains(ansi.Strip(m.View()), "10 more not loaded") {
		t.Fatalf("the row is not drawn:\n%s", ansi.Strip(m.View()))
	}
	calls = src.callCount()
	cmd, ok := m.Activate()
	if !ok {
		t.Fatal("Activate() did not take the row")
	}
	m = run(t, m, cmd)
	if got := src.callCount(); got != calls+1 {
		t.Fatalf("%d fetches on enter, want %d", got, calls+1)
	}
	if got := under(t, m); got != "more" {
		t.Fatalf("cursor on %q after enter, want the row of the items not loaded", got)
	}
	m.sel = 0
	if _, ok := m.Activate(); ok {
		t.Fatal("Activate() took a header")
	}
}

func TestGroupMoreRowGoesWhenDone(t *testing.T) {
	m := load(t, newSource(4, 4), WithSize(40, 20))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	wantOutline(t, m, "even(2)", ".0", ".2", "odd(2)", ".1", ".3")
}

func TestGroupQuickFilter(t *testing.T) {
	m := load(t, bugs(10), WithSize(40, 20))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	m = typed(t, m, "&", "bug", "enter")
	// Items 2 and 7 are the bug fixes: the headers count what is left.
	wantOutline(t, m, "even(1)", ".2", "odd(1)", ".7")
	// The row counts the items not fetched, not those the filter leaves out.
	m.SetTotal(20)
	if got := m.moreText(); got != "10 more not loaded" {
		t.Fatalf("text = %q", got)
	}
	m = typed(t, m, "esc")
	if got := len(m.lines); got != 12 {
		t.Fatalf("%d lines after clearing the filter, want 12", got)
	}
}

func TestGroupFindOpensClosedGroup(t *testing.T) {
	m := load(t, bugs(10), WithSize(40, 20))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	m = keys(t, m, "*")
	wantOutline(t, m, "even(5)-", "odd(5)-")
	m = typed(t, m, "/", "bug", "enter")
	if got := under(t, m); got != "2" {
		t.Fatalf("on %q after a find in closed groups, want item 2", got)
	}
	// Next goes on in the order of the groups, into the other closed group.
	m = keys(t, m, "n")
	if got := under(t, m); got != "7" {
		t.Fatalf("on %q after n, want item 7", got)
	}
	m = keys(t, m, "n")
	if got := under(t, m); got != "2" {
		t.Fatalf("on %q after n wraps, want item 2", got)
	}
}
