package feed

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// pinFits checks that the view is height lines of width cells.
func pinFits(t *testing.T, m Model[item]) string {
	t.Helper()
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != m.Height() {
		t.Fatalf("view has %d lines, want %d:\n%s", len(lines), m.Height(), out)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.Width() {
			t.Fatalf("line %d is %d cells, want %d: %q", i, w, m.Width(), l)
		}
	}
	return out
}

func TestPinnedViewFits(t *testing.T) {
	t.Parallel()
	t.Run("flat with the cursor on a pin", func(t *testing.T) {
		t.Parallel()
		m := loadPinned(t, newSource(5, 10), pins("p0", "p1"))
		m = keys(t, m, "up")
		if _, ok := m.SelectedPinned(); !ok {
			t.Fatal("cursor not on a pin")
		}
		if m.SelectedRow() < 0 {
			t.Fatal("SelectedRow is out of view on a pin")
		}
		out := pinFits(t, m)
		if !strings.Contains(out, "pin p1") {
			t.Fatalf("view misses the pin:\n%s", out)
		}
	})
	t.Run("pins only", func(t *testing.T) {
		t.Parallel()
		m := loadPinned(t, newSource(0, 10), pins("p0"))
		out := pinFits(t, m)
		if !strings.Contains(out, "pin p0") {
			t.Fatalf("view misses the pin:\n%s", out)
		}
		if m.Loading() {
			t.Fatal("a feed with pins is loading")
		}
		if m.Dimmed() == nil {
			t.Fatal("a feed with pins has no dimmed lines")
		}
	})
	t.Run("filtered to zero", func(t *testing.T) {
		t.Parallel()
		m := loadPinned(t, bugs(30), pins("p0", "p1"))
		m = typed(t, m, "&", "zzz", "enter")
		out := pinFits(t, m)
		if !strings.Contains(out, "pin p0") || !strings.Contains(out, "No loaded") {
			t.Fatalf("view misses the pins or the status:\n%s", out)
		}
	})
	t.Run("grouped", func(t *testing.T) {
		t.Parallel()
		m := loadGrouped(t, newSource(6, 10), []Group[item]{{Key: parity}}, nil, WithPinned(pins("p0", "p1")))
		pinFits(t, m)
	})
	t.Run("children reserve the fold marker", func(t *testing.T) {
		t.Parallel()
		kids := WithChildren(func(item) int { return 1 }, func(_ item, _ int, _ bool, _ int) string { return "kid" })
		m := load(t, newSource(3, 10), WithPinned(pins("p0")), WithKey(itemKey), kids)
		out := pinFits(t, m)
		pin, list := -1, -1
		for l := range strings.SplitSeq(ansi.Strip(out), "\n") {
			if i := strings.Index(l, "pin p0"); i >= 0 {
				pin = i
			}
			if i := strings.Index(l, "#0 item 0"); i >= 0 {
				list = i
			}
		}
		if pin < 0 || pin != list {
			t.Fatalf("pin text at %d, item text at %d:\n%s", pin, list, out)
		}
	})
	t.Run("narrow", func(t *testing.T) {
		t.Parallel()
		m := loadPinned(t, newSource(5, 10), pins("p0", "p1"))
		m.SetSize(8, 6)
		pinFits(t, m)
	})
	t.Run("wide gutter is cut to the list's", func(t *testing.T) {
		t.Parallel()
		m := load(t, newSource(3, 10), WithPinned(pins("p0")), WithKey(itemKey), WithMarkKeys(testMarkKeys),
			WithPinnedGutter(func(_, _ bool) string { return "*******" }))
		pinFits(t, m)
	})
}

func BenchmarkViewPinned(b *testing.B) {
	for _, n := range []int{0, 3} {
		b.Run(strconv.Itoa(n)+" pins", func(b *testing.B) {
			ids := make([]string, n)
			for i := range ids {
				ids[i] = "p" + strconv.Itoa(i)
			}
			m := load(b, newSource(1_000, 100), WithSize(100, 40), WithPinned(pins(ids...)), WithKey(itemKey))
			b.ReportAllocs()
			for b.Loop() {
				_ = m.View()
			}
		})
	}
}

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
