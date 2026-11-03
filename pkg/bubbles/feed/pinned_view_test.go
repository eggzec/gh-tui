package feed

import (
	"strings"
	"testing"
)

func TestPinnedView(t *testing.T) {
	t.Parallel()
	t.Run("flat", func(t *testing.T) {
		t.Parallel()
		m := loadPinned(t, newSource(3, 10), pins("p0", "p1"))
		m = keys(t, m, "end")
		out := m.View()
		if !strings.Contains(out, "pin p0") || !strings.Contains(out, "#2 item 2") {
			t.Fatalf("view misses rows:\n%s", out)
		}
	})
	t.Run("grouped reaching the end", func(t *testing.T) {
		t.Parallel()
		m := loadGrouped(t, newSource(6, 10), []Group[item]{{Key: parity}}, nil, WithPinned(pins("p0")))
		out := m.View()
		if !strings.Contains(out, "pin p0") || !strings.Contains(out, "#5") {
			t.Fatalf("view misses rows:\n%s", out)
		}
		m = keys(t, m, "end")
		_ = m.View()
	})
	t.Run("pins only", func(t *testing.T) {
		t.Parallel()
		m := loadPinned(t, newSource(0, 10), pins("p0"))
		_ = m.View()
	})
}

func TestPinnedItemOrigin(t *testing.T) {
	t.Parallel()
	m := loadPinned(t, newSource(10, 10), pins("p0", "p1"))
	m = keys(t, m, "down", "down")
	got, ok := m.Item(m.Index())
	want, _ := m.Selected()
	if !ok || got != want {
		t.Fatalf("Item(Index()) = %v, want %v", got, want)
	}
	if m.Shown() != 10 {
		t.Fatalf("Shown = %d, want 10", m.Shown())
	}
	if it, ok := m.Item(0); !ok || it.id != "0" {
		t.Fatalf("Item(0) = %v, %v", it, ok)
	}
}

func TestPinnedStaysOnReindex(t *testing.T) {
	t.Parallel()
	m := loadPinned(t, bugs(30), pins("p0", "p1"))
	m = typed(t, m, "&", "bug", "enter")
	m = keys(t, m, "up", "up", "up")
	if _, ok := m.SelectedPinned(); !ok {
		t.Fatal("cursor not on a pin")
	}
	sel := m.sel
	m.reindex()
	if m.sel != sel {
		t.Fatalf("a chunk moved the cursor from %d to %d", sel, m.sel)
	}
	m = typed(t, m, "&", "zzz", "enter")
	if m.sel != sel {
		t.Fatalf("a filter moved the cursor from %d to %d", sel, m.sel)
	}
	m = typed(t, m, "esc", "esc")
	if m.sel != sel {
		t.Fatalf("clearing the filter moved the cursor from %d to %d", sel, m.sel)
	}
}

func TestPinnedActivateChildren(t *testing.T) {
	t.Parallel()
	kids := WithChildren(func(item) int { return 2 }, func(_ item, _ int, _ bool, _ int) string { return "kid" })
	m := load(t, newSource(3, 10), WithPinned(pins("p0")), WithKey(itemKey), kids)
	m = keys(t, m, "up")
	if _, taken := m.Activate(); taken {
		t.Fatal("Enter on a pin was taken")
	}
	m = keys(t, m, "down")
	if _, taken := m.Activate(); !taken {
		t.Fatal("Enter on a foldable item was not taken")
	}
	if l, ok := m.cur(); !ok || l.open {
		t.Fatalf("the item under the cursor did not fold: %v %v", l, ok)
	}
}
