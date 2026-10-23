package feed

import "testing"

// loadGrouped builds a feed grouped from the start, so the first chunk arrives
// into grouped lines, and fetches it.
func loadGrouped(t *testing.T, src *source, groups []Group[item], closed []string, opts ...Option) Model[item] {
	t.Helper()
	opts = append([]Option{WithSize(40, 30), WithFocused(true), WithKey(itemKey)}, opts...)
	m := newModel(src.fetch, renderItem, opts...)
	_ = m.SetGroups(groups)
	if m.closed == nil {
		m.closed = map[string]struct{}{}
	}
	for _, k := range closed {
		m.closed["\x1f"+k] = struct{}{}
	}
	return run(t, m, m.Init())
}

func TestWithFirstItem(t *testing.T) {
	t.Parallel()
	byParity := []Group[item]{{Key: parity}}
	tests := []struct {
		name    string
		src     *source
		groups  []Group[item]
		closed  []string
		opts    []Option
		wantSel int
		// wantHead is whether the cursor ends on a header.
		wantHead bool
	}{
		{"grouped with the option starts on the first item", newSource(6, 6), byParity, nil, []Option{WithFirstItem()}, 1, false},
		{"grouped without the option stays on the header", newSource(6, 6), byParity, nil, nil, 0, true},
		{"ungrouped is not moved", newSource(6, 6), nil, nil, []Option{WithFirstItem()}, 0, false},
		{"a closed first group leaves the cursor on a header", newSource(6, 6), byParity, []string{"even", "odd"}, []Option{WithFirstItem()}, 0, true},
		{"the first item may come after a closed group", newSource(6, 6), byParity, []string{"even"}, []Option{WithFirstItem()}, 2, false},
		{"headers over a more row leave a valid cursor", newSource(12, 6), byParity, []string{"even", "odd"}, []Option{WithFirstItem()}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := loadGrouped(t, tt.src, tt.groups, tt.closed, tt.opts...)
			if m.sel != tt.wantSel {
				t.Fatalf("sel = %d, want %d (lines %q)", m.sel, tt.wantSel, outline(m))
			}
			if m.sel < 0 || m.sel >= max(len(m.lines), 1) {
				t.Fatalf("sel = %d is outside the %d lines", m.sel, len(m.lines))
			}
			if len(m.lines) > 0 && m.lines[m.sel].head != tt.wantHead {
				t.Fatalf("cursor on %+v, want header = %v", m.lines[m.sel], tt.wantHead)
			}
		})
	}
}

func TestWithFirstItemLaterChunksKeepTheCursor(t *testing.T) {
	t.Parallel()
	m := loadGrouped(t, newSource(12, 6), []Group[item]{{Key: parity}}, nil, WithFirstItem(), WithPrefetch(1))
	if it, ok := m.Selected(); !ok || it.id != "0" {
		t.Fatalf("selected %v %v, want item 0", it, ok)
	}
	// Back on the header: the next chunk must not move it to an item.
	m = keys(t, m, "home")
	m = run(t, m, m.fetchMore())
	if m.Len() != 12 {
		t.Fatalf("Len() = %d, want the next chunk fetched", m.Len())
	}
	if !m.lines[m.sel].head || m.lines[m.sel].label != "even" {
		t.Fatalf("cursor on %+v after the chunk, want the even header", m.lines[m.sel])
	}
}
