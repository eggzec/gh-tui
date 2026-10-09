package feed

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// byID identifies an item, so that the rows can be marked.
func byID(it item) string { return it.id }

// marking is a feed over src whose rows can be marked.
func marking(tb testing.TB, src *source) Model[item] {
	tb.Helper()
	return load(tb, src, WithKey(byID))
}

func TestMarkToggles(t *testing.T) {
	m := marking(t, newSource(10, 10))
	m = keys(t, m, "space")
	if got := m.Marks(); got != 1 {
		t.Fatalf("after space, %d items are marked, want 1", got)
	}
	if it, _ := m.Selected(); !m.Marked(it) {
		t.Errorf("the selected item %s is not marked", it.id)
	}
	// Marking does not move the cursor.
	if got := selectedID(t, m); got != "0" {
		t.Errorf("selected %s after marking, want 0", got)
	}
	m = keys(t, m, "j", "j", "space")
	if got := m.Marks(); got != 2 {
		t.Fatalf("%d items are marked, want 2", got)
	}
	m = keys(t, m, "space")
	if got := m.Marks(); got != 1 {
		t.Errorf("space on a marked row left %d marks, want 1", got)
	}
	if it, _ := m.Selected(); m.Marked(it) {
		t.Errorf("item %s is still marked", it.id)
	}
}

// Copies of a model share what they hold, so a mark must not show in the
// copy a parent kept.
func TestMarkDoesNotChangeCopies(t *testing.T) {
	before := marking(t, newSource(10, 10))
	after := keys(t, before, "space")
	if before.Marks() != 0 || after.Marks() != 1 {
		t.Errorf("marks: %d before, %d after; want 0 and 1", before.Marks(), after.Marks())
	}
	again := keys(t, after, "j", "space")
	if after.Marks() != 1 || again.Marks() != 2 {
		t.Errorf("marks: %d, %d; want 1 and 2", after.Marks(), again.Marks())
	}
}

func TestMarkNeedsAKeyAndAKeyForTheItems(t *testing.T) {
	t.Run("items without a key", func(t *testing.T) {
		m := keys(t, load(t, newSource(10, 10)), "space")
		if m.Marks() != 0 {
			t.Errorf("an item without a key was marked")
		}
		if m.Gutter() != gutterWidth {
			t.Errorf("gutter is %d wide, want %d", m.Gutter(), gutterWidth)
		}
	})
	t.Run("unbound", func(t *testing.T) {
		src := newSource(10, 10)
		m := New(src.fetch, renderItem, WithKey(byID), WithSize(40, 5), WithFocused(true),
			WithKeyMap(testKeyMap), WithMarkKeys(NewMarkKeys(keytest.Table(nil))), WithPromptKeys(testPromptKeys))
		m = run(t, m, m.Init())
		m = keys(t, m, "space")
		if m.Marks() != 0 {
			t.Error("space marked a row while mark is unbound")
		}
		if m.Gutter() != gutterWidth {
			t.Errorf("gutter is %d wide, want %d", m.Gutter(), gutterWidth)
		}
	})
}

func TestMarkGutter(t *testing.T) {
	m := marking(t, newSource(10, 10))
	if m.Gutter() != markedWidth {
		t.Fatalf("gutter is %d wide, want %d", m.Gutter(), markedWidth)
	}
	m = keys(t, m, "space", "j")
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if !strings.HasPrefix(lines[0], " ◆ #0 item 0") {
		t.Errorf("the marked row reads %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "▌  #1 item 1") {
		t.Errorf("the selected row reads %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "   #2 item 2") {
		t.Errorf("a plain row reads %q", lines[2])
	}
	// Every row keeps to the width, with the cell for the mark.
	assertFits(t, m.View(), m.Width(), m.Height())
}

func TestMarkGlyphFollowsStyles(t *testing.T) {
	m := marking(t, newSource(10, 10))
	s := m.Styles()
	s.MarkGlyph = "+"
	m.SetStyles(s)
	m = keys(t, m, "space")
	if !strings.Contains(ansi.Strip(m.View()), "▌+ #0 item 0") {
		t.Errorf("the mark is not the glyph of the styles:\n%s", ansi.Strip(m.View()))
	}
}

func TestMarksSurviveReload(t *testing.T) {
	src := newSource(25, 10)
	m := marking(t, src)
	m = keys(t, m, "space", "j", "j", "space")
	// Page down so that another chunk loads, then reload them all.
	m = keys(t, m, "end")
	m = keys(t, m, "end")
	if m.Marks() != 2 {
		t.Fatalf("%d marks after paging, want 2", m.Marks())
	}
	m = run(t, m, m.Reload())
	if m.Marks() != 2 {
		t.Errorf("%d marks after a reload, want 2", m.Marks())
	}
	// The marked items moved down one row.
	src.items = append([]item{{id: "new", title: "new"}}, src.items...)
	m = run(t, m, m.Reload())
	m = keys(t, m, "g")
	if m.Marks() != 2 {
		t.Errorf("%d marks after items moved, want 2", m.Marks())
	}
	for _, id := range []string{"0", "2"} {
		i, ok := m.indexOf(id)
		if it, loaded := m.LoadedItem(i); !ok || !loaded || !m.Marked(it) {
			t.Errorf("item %s is not marked after a reload", id)
		}
	}
}

func TestMarkDroppedWithItsItem(t *testing.T) {
	src := newSource(10, 10)
	m := marking(t, src)
	m = keys(t, m, "space", "j", "space")
	src.items = src.items[1:]
	m = run(t, m, m.Reload())
	if m.Marks() != 1 {
		t.Fatalf("%d marks after an item went, want 1", m.Marks())
	}
	if it, ok := m.LoadedItem(0); !ok || it.id != "1" || !m.Marked(it) {
		t.Errorf("the mark of the item that stayed is gone")
	}
}

// An item that the loaded chunks lack may be in one that isn't loaded, so
// its mark stays until the list says it is gone.
func TestMarkKeptWhileTheListIsNotLoaded(t *testing.T) {
	src := newSource(30, 10)
	m := marking(t, src)
	m = keys(t, m, "space")
	src.items = src.items[1:]
	m = run(t, m, m.Reload())
	if m.Marks() != 1 {
		t.Errorf("%d marks, want the one that cannot be told gone", m.Marks())
	}
}

func TestMarksSurviveFilter(t *testing.T) {
	m := marking(t, bugs(10))
	// Item 0 is plain, item 2 is a bug.
	m = keys(t, m, "space", "j", "j", "space")
	m = typed(t, m, "&", "bug", "enter")
	if m.Shown() != 2 {
		t.Fatalf("the filter shows %d rows, want 2", m.Shown())
	}
	if m.Marks() != 2 {
		t.Errorf("%d marks under the filter, want 2: the hidden one stays", m.Marks())
	}
	// The row of a marked bug shows its mark.
	if it, _ := m.Item(0); !m.Marked(it) {
		t.Errorf("the marked bug %s is not marked under the filter", it.id)
	}
	m = typed(t, m, "esc")
	if m.FilterQuery() != "" || m.Marks() != 2 {
		t.Errorf("filter %q, %d marks after esc; want no filter and 2 marks", m.FilterQuery(), m.Marks())
	}
}

func TestMarkCanBeSetUnderTheFilter(t *testing.T) {
	m := typed(t, marking(t, bugs(10)), "&", "bug", "enter")
	m = keys(t, m, "j", "space")
	it, _ := m.Selected()
	if it.id != "7" || m.Marks() != 1 || !m.Marked(it) {
		t.Errorf("marked %q (%d marks), want item 7", it.id, m.Marks())
	}
	m = typed(t, m, "esc", "esc")
	if m.Marks() != 0 {
		t.Errorf("%d marks left", m.Marks())
	}
}

func TestEscPeelsPromptFindFilterMarks(t *testing.T) {
	m := marking(t, bugs(10))
	m = keys(t, m, "space")
	m = typed(t, m, "&", "bug", "enter", "/", "7", "enter")
	m = keys(t, m, "space")
	state := func() (bool, bool, int) { return m.FindQuery() != "", m.FilterQuery() != "", m.Marks() }
	if f, q, n := state(); !f || !q || n != 2 {
		t.Fatalf("find %v, filter %v, %d marks; want all three", f, q, n)
	}
	// An open prompt takes the first esc and leaves everything shown.
	m = typed(t, m, "/", "x", "esc")
	if m.Capturing() {
		t.Error("the prompt is open after esc")
	}
	if f, q, n := state(); !f || !q || n != 2 {
		t.Errorf("after the prompt closed: find %v, filter %v, %d marks; want all three", f, q, n)
	}
	for i, want := range []struct {
		find, filter bool
		marks        int
	}{{false, true, 2}, {false, false, 2}, {false, false, 0}} {
		if !m.Takes(press("esc")) {
			t.Fatalf("esc %d is not taken by the feed", i+1)
		}
		m = typed(t, m, "esc")
		if f, q, n := state(); f != want.find || q != want.filter || n != want.marks {
			t.Errorf("after esc %d: find %v, filter %v, %d marks; want %v, %v, %d",
				i+1, f, q, n, want.find, want.filter, want.marks)
		}
	}
	if m.Takes(press("esc")) {
		t.Error("the feed takes esc with nothing left to clear")
	}
}

func TestResetForgetsMarks(t *testing.T) {
	m := keys(t, marking(t, newSource(10, 10)), "space")
	m = run(t, m, m.Reset())
	if m.Marks() != 0 {
		t.Errorf("%d marks after a reset, want none", m.Marks())
	}
}

func TestMarkChip(t *testing.T) {
	m := marking(t, newSource(10, 10))
	if strings.Contains(ansi.Strip(m.View()), "marked") {
		t.Error("the chip shows with nothing marked")
	}
	m = keys(t, m, "space", "j", "space", "j", "space")
	last := lastLine(ansi.Strip(m.View()))
	if !strings.Contains(last, "3 marked") {
		t.Errorf("the last line reads %q, want the chip %q", last, "3 marked")
	}
	m = keys(t, m, "space")
	if last := lastLine(ansi.Strip(m.View())); !strings.Contains(last, "2 marked") {
		t.Errorf("the last line reads %q, want the chip %q", last, "2 marked")
	}
	m = keys(t, m, "esc")
	if strings.Contains(ansi.Strip(m.View()), "marked") {
		t.Error("the chip shows after the marks were cleared")
	}
	assertFits(t, m.View(), m.Width(), m.Height())
}

func lastLine(v string) string {
	lines := strings.Split(v, "\n")
	return lines[len(lines)-1]
}

func TestMarkHelp(t *testing.T) {
	has := func(m Model[item], desc string) bool {
		for _, g := range m.FullHelp() {
			for _, b := range g {
				if b.Help().Desc == desc && b.Enabled() {
					return true
				}
			}
		}
		return false
	}
	m := marking(t, newSource(10, 10))
	if !has(m, "mark") {
		t.Error("help lacks the mark key")
	}
	if has(m, "clear marks") || has(m, "cancel") {
		t.Error("help lists esc with nothing to clear")
	}
	m = keys(t, m, "space")
	if !has(m, "clear marks") {
		t.Error("help lacks esc, clear marks, with a mark set")
	}
	// A find has esc to itself first.
	m = typed(t, m, "/", "1", "enter")
	if has(m, "clear marks") || !has(m, "cancel") {
		t.Error("help should list esc as cancel while a find shows")
	}
}

func TestMarkView(t *testing.T) {
	tests := []struct {
		name  string
		model func(t *testing.T) Model[item]
	}{
		{"marked rows", func(t *testing.T) Model[item] {
			t.Helper()
			return keys(t, marking(t, newSource(10, 10)), "space", "j", "j", "space", "j")
		}},
		{"marked rows and a filter", func(t *testing.T) Model[item] {
			t.Helper()
			m := keys(t, marking(t, bugs(10)), "space", "j", "j", "space")
			return typed(t, m, "&", "bug", "enter")
		}},
		{"marked rows blurred", func(t *testing.T) Model[item] {
			t.Helper()
			m := keys(t, marking(t, newSource(10, 10)), "space", "j")
			m.Blur()
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

func TestNoMarkKeysNoMarkHelp(t *testing.T) {
	src := newSource(10, 10)
	m := New(src.fetch, renderItem, WithKey(byID), WithSize(40, 5), WithFocused(true),
		WithKeyMap(testKeyMap), WithPromptKeys(testPromptKeys))
	m = run(t, m, m.Init())
	m = keys(t, m, "space")
	if m.Marks() != 0 || m.Gutter() != gutterWidth {
		t.Errorf("a feed without the key marks rows: %d marks, gutter %d", m.Marks(), m.Gutter())
	}
	for _, g := range m.FullHelp() {
		for _, b := range g {
			if b.Help().Desc == "mark" {
				t.Error("help lists the mark key of a feed that has none")
			}
		}
	}
}

// A parent that dismisses with its own key peels the same layers: the find,
// the filter, then the marks.
func TestClearTransientPeelsMarksLast(t *testing.T) {
	m := marking(t, bugs(10))
	m = keys(t, m, "space")
	m = typed(t, m, "&", "bug", "enter", "/", "7", "enter")
	for i, want := range []int{1, 1, 0} {
		cmd, ok := m.ClearTransient()
		m = run(t, m, cmd)
		if !ok || m.Marks() != want {
			t.Fatalf("call %d: ok %v, %d marks, want true and %d", i+1, ok, m.Marks(), want)
		}
	}
	if m.FindQuery() != "" || m.FilterQuery() != "" {
		t.Errorf("find %q or filter %q left", m.FindQuery(), m.FilterQuery())
	}
	if _, ok := m.ClearTransient(); ok {
		t.Error("ClearTransient reports a layer with nothing left")
	}
}

// A feed given the mark key but no key for its items can't mark rows, so
// help lists the mark row with its key off.
func TestMarkHelpOffWithoutItemKeys(t *testing.T) {
	src := newSource(10, 10)
	rowOf := func(m Model[item]) (key.Binding, bool) {
		for _, g := range m.FullHelp() {
			for _, b := range g {
				if b.Help().Desc == "mark" {
					return b, true
				}
			}
		}
		return key.Binding{}, false
	}
	m := New(src.fetch, renderItem, WithSize(40, 5), WithFocused(true),
		WithKeyMap(testKeyMap), WithMarkKeys(testMarkKeys), WithPromptKeys(testPromptKeys))
	if b, ok := rowOf(m); !ok || b.Enabled() {
		t.Errorf("a feed that can't mark: row listed %v, enabled %v; want listed and off", ok, ok && b.Enabled())
	}
	if b, ok := rowOf(marking(t, src)); !ok || !b.Enabled() {
		t.Errorf("a feed that marks: row listed %v, enabled %v; want listed and on", ok, ok && b.Enabled())
	}
}

func TestMarkedItems(t *testing.T) {
	src := newSource(25, 10)
	m := marking(t, src)
	if items, missing := m.MarkedItems(); len(items) != 0 || missing != 0 {
		t.Errorf("nothing marked, got %d items and %d missing", len(items), missing)
	}
	// Mark 2, then 0, then 12 (in the second chunk), out of order.
	m = keys(t, m, "j", "j", "space", "g", "space")
	m = keys(t, m, "end")
	m = keys(t, m, "end")
	m = keys(t, m, "g")
	for range 12 {
		m = keys(t, m, "j")
	}
	m = keys(t, m, "space")
	items, missing := m.MarkedItems()
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.id)
	}
	if got := strings.Join(ids, ","); got != "0,2,12" || missing != 0 {
		t.Errorf("marked items %s with %d missing, want 0,2,12 in list order and 0 missing", got, missing)
	}
	// A quick filter hides rows, not their marks.
	m = typed(t, m, "&", "item 1", "enter")
	if items, _ := m.MarkedItems(); len(items) != 3 {
		t.Errorf("under a filter %d marked items, want all 3", len(items))
	}
}

// A mark of an item that is not among the loaded ones is counted, not
// returned.
func TestMarkedItemsMissing(t *testing.T) {
	src := newSource(30, 10)
	m := marking(t, src)
	m = keys(t, m, "space")
	// The first item goes, but the list can't tell as its last chunk is
	// not loaded, so the mark stays and cannot be resolved.
	src.items = src.items[1:]
	m = run(t, m, m.Reload())
	items, missing := m.MarkedItems()
	if len(items) != 0 || missing != 1 {
		t.Errorf("got %d items and %d missing, want 0 and 1", len(items), missing)
	}
}

// An item that two loaded chunks hold, as after the list shifted, is
// returned once, and the count of the ones missing is never negative.
func TestMarkedItemsOnceWhenInTwoChunks(t *testing.T) {
	m := marking(t, newSource(25, 10))
	m = keys(t, m, "end")
	m = keys(t, m, "end")
	m = keys(t, m, "g", "space")
	if len(m.chunks) < 2 {
		t.Fatalf("%d chunks loaded, want at least 2", len(m.chunks))
	}
	// The marked item is the first of chunk 0; the second chunk holds it too.
	dup := m.chunks[0].items[0]
	m.chunks[1].items = append([]item{dup}, m.chunks[1].items...)
	items, missing := m.MarkedItems()
	if len(items) != 1 || missing != 0 {
		t.Errorf("got %d items and %d missing, want 1 and 0", len(items), missing)
	}
}

func TestSetMarks(t *testing.T) {
	m := keys(t, marking(t, newSource(10, 10)), "space", "j", "space", "j", "space")
	m.SetMarks("1", "7")
	items, _ := m.MarkedItems()
	if len(items) != 2 || items[0].id != "1" || items[1].id != "7" || m.Marks() != 2 {
		t.Errorf("marks left %v (%d), want items 1 and 7 alone", items, m.Marks())
	}
	m.SetMarks()
	if m.Marks() != 0 {
		t.Errorf("SetMarks() left %d marks, want none", m.Marks())
	}
}
