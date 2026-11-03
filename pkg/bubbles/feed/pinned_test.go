package feed

import "testing"

func pins(ids ...string) []item {
	out := make([]item, 0, len(ids))
	for _, id := range ids {
		out = append(out, item{id: id, title: "pin " + id})
	}
	return out
}

func loadPinned(t *testing.T, src *source, p []item) Model[item] {
	t.Helper()
	return load(t, src, WithPinned(p), WithKey(itemKey), WithMarkKeys(testMarkKeys))
}

func TestPinnedState(t *testing.T) {
	t.Parallel()
	m := loadPinned(t, newSource(10, 10), pins("p0", "p1"))
	if m.Pinned() != 2 {
		t.Fatalf("Pinned = %d, want 2", m.Pinned())
	}
	if m.sel != 2 {
		t.Fatalf("starts on row %d, want 2", m.sel)
	}
	if m.Len() != 10 || m.shown() != 12 {
		t.Fatalf("Len %d, shown %d: pins must not be items", m.Len(), m.shown())
	}
	if m.at(0) != -1 || m.at(1) != -1 || m.at(2) != 0 {
		t.Fatalf("at: %d %d %d", m.at(0), m.at(1), m.at(2))
	}
	if m.posOf(0) != 2 || m.posOf(-5) != -1 {
		t.Fatalf("posOf: %d %d", m.posOf(0), m.posOf(-5))
	}
}

func TestPinnedCursor(t *testing.T) {
	t.Parallel()
	m := loadPinned(t, newSource(10, 10), pins("p0", "p1"))
	m = keys(t, m, "up")
	if i, ok := m.SelectedPinned(); !ok || i != 1 {
		t.Fatalf("SelectedPinned = %d, %v", i, ok)
	}
	if it, ok := m.Selected(); !ok || it.id != "p1" {
		t.Fatalf("Selected = %v, %v", it, ok)
	}
	if m.Index() != -1 {
		t.Fatalf("Index = %d on a pin", m.Index())
	}
	if _, _, ok := m.SelectedChild(); ok {
		t.Fatal("SelectedChild ok on a pin")
	}
	m = keys(t, m, "up", "up")
	if m.sel != 0 {
		t.Fatalf("up past the first pin: sel %d", m.sel)
	}
	m = keys(t, m, "down", "down")
	if i, ok := m.SelectedPinned(); ok {
		t.Fatalf("down across the boundary stays on pin %d", i)
	}
	if m.Index() != 0 {
		t.Fatalf("Index = %d, want 0", m.Index())
	}
	m = keys(t, m, "end")
	if m.Index() != 9 {
		t.Fatalf("end: Index %d", m.Index())
	}
	m = keys(t, m, "home")
	if m.sel != 2 {
		t.Fatalf("home goes to row %d, want the first of the list, 2", m.sel)
	}
}

func TestPinnedInert(t *testing.T) {
	t.Parallel()
	m := loadPinned(t, newSource(10, 10), pins("p0"))
	m = keys(t, m, "up", "space", "l", "h", "*")
	if _, ok := m.SelectedPinned(); !ok {
		t.Fatal("cursor left the pin")
	}
	if len(m.marks) != 0 {
		t.Fatalf("a pin was marked: %v", m.marks)
	}
}

func TestSetPinned(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		start   []string
		keys    []string
		set     []string
		wantSel int
	}{
		{"shrinks under the cursor", []string{"a", "b", "c"}, []string{"up"}, []string{"a"}, 0},
		{"shrinks 3 to 2 with the cursor on the third", []string{"a", "b", "c"}, []string{"up"}, []string{"a", "b"}, 1},
		{"cursor on a removed pin goes to the last", []string{"a", "b", "c"}, []string{"up", "up", "up"}, nil, 0},
		{"the list's row keeps its item", []string{"a"}, []string{"j"}, []string{"a", "b"}, 3},
		{"a pin before the cursor stays", []string{"a", "b"}, []string{"up", "up"}, []string{"a", "b", "c"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := newSource(10, 10)
			m := loadPinned(t, src, pins(tt.start...))
			m = keys(t, m, tt.keys...)
			calls := src.callCount()
			m.SetPinned(pins(tt.set...))
			if m.sel != tt.wantSel {
				t.Fatalf("sel = %d, want %d", m.sel, tt.wantSel)
			}
			if m.Pinned() != len(tt.set) {
				t.Fatalf("Pinned = %d", m.Pinned())
			}
			if src.callCount() != calls {
				t.Fatal("SetPinned fetched")
			}
			assertVisible(t, m)
		})
	}
}

func TestPinnedOnly(t *testing.T) {
	t.Parallel()
	src := newSource(0, 10)
	m := loadPinned(t, src, pins("a", "b"))
	calls := src.callCount()
	m = keys(t, m, "down", "down", "up", "G", "g")
	if src.callCount() != calls {
		t.Fatalf("a pin-only feed fetched %d more times", src.callCount()-calls)
	}
	if m.Len() != 0 {
		t.Fatalf("Len = %d", m.Len())
	}
}

func TestPinnedFind(t *testing.T) {
	t.Parallel()
	m := loadPinned(t, bugs(20), pins("p0", "p1"))
	m = keys(t, m, "up")
	m = typed(t, m, "/", "bug", "enter")
	if got := selectedID(t, m); got != "2" {
		t.Fatalf("find from a pin selected %q, want 2", got)
	}
	m = keys(t, m, "N")
	if got := selectedID(t, m); got != "7" {
		t.Fatalf("N selected %q, want 7", got)
	}
	m = keys(t, m, "n")
	if got := selectedID(t, m); got != "12" {
		t.Fatalf("n selected %q, want 12", got)
	}
}

func TestPinnedFilter(t *testing.T) {
	t.Parallel()
	m := loadPinned(t, bugs(20), pins("p0", "p1"))
	m = typed(t, m, "&", "bug", "enter")
	if m.Pinned() != 2 {
		t.Fatal("the filter dropped the pins")
	}
	if m.sel < 2 {
		t.Fatalf("filter left the cursor on row %d", m.sel)
	}
	if got := selectedID(t, m); got != "2" {
		t.Fatalf("selected %q, want 2", got)
	}
	m = keys(t, m, "up", "up")
	if i, ok := m.SelectedPinned(); !ok || i != 0 {
		t.Fatalf("cursor not on the first pin: %d %v", i, ok)
	}
	m = typed(t, m, "esc")
	if m.Pinned() != 2 || m.sel < 0 {
		t.Fatal("clearing the filter broke the pins")
	}
}

func TestPinnedGrouped(t *testing.T) {
	t.Parallel()
	m := loadGrouped(t, newSource(6, 10), []Group[item]{{Key: parity}}, nil, WithPinned(pins("p0")))
	if m.sel < 1 {
		t.Fatalf("starts on row %d", m.sel)
	}
	if !m.onHeader() {
		t.Fatal("first list row is not a header")
	}
	m = keys(t, m, "up")
	if m.onHeader() || !m.onPin() {
		t.Fatal("the pin read as a header")
	}
	m = keys(t, m, "l", "h", "*")
	if !m.onPin() {
		t.Fatal("fold keys moved the cursor off the pin")
	}
	m = keys(t, m, "down", "down")
	if it, ok := m.Selected(); !ok || it.id != "0" {
		t.Fatalf("Selected = %v, %v", it, ok)
	}
	m = keys(t, m, "end")
	assertVisible(t, m)
}
