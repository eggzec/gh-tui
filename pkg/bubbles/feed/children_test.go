package feed

import (
	"strconv"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// kidCounts gives the number of child rows of an item, by id.
var kidCounts = map[string]int{"1": 2, "3": 1}

func kidCount(it item) int { return kidCounts[it.id] }

func kidRender(it item, k int, _ bool, _ int) string {
	return "kid " + it.id + "." + strconv.Itoa(k)
}

func withKids() Option { return WithChildren(kidCount, kidRender) }

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
	m := kidsModel(t)
	m = keys(t, m, "home", "down", "down", "down")
	onKid(t, m, 1)
	m.Resort()
	onKid(t, m, 1)

	// The child goes away: the cursor falls back to its item.
	kidCounts["1"] = 1
	defer func() { kidCounts["1"] = 2 }()
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
