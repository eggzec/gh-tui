package feed

import (
	"strconv"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/golden"
)

func kidRender(it item, k int, _ bool, _ int) string {
	return "kid " + it.id + "." + strconv.Itoa(k)
}

// withKidCounts gives items the number of child rows that counts holds by id.
func withKidCounts(counts map[string]int) Option {
	return WithChildren(func(it item) int { return counts[it.id] }, kidRender)
}

// withKids gives the second item two children and the fourth one.
func withKids() Option { return withKidCounts(map[string]int{"1": 2, "3": 1}) }

// kidsModel is six items whose second and fourth own children: nine lines.
func kidsModel(t *testing.T) Model[item] {
	t.Helper()
	return load(t, newSource(6, 6), WithSize(30, 12), withKids())
}

// onKid fails unless the cursor is on child k of item 1.
func onKid(t *testing.T, m Model[item], k int) {
	t.Helper()
	it, gk, ok := m.SelectedChild()
	if !ok || it.id != "1" || gk != k {
		t.Fatalf("SelectedChild = %q, %d, %v; want \"1\", %d", it.id, gk, ok, k)
	}
}

func activate(t *testing.T, m Model[item]) Model[item] {
	t.Helper()
	cmd, _ := m.Activate()
	return run(t, m, cmd)
}

func TestChildrenBuild(t *testing.T) {
	m := kidsModel(t)
	if got := len(m.lines); got != 9 {
		t.Fatalf("%d lines, want 9", got)
	}
	for i, want := range []bool{false, false, true, true, false, false, true, false, false} {
		if m.lines[i].child != want {
			t.Fatalf("line %d child = %v, want %v", i, m.lines[i].child, want)
		}
	}
	if l := m.lines[3]; l.k != 1 || l.item != 1 {
		t.Fatalf("line 3 is child %d of item %d, want 1 of 1", l.k, l.item)
	}
	assertFits(t, m.View(), m.Width(), m.Height())
}

func TestChildrenKeys(t *testing.T) {
	m := kidsModel(t)
	m = keys(t, m, "home", "down")
	if _, _, ok := m.SelectedChild(); ok {
		t.Fatal("SelectedChild on an item")
	}
	// l on an open item steps to its first child, and on a child does nothing.
	m = keys(t, m, "l")
	onKid(t, m, 0)
	m = keys(t, m, "l")
	onKid(t, m, 0)
	if _, ok := m.Selected(); ok {
		t.Fatal("Selected is true on a child")
	}
	// Enter does nothing on a child.
	m = activate(t, m)
	if len(m.lines) != 9 {
		t.Fatalf("%d lines after enter on a child, want 9", len(m.lines))
	}
	// h on a child goes to its item, and h on an open item closes it.
	m = keys(t, m, "h")
	if it, ok := m.Selected(); !ok || it.id != "1" {
		t.Fatalf("not on item 1 after h on a child")
	}
	m = keys(t, m, "h")
	if len(m.lines) != 7 || m.lines[m.sel].open {
		t.Fatalf("%d lines, open %v after h on an open item; want 7 and closed", len(m.lines), m.lines[m.sel].open)
	}
	// h on a closed item with no parent stays, and l opens it.
	m = keys(t, m, "h")
	if it, _ := m.Selected(); it.id != "1" {
		t.Fatalf("moved to %q", it.id)
	}
	m = keys(t, m, "l")
	if len(m.lines) != 9 {
		t.Fatalf("%d lines after l on a closed item, want 9", len(m.lines))
	}
	// Enter toggles the item.
	m = activate(t, m)
	if len(m.lines) != 7 {
		t.Fatalf("%d lines after enter, want 7", len(m.lines))
	}
	m = activate(t, m)
	if len(m.lines) != 9 {
		t.Fatalf("%d lines after enter again, want 9", len(m.lines))
	}
	// A leaf item ignores enter.
	m = keys(t, m, "home")
	if _, ok := m.Activate(); ok {
		t.Fatal("Activate handled a leaf item")
	}
}

func TestChildrenToggleAll(t *testing.T) {
	m := kidsModel(t)
	m = keys(t, m, "home", "down", "l")
	onKid(t, m, 0)
	m = apply(t, m, (*Model[item]).toggleAll)
	if len(m.lines) != 6 {
		t.Fatalf("%d lines after closing all, want 6", len(m.lines))
	}
	if it, ok := m.Selected(); !ok || it.id != "1" {
		t.Fatal("cursor not on item 1 after closing all")
	}
	m = apply(t, m, (*Model[item]).toggleAll)
	if len(m.lines) != 9 {
		t.Fatalf("%d lines after opening all, want 9", len(m.lines))
	}
}

func TestChildrenCursorKept(t *testing.T) {
	counts := map[string]int{"1": 2, "3": 1}
	m := load(t, newSource(6, 6), WithSize(30, 12), withKidCounts(counts))
	m = keys(t, m, "home", "down", "down", "down")
	onKid(t, m, 1)
	m.Resort()
	onKid(t, m, 1)

	// The child goes away: the cursor falls back to its item.
	counts["1"] = 1
	m.Resort()
	if it, ok := m.Selected(); !ok || it.id != "1" {
		t.Fatal("cursor not on item 1 after its child went")
	}
}

func TestChildrenFindSkipsChildren(t *testing.T) {
	m := kidsModel(t)
	m = keys(t, m, "home", "/", "kid", "enter")
	if _, _, ok := m.SelectedChild(); ok {
		t.Fatal("find stopped on a child")
	}
}

func TestChildrenOffIsUnchanged(t *testing.T) {
	m := load(t, newSource(6, 6), WithSize(30, 12))
	if m.lined() || len(m.lines) != 0 {
		t.Fatal("line model in use without groups or children")
	}
	if _, _, ok := m.SelectedChild(); ok {
		t.Fatal("SelectedChild without children")
	}
}

func TestChildrenView(t *testing.T) {
	tests := []struct {
		name  string
		model func(t *testing.T) Model[item]
	}{
		{"open", func(t *testing.T) Model[item] {
			t.Helper()
			return kidsModel(t)
		}},
		{"cursor on child", func(t *testing.T) Model[item] {
			t.Helper()
			return keys(t, kidsModel(t), "home", "down", "down")
		}},
		{"closed", func(t *testing.T) Model[item] {
			t.Helper()
			return apply(t, kidsModel(t), (*Model[item]).toggleAll)
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

// bugTitled is a source of six items whose items at the given indexes are
// titled "bug".
func bugTitled(at ...int) *source {
	src := newSource(6, 6)
	for _, i := range at {
		src.items[i].title = "bug"
	}
	return src
}

// foldOf returns whether item 1 is open, and fails if it is not a foldable
// one.
func foldOf(t *testing.T, m Model[item]) bool {
	t.Helper()
	const id = "1"
	for _, l := range m.lines {
		if l.head || l.child || l.more {
			continue
		}
		if it, _ := m.item(l.item); it.id == id {
			if l.kids == 0 {
				t.Fatalf("item %s has no children", id)
			}
			return l.open
		}
	}
	t.Fatalf("item %s has no line", id)
	return false
}

// lineItem returns the id of the item the cursor line is, or is under.
func lineItem(t *testing.T, m Model[item]) string {
	t.Helper()
	l := m.lines[m.sel]
	if l.head || l.more {
		t.Fatalf("cursor on %+v, want an item or a child", l)
	}
	it, _ := m.item(l.item)
	return it.id
}

func TestChildrenFilterWithCursorOnChild(t *testing.T) {
	m := load(t, bugTitled(1, 4), WithSize(30, 12), withKids())
	m = keys(t, m, "home", "down", "down")
	onKid(t, m, 0)
	m = typed(t, m, "&", "bug", "enter")
	// Items 1 and 4 stay, with the children of item 1.
	if len(m.lines) != 4 {
		t.Fatalf("%d lines under the filter, want 4", len(m.lines))
	}
	if got := lineItem(t, m); got != "1" {
		t.Fatalf("cursor under item %s after the filter, want 1", got)
	}
	m = keys(t, m, "esc")
	if len(m.lines) != 9 {
		t.Fatalf("%d lines after clearing the filter, want 9", len(m.lines))
	}
	if got := lineItem(t, m); got != "1" {
		t.Fatalf("cursor under item %s after clearing the filter, want 1", got)
	}
	assertFits(t, m.View(), m.Width(), m.Height())
}

func TestChildrenFilterDropsItemWithOpenChildren(t *testing.T) {
	m := load(t, bugTitled(4, 5), WithSize(30, 12), withKids())
	m = keys(t, m, "home", "down", "down")
	onKid(t, m, 0)
	m = typed(t, m, "&", "bug", "enter")
	if len(m.lines) != 2 {
		t.Fatalf("%d lines under the filter, want 2", len(m.lines))
	}
	for _, l := range m.lines {
		if l.child {
			t.Fatal("a child line stays when its item is filtered out")
		}
	}
	if _, ok := m.Selected(); !ok {
		t.Fatal("no item selected after the filter dropped the cursor's item")
	}
	assertFits(t, m.View(), m.Width(), m.Height())
	m = keys(t, m, "esc")
	if len(m.lines) != 9 {
		t.Fatalf("%d lines after clearing the filter, want 9", len(m.lines))
	}
}

func TestChildrenMarks(t *testing.T) {
	m := load(t, newSource(6, 6), WithSize(30, 12), withKids(), WithKey(byID))
	// A child is never marked, and the key leaves the cursor where it is.
	m = keys(t, m, "home", "down", "down")
	onKid(t, m, 0)
	m = keys(t, m, "space")
	onKid(t, m, 0)
	if got := m.Marks(); got != 0 {
		t.Fatalf("%d marks after the mark key on a child, want 0", got)
	}
	// A foldable item is marked like any other, and the cursor moves on.
	m = keys(t, m, "up", "space")
	if got := m.Marks(); got != 1 || !m.Marked(item{id: "1"}) {
		t.Fatalf("%d marks, item 1 marked %v; want 1 and true", got, m.Marked(item{id: "1"}))
	}
	m = keys(t, m, "down")
	if _, _, ok := m.SelectedChild(); !ok {
		if _, ok := m.Selected(); !ok {
			t.Fatal("cursor is on neither an item nor a child")
		}
	}
	if got := m.Marks(); got != 1 {
		t.Fatalf("%d marks after moving, want 1", got)
	}
}

// groupedKids is a feed of parity groups whose items 1 and 3 own children.
func groupedKids(t *testing.T) Model[item] {
	t.Helper()
	m := load(t, newSource(6, 6), WithSize(40, 20), withKids())
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	wantOutline(t, m, "even(3)", ".0", ".2", ".4", "odd(3)", ".1", "..1", "..1", ".3", "..3", ".5")
	return m
}

func TestChildrenGroupsLeft(t *testing.T) {
	m := groupedKids(t)
	m = keys(t, m, "home", "down", "down", "down", "down", "down", "down")
	onKid(t, m, 0)
	// h on a child goes to its item, and on an open item closes it.
	m = keys(t, m, "h")
	if got := lineItem(t, m); got != "1" || m.lines[m.sel].child {
		t.Fatalf("not on item 1 after h on a child")
	}
	m = keys(t, m, "h")
	wantOutline(t, m, "even(3)", ".0", ".2", ".4", "odd(3)", ".1", ".3", "..3", ".5")
	// h on a closed item goes to the header of its group.
	m = keys(t, m, "h")
	if l := m.lines[m.sel]; !l.head || l.label != "odd" {
		t.Fatalf("cursor on %+v, want the odd header", l)
	}
	// The fold survives closing and opening the group.
	m = keys(t, m, "h")
	wantOutline(t, m, "even(3)", ".0", ".2", ".4", "odd(3)-")
	m = keys(t, m, "l")
	wantOutline(t, m, "even(3)", ".0", ".2", ".4", "odd(3)", ".1", ".3", "..3", ".5")
	if foldOf(t, m) {
		t.Fatal("item 1 is open after its group was closed and opened")
	}
}

func TestChildrenGroupsToggleAll(t *testing.T) {
	m := groupedKids(t)
	m = keys(t, m, "home", "down", "down", "down", "down", "down", "h")
	wantOutline(t, m, "even(3)", ".0", ".2", ".4", "odd(3)", ".1", ".3", "..3", ".5")
	// Item 1 is closed, so toggling opens everything.
	m = apply(t, m, (*Model[item]).toggleAll)
	wantOutline(t, m, "even(3)", ".0", ".2", ".4", "odd(3)", ".1", "..1", "..1", ".3", "..3", ".5")
	// Closing all closes the groups, which hides the folds of the items.
	m = apply(t, m, (*Model[item]).toggleAll)
	wantOutline(t, m, "even(3)-", "odd(3)-")
	if !m.lines[m.sel].head {
		t.Fatalf("cursor on %+v after closing all, want a header", m.lines[m.sel])
	}
	m = apply(t, m, (*Model[item]).toggleAll)
	wantOutline(t, m, "even(3)", ".0", ".2", ".4", "odd(3)", ".1", "..1", "..1", ".3", "..3", ".5")
}

func TestChildrenFoldsKeptBySortResetByGroups(t *testing.T) {
	m := groupedKids(t)
	m = keys(t, m, "home", "down", "down", "down", "down", "down", "h")
	if foldOf(t, m) {
		t.Fatal("item 1 is open after h")
	}
	byDesc := func(a, b item) int { return idNum(b) - idNum(a) }
	m = apply(t, m, func(m *Model[item]) tea.Cmd { return m.SetSort(byDesc) })
	if foldOf(t, m) {
		t.Fatal("SetSort opened the fold of item 1")
	}
	m = applyGroups(t, m, []Group[item]{{Key: parity}})
	if !foldOf(t, m) {
		t.Fatal("SetGroups kept the fold of item 1")
	}
}

func TestChildrenFind(t *testing.T) {
	m := load(t, bugTitled(1, 3, 5), WithSize(30, 12), withKids())
	// A hit on an item whose children are closed.
	m = keys(t, m, "home", "down", "h")
	if len(m.lines) != 7 {
		t.Fatalf("%d lines with item 1 closed, want 7", len(m.lines))
	}
	m = typed(t, keys(t, m, "home"), "/", "bug", "enter")
	if got := selectedID(t, m); got != "1" {
		t.Fatalf("find stopped on item %s, want the closed item 1", got)
	}
	m = keys(t, m, "l", "l")
	onKid(t, m, 0)
	// From a child, n goes to the next hit after its item, not to its item.
	m = keys(t, m, "n")
	if got := selectedID(t, m); got != "3" {
		t.Fatalf("n from a child of item 1 stopped on item %s, want 3", got)
	}
	m = keys(t, m, "l", "l")
	if it, k, ok := m.SelectedChild(); !ok || it.id != "3" || k != 0 {
		t.Fatalf("not on the child of item 3: %v %d %v", it, k, ok)
	}
	// N from a child goes back to the hit above it, which is its own item.
	m = keys(t, m, "N")
	if got := selectedID(t, m); got != "3" {
		t.Fatalf("N from a child of item 3 stopped on item %s, want 3", got)
	}
	m = keys(t, m, "N")
	if got := selectedID(t, m); got != "1" {
		t.Fatalf("N from item 3 stopped on item %s, want 1", got)
	}
}

func TestChildrenFoldPathUsesKey(t *testing.T) {
	path := func(m Model[item]) string {
		for _, l := range m.lines {
			if l.kids > 0 {
				return l.path
			}
		}
		t.Fatal("no foldable line")
		return ""
	}
	keyless := load(t, newSource(6, 6), WithSize(30, 12), withKids())
	if got := path(keyless); got != "\x1f1" {
		t.Fatalf("keyless path %q, want the index", got)
	}
	keyed := load(t, newSource(6, 6), WithSize(30, 12), withKids(),
		WithKey(func(it item) string { return "k" + it.id }))
	if got := path(keyed); got != "\x1fk1" {
		t.Fatalf("keyed path %q, want the key", got)
	}

	// Reload with two items in front: a keyed fold stays on its item.
	src := newSource(6, 6)
	m := load(t, src, WithSize(30, 12), withKids(), WithKey(byID))
	m = keys(t, m, "home", "down", "h")
	src.insert(2)
	m = run(t, m, m.Reload())
	if foldOf(t, m) {
		t.Fatal("the fold of item 1 did not follow it through a reload")
	}
}

// A fold marker is as wide as its blank stand-in, so that items line up.
func TestChildrenMarkerWidth(t *testing.T) {
	m := kidsModel(t)
	if w := lipgloss.Width(m.groupOpen); w != m.foldWidth {
		t.Errorf("open marker is %d cells, foldWidth %d", w, m.foldWidth)
	}
	if w := lipgloss.Width(m.groupClosed); w != m.foldWidth {
		t.Errorf("closed marker is %d cells, foldWidth %d", w, m.foldWidth)
	}
	if w := lipgloss.Width(m.blankFold); w != m.foldWidth {
		t.Errorf("blank stand-in is %d cells, foldWidth %d", w, m.foldWidth)
	}
	if m.foldWidth != 2 {
		t.Errorf("foldWidth = %d, want a glyph and a space", m.foldWidth)
	}
}
