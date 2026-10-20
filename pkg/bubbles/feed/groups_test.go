package feed

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func idNum(it item) int {
	n, _ := strconv.Atoi(it.id)
	return n
}

func parity(it item) string {
	if idNum(it)%2 == 0 {
		return "even"
	}
	return "odd"
}

func third(it item) string { return "m" + strconv.Itoa(idNum(it)%3) }

// outline writes the lines of a grouped feed: a header as "label(count)" with
// its depth in dots, "-" while closed, and an item as its id.
func outline[T any](m Model[T]) []string {
	var got []string
	for _, l := range m.lines {
		if l.more {
			got = append(got, "more")
			continue
		}
		if !l.head {
			got = append(got, strings.Repeat(".", l.depth)+strconv.Itoa(l.item))
			continue
		}
		s := strings.Repeat(".", l.depth) + l.label + "(" + strconv.Itoa(l.count) + ")"
		if !l.open {
			s += "-"
		}
		got = append(got, s)
	}
	return got
}

func wantOutline(t *testing.T, m Model[item], want ...string) {
	t.Helper()
	if got := outline(m); !slices.Equal(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

// apply calls a method of m that changes it and returns a command, and runs
// the command.
func apply(t *testing.T, m Model[item], do func(*Model[item]) tea.Cmd) Model[item] {
	t.Helper()
	cmd := do(&m)
	return run(t, m, cmd)
}

func applyGroups(t *testing.T, m Model[item], groups []Group[item]) Model[item] {
	t.Helper()
	return apply(t, m, func(m *Model[item]) tea.Cmd { return m.SetGroups(groups) })
}

func TestGroupsOneLevel(t *testing.T) {
	m := load(t, newSource(6, 6), WithSize(40, 20))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	wantOutline(t, m, "even(3)", ".0", ".2", ".4", "odd(3)", ".1", ".3", ".5")
	if !m.Grouped() {
		t.Fatal("Grouped() = false")
	}
	m = applyGroups(t, m, nil)
	if m.Grouped() || m.shown() != 6 {
		t.Fatalf("flat again: grouped %v, shown %d", m.Grouped(), m.shown())
	}
}

func TestGroupsSortByShownLabel(t *testing.T) {
	m := load(t, newSource(6, 6), WithSize(40, 20))
	key := func(it item) string {
		if idNum(it) == 0 {
			return ""
		}
		return parity(it)
	}
	label := func(k string) string {
		switch k {
		case "even":
			return "Zed"
		case "odd":
			return "alpha"
		}
		return "Aardvark"
	}
	m = applyGroups(t, m, []Group[item]{{Key: key, Label: label}})
	got := outline(m)
	want := []string{"alpha(3)", ".1", ".3", ".5", "Zed(2)", ".2", ".4"}
	if len(got) != len(want)+2 || !slices.Equal(got[:len(want)], want) || got[len(got)-1] != ".0" {
		t.Fatalf("lines = %q, want %q then the empty key's group last", got, want)
	}
}

func TestGroupsTwoLevels(t *testing.T) {
	m := load(t, newSource(12, 12), WithSize(40, 30))
	m = applyGroups(t, m, []Group[item]{{Key: parity}, {Key: third}})
	wantOutline(t, m,
		"even(6)", ".m0(2)", "..0", "..6", ".m1(2)", "..4", "..10", ".m2(2)", "..2", "..8",
		"odd(6)", ".m0(2)", "..3", "..9", ".m1(2)", "..1", "..7", ".m2(2)", "..5", "..11")
}

func TestGroupsOrderLabelAndNone(t *testing.T) {
	key := func(it item) string {
		if idNum(it) == 0 {
			return ""
		}
		return parity(it)
	}
	m := load(t, newSource(5, 5), WithSize(40, 20))
	m = applyGroups(t, m, []Group[item]{{Key: key, Label: strings.ToUpper, Desc: true}})
	wantOutline(t, m, "ODD(2)", ".1", ".3", "EVEN(2)", ".2", ".4", "none(1)", ".0")
}

func TestGroupsExpandCollapse(t *testing.T) {
	m := load(t, newSource(12, 12), WithSize(40, 30))
	m = applyGroups(t, m, []Group[item]{{Key: parity}, {Key: third}})
	m = keys(t, m, "home")

	m = apply(t, m, (*Model[item]).Collapse)
	wantOutline(t, m, "even(6)-", "odd(6)", ".m0(2)", "..3", "..9", ".m1(2)", "..1", "..7", ".m2(2)", "..5", "..11")
	if m.sel != 0 {
		t.Fatalf("sel = %d, want 0", m.sel)
	}
	m = apply(t, m, (*Model[item]).Expand)
	if got := len(m.lines); got != 20 {
		t.Fatalf("%d lines after Expand, want 20", got)
	}

	m = keys(t, m, "down", "down")
	m = apply(t, m, (*Model[item]).Collapse)
	// On an item, Collapse closes its group and goes to the header.
	if m.lines[m.sel].label != "m0" || m.lines[m.sel].open {
		t.Fatalf("cursor on %+v, want the closed m0 header", m.lines[m.sel])
	}

	m = apply(t, m, (*Model[item]).CollapseAll)
	wantOutline(t, m, "even(6)-", "odd(6)-")
	if m.sel != 0 {
		t.Fatalf("sel = %d after CollapseAll, want the header of the group it was in", m.sel)
	}
	m = apply(t, m, (*Model[item]).ExpandAll)
	if got := len(m.lines); got != 20 {
		t.Fatalf("%d lines after ExpandAll, want 20", got)
	}
}

func TestGroupsCursorKept(t *testing.T) {
	m := load(t, newSource(12, 6), WithSize(40, 30), WithKey(itemKey))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	// Select item 3.
	for it, _ := m.Selected(); it.id != "3"; it, _ = m.Selected() {
		m = keys(t, m, "down")
	}
	before := m.sel
	m = applyGroups(t, m, []Group[item]{{Key: third}, {Key: parity}})
	if it, ok := m.Selected(); !ok || it.id != "3" {
		t.Fatalf("selected %v %v after SetGroups, want item 3 (was row %d)", it, ok, before)
	}
	m = applyGroups(t, m, nil)
	if it, ok := m.Selected(); !ok || it.id != "3" || m.sel != 3 {
		t.Fatalf("selected %v at %d after flat again, want item 3 at 3", it, m.sel)
	}
}

func TestGroupsCursorKeptOnNewChunk(t *testing.T) {
	groups := []Group[item]{{Key: parity}}
	m := load(t, newSource(12, 6), WithSize(40, 5), WithKey(itemKey), WithPrefetch(1))
	m = applyGroups(t, m, groups)
	// The last item is 5, and fetching the next chunk puts new rows above it.
	m.sel = len(m.lines) - 2
	m = run(t, m, m.fetchMore())
	if m.Len() != 12 {
		t.Fatalf("Len() = %d, want the next chunk fetched", m.Len())
	}
	if it, ok := m.Selected(); !ok || it.id != "5" {
		t.Fatalf("selected %v %v after the chunk, want item 5", it, ok)
	}

	// A cursor on a header stays on it.
	m = keys(t, m, "home", "down", "down", "down", "down", "down", "down", "down")
	if !m.lines[m.sel].head || m.lines[m.sel].label != "odd" {
		t.Fatalf("cursor on %+v, want the odd header", m.lines[m.sel])
	}
	m = apply(t, m, (*Model[item]).Reload)
	if !m.lines[m.sel].head || m.lines[m.sel].label != "odd" {
		t.Fatalf("cursor on %+v after reload, want the odd header", m.lines[m.sel])
	}
}

func TestGroupsHeaderIsNotSelectedOrMarked(t *testing.T) {
	m := load(t, newSource(6, 6), WithSize(40, 20), WithKey(itemKey))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	// Grouping leaves the cursor on item 0.
	if it, ok := m.Selected(); !ok || it.id != "0" {
		t.Fatalf("Selected() = %v %v after grouping, want item 0", it, ok)
	}
	m = keys(t, m, "home")
	if _, ok := m.Selected(); ok {
		t.Fatal("Selected() true on a header")
	}
	if _, ok := m.Item(0); ok {
		t.Fatal("Item(0) true for a header")
	}
	m.toggleMark()
	if len(m.marks) != 0 {
		t.Fatalf("marks = %v after marking a header", m.marks)
	}
	m = keys(t, m, "down")
	if it, ok := m.Selected(); !ok || it.id != "0" {
		t.Fatalf("Selected() = %v %v, want item 0", it, ok)
	}
}

func TestGroupsCounts(t *testing.T) {
	m := load(t, newSource(40, 10), WithSize(40, 5))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	m = keys(t, m, "home")
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "even (5+)") {
		t.Fatalf("count lacks the +:\n%s", v)
	}
	for !m.Done() {
		m = keys(t, m, "end")
	}
	m = keys(t, m, "home")
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "even (20)") || strings.Contains(v, "+") {
		t.Fatalf("counts of a done feed:\n%s", v)
	}
}

func TestGroupsPrefetchCountsItems(t *testing.T) {
	m := load(t, newSource(30, 10), WithSize(40, 5))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	// Rows: even header, five items, odd header, five items. The next chunk is
	// fetched once five items or fewer follow the cursor.
	m = keys(t, m, "home", "down", "down", "down", "down")
	if m.Len() != 10 {
		t.Fatalf("Len() = %d with 6 items after the cursor, want 10", m.Len())
	}
	m = keys(t, m, "down")
	if m.Len() != 20 {
		t.Fatalf("Len() = %d with 5 items after the cursor, want 20", m.Len())
	}
}

func TestGroupsClosedGroupsDoNotLoadTheFeed(t *testing.T) {
	m := load(t, newSource(1000, 10), WithSize(40, 6))
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	m = apply(t, m, (*Model[item]).CollapseAll)
	m = keys(t, m, "end")
	if m.Done() || m.Len() >= 100 {
		t.Fatalf("Len() = %d, Done() = %v with every group closed, want a few chunks", m.Len(), m.Done())
	}
	n := m.Len()
	m = apply(t, m, (*Model[item]).ExpandAll)
	if m.Len() < n || m.Done() || m.Len() >= 100 {
		t.Fatalf("Len() = %d after opening the groups, want only what the window needs", m.Len())
	}
}

func TestGroupsView(t *testing.T) {
	tests := []struct {
		name  string
		model func(t *testing.T) Model[item]
	}{
		{"one level", func(t *testing.T) Model[item] {
			t.Helper()
			m := load(t, newSource(6, 6), WithSize(30, 9))
			return applyGroups(t, m, []Group[item]{{Key: parity}})
		}},
		{"two levels, cursor on item", func(t *testing.T) Model[item] {
			t.Helper()
			m := load(t, newSource(12, 12), WithSize(30, 12))
			m = applyGroups(t, m, []Group[item]{{Key: parity}, {Key: third}})
			return keys(t, m, "down", "down", "down")
		}},
		{"collapsed", func(t *testing.T) Model[item] {
			t.Helper()
			m := load(t, newSource(12, 12), WithSize(30, 8))
			m = applyGroups(t, m, []Group[item]{{Key: parity}, {Key: third}})
			return apply(t, m, (*Model[item]).CollapseAll)
		}},
		{"scrolled to the end", func(t *testing.T) Model[item] {
			t.Helper()
			m := load(t, newSource(12, 6), WithSize(30, 9))
			m = applyGroups(t, m, []Group[item]{{Key: parity}})
			m, _ = m.Update(press("end"))
			return m
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model(t)
			v := m.View()
			assertFits(t, v, m.Width(), m.Height())
			golden.RequireEqual(t, v)
		})
	}
}
