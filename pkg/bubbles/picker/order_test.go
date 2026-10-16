package picker

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

var fruit = []Item{
	{Title: "Zebra", Value: "z"},
	{Title: "apple", Value: "a"},
	{Title: "Banana", Value: "b"},
	{Title: "cherry", Value: "c"},
	{Title: "Date", Value: "d"},
	{Title: "elder", Value: "e"},
}

// firstPicker returns a focused checklist that lists the marked first, in
// normal mode, with marked set.
func firstPicker(tb testing.TB, items []Item, marked ...any) Model {
	tb.Helper()
	m := open(tb, nil, WithItems(items), WithMarks("[x]", "[ ]"), WithMarkedFirst(true), WithModes(true))
	m.SetMarked(marked)
	return m
}

// toggled sets the marks the way a parent that flips the selected item does.
func toggled(m *Model) {
	it, _ := m.Selected()
	var next []any
	found := false
	for _, v := range m.marks {
		if equal(v, it.Value) {
			found = true
			continue
		}
		next = append(next, v)
	}
	if !found {
		next = append(next, it.Value)
	}
	m.SetMarked(next)
}

// ruleAfter returns how many items come before the rule, or -1 if there is
// no rule. It fails when there are several, or when one is not between items.
func ruleAfter(t *testing.T, m Model) int {
	t.Helper()
	at := -1
	for i, r := range m.rows {
		if !r.rule {
			continue
		}
		if at >= 0 {
			t.Fatalf("rows have more than one rule")
		}
		if r.item != -1 || i == 0 || i == len(m.rows)-1 || m.rows[i-1].item < 0 || m.rows[i+1].item < 0 {
			t.Fatalf("rule at row %d is not between two items: %+v", i, m.rows)
		}
		at = m.rows[i-1].item + 1
	}
	return at
}

func selectedTitle(m Model) string {
	it, _ := m.Selected()
	return it.Title
}

func TestMarkedFirstOrder(t *testing.T) {
	tests := []struct {
		name   string
		items  []Item
		marked []any
		want   []string
		rule   int
	}{
		{"none marked", fruit, nil, []string{"apple", "Banana", "cherry", "Date", "elder", "Zebra"}, -1},
		{"some marked", fruit, []any{"z", "b"}, []string{"Banana", "Zebra", "apple", "cherry", "Date", "elder"}, 2},
		{"all marked", fruit, []any{"z", "a", "b", "c", "d", "e"}, []string{"apple", "Banana", "cherry", "Date", "elder", "Zebra"}, -1},
		{"marks outside the list", fruit, []any{"nope", "e"}, []string{"elder", "apple", "Banana", "cherry", "Date", "Zebra"}, 1},
		{
			"ties go by exact title then value",
			[]Item{{Title: "same", Value: "2"}, {Title: "Same", Value: "3"}, {Title: "same", Value: "1"}},
			nil,
			[]string{"Same", "same", "same"},
			-1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := firstPicker(t, tt.items, tt.marked...)
			if got := titlesOf(m); !slices.Equal(got, tt.want) {
				t.Errorf("titles = %q, want %q", got, tt.want)
			}
			if got := ruleAfter(t, m); got != tt.rule {
				t.Errorf("rule after %d items, want %d", got, tt.rule)
			}
			if m.Len() != len(tt.items) {
				t.Errorf("Len = %d, want %d: the rule is not an item", m.Len(), len(tt.items))
			}
			if tt.name == "ties go by exact title then value" {
				if v := m.results[1].Value; v != "1" {
					t.Errorf("second row's value = %v, want 1", v)
				}
			}
		})
	}
}

func TestMarkedFirstOffKeepsTheOrder(t *testing.T) {
	m := open(t, nil, WithItems(fruit), WithMarks("[x]", "[ ]"))
	m.SetMarked([]any{"z"})
	want := []string{"Zebra", "apple", "Banana", "cherry", "Date", "elder"}
	if got := titlesOf(m); !slices.Equal(got, want) {
		t.Errorf("titles = %q, want %q", got, want)
	}
	if got := ruleAfter(t, m); got != -1 {
		t.Errorf("rule after %d items, want none", got)
	}
	if got := m.Title("Labels"); got != "Labels" {
		t.Errorf("Title = %q, want the name alone", got)
	}
}

func TestMarkedFirstNeedsMarks(t *testing.T) {
	m := open(t, nil, WithItems(fruit), WithMarkedFirst(true))
	if got := titlesOf(m); !slices.Equal(got, titlesOf(open(t, nil, WithItems(fruit)))) {
		t.Errorf("titles = %q, want the order of the items", got)
	}
}

func TestMarkedFirstTitle(t *testing.T) {
	tests := []struct {
		marked []any
		want   string
	}{
		{nil, "Labels · 0 selected"},
		{[]any{"a"}, "Labels · 1 selected"},
		{[]any{"a", "b", "z"}, "Labels · 3 selected"},
	}
	for _, tt := range tests {
		m := firstPicker(t, fruit, tt.marked...)
		if got := m.Title("Labels"); got != tt.want {
			t.Errorf("Title = %q, want %q", got, tt.want)
		}
	}
}

func TestMarkedFirstToggle(t *testing.T) {
	tests := []struct {
		name string
		// items defaults to fruit.
		items  []Item
		marked []any
		// filter is typed first, and start is the value selected.
		filter string
		start  any
		want   []string
		// selected is the title the selection is on after the toggle.
		selected string
		rule     int
	}{
		{
			name: "middle, into the marked", marked: []any{"z"}, start: "b",
			want: []string{"Banana", "Zebra", "apple", "cherry", "Date", "elder"}, selected: "Banana", rule: 2,
		},
		{
			name: "middle, out of the marked", marked: []any{"z", "b", "d"}, start: "b",
			want: []string{"Date", "Zebra", "apple", "Banana", "cherry", "elder"}, selected: "Banana", rule: 2,
		},
		{
			name: "last row, into the marked", marked: []any{"z"}, start: "e",
			want: []string{"elder", "Zebra", "apple", "Banana", "cherry", "Date"}, selected: "elder", rule: 2,
		},
		{
			name: "last row, out of the marked", marked: []any{"z", "a", "b", "c", "d", "e"}, start: "z",
			want: []string{"apple", "Banana", "cherry", "Date", "elder", "Zebra"}, selected: "Zebra", rule: 5,
		},
		{
			name: "last of the marked, out of them", marked: []any{"z", "b"}, start: "z",
			want: []string{"Banana", "apple", "cherry", "Date", "elder", "Zebra"}, selected: "Zebra", rule: 1,
		},
		{
			name: "first row, out of the marked", marked: []any{"b", "z"}, start: "b",
			want: []string{"Zebra", "apple", "Banana", "cherry", "Date", "elder"}, selected: "Banana", rule: 1,
		},
		{
			name: "only item", items: []Item{{Title: "solo", Value: "s"}}, start: "s",
			want: []string{"solo"}, selected: "solo", rule: -1,
		},
		{
			name: "only item, out again", items: []Item{{Title: "solo", Value: "s"}}, marked: []any{"s"}, start: "s",
			want: []string{"solo"}, selected: "solo", rule: -1,
		},
		{
			name: "filtered, into the marked", marked: []any{"z"}, filter: "e", start: "a",
			want: []string{"apple", "Zebra", "cherry", "Date", "elder"}, selected: "apple", rule: 2,
		},
		{
			name: "filtered, last row", marked: []any{"z"}, filter: "e", start: "e",
			want: []string{"elder", "Zebra", "apple", "cherry", "Date"}, selected: "elder", rule: 2,
		},
		{
			name: "filtered, out of the marked", marked: []any{"z", "a"}, filter: "e", start: "z",
			want: []string{"apple", "cherry", "Date", "elder", "Zebra"}, selected: "Zebra", rule: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := tt.items
			if items == nil {
				items = fruit
			}
			m := firstPicker(t, items, tt.marked...)
			if tt.filter != "" {
				m = typeText(t, m, "/"+tt.filter)
				m, _ = press(t, m, enter)
			}
			if !m.Select(tt.start) {
				t.Fatalf("%v is not listed: %q", tt.start, titlesOf(m))
			}
			toggled(&m)
			// A filter that matches fewer items must not bring others back.
			if got := titlesOf(m); !slices.Equal(got, tt.want) {
				t.Fatalf("titles = %q, want %q", got, tt.want)
			}
			if got := selectedTitle(m); got != tt.selected {
				t.Errorf("selected = %q, want %q", got, tt.selected)
			}
			if got := ruleAfter(t, m); got != tt.rule {
				t.Errorf("rule after %d items, want %d", got, tt.rule)
			}
			assertOnItem(t, m)
		})
	}
}

// Marks that change under other items, or not at all, leave the selection on
// its item.
func TestMarkedFirstSelectionStays(t *testing.T) {
	m := firstPicker(t, fruit, "z")
	m.Select("c")
	m.SetMarked([]any{"z", "a"})
	if got := selectedTitle(m); got != "cherry" {
		t.Errorf("selected = %q after other items changed, want cherry", got)
	}
	m.SetMarked([]any{"z", "a"})
	if got := selectedTitle(m); got != "cherry" {
		t.Errorf("selected = %q after the same marks, want cherry", got)
	}
}

// The selection stays on its item when the item moves out of the window, up
// into the marked group or down out of it, and the list scrolls to show it.
func TestMarkedFirstScrolled(t *testing.T) {
	items := make([]Item, 30)
	for i := range items {
		items[i] = Item{Title: "item " + strconv.Itoa(100+i), Value: i}
	}
	tests := []struct {
		name   string
		marked []any
		start  int
		want   string
	}{
		{name: "up into the marked", marked: []any{29}, start: 20, want: "item 120"},
		{name: "down out of the marked", marked: []any{29}, start: 29, want: "item 129"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, nil, WithItems(items), WithMarks("[x]", "[ ]"), WithMarkedFirst(true), WithModes(true), WithSize(40, 8))
			m.SetMarked(tt.marked)
			m.Select(tt.start)
			before := m.top
			toggled(&m)
			if got := selectedTitle(m); got != tt.want {
				t.Errorf("selected = %q, want %q", got, tt.want)
			}
			if m.top == before {
				t.Errorf("top = %d, the list did not scroll to the item", m.top)
			}
			assertOnItem(t, m)
			v := m.View()
			if !strings.Contains(v, tt.want) {
				t.Errorf("the item is not in view:\n%s", v)
			}
			assertFits(t, v, 40, 8)
		})
	}
}

// The selection never rests on the rule, however it moves.
func TestMarkedFirstCursorSkipsTheRule(t *testing.T) {
	// The rule follows the marked rows that the filter leaves.
	for filter, rule := range map[string]int{"": 2, "e": 1} {
		for _, size := range []int{4, 6, 8, 12} {
			m := open(t, nil, WithItems(fruit), WithMarks("[x]", "[ ]"), WithMarkedFirst(true), WithModes(true), WithSize(40, size))
			m.SetMarked([]any{"z", "b"})
			if filter != "" {
				m = typeText(t, m, "/"+filter)
				m, _ = press(t, m, enter)
			}
			keys := []any{down, down, down, up, up, pgDown, pgUp, pgDown, up, down, down, down, down, down, down, up}
			for i, k := range keys {
				m, _ = press(t, m, k)
				assertOnItem(t, m)
				if got := ruleAfter(t, m); got != rule {
					t.Fatalf("filter %q size %d key %d: rule after %d items, want %d", filter, size, i, got, rule)
				}
			}
			for _, k := range []string{"g", "G", "g"} {
				m = typeText(t, m, k)
				assertOnItem(t, m)
			}
			if got := m.Len(); got != len(m.listed) {
				t.Errorf("Len = %d, want %d", got, len(m.listed))
			}
		}
	}
}

// assertOnItem checks the selection is an item row, which the rule never is.
func assertOnItem(t *testing.T, m Model) {
	t.Helper()
	if m.sel < 0 || m.sel >= len(m.results) {
		t.Fatalf("selection %d is outside the %d results", m.sel, len(m.results))
	}
	if r := m.rows[m.itemRow[m.sel]]; r.rule || r.item != m.sel {
		t.Fatalf("selection %d is on row %+v", m.sel, r)
	}
}

func TestMarkedFirstFits(t *testing.T) {
	for _, w := range []int{1, 2, 3, 4, 5, 10, 40, 80} {
		for _, h := range []int{1, 2, 3, 4, 5, 6, 12} {
			t.Run(strconv.Itoa(w)+"x"+strconv.Itoa(h), func(t *testing.T) {
				m := open(t, nil, WithItems(fruit), WithMarks("[x]", "[ ]"), WithMarkedFirst(true), WithModes(true), WithSize(w, h))
				m.SetMarked([]any{"b", "z"})
				assertFits(t, m.View(), w, h)
				m.Select("c")
				toggled(&m)
				assertFits(t, m.View(), w, h)
				m.SetSize(h*3, w%7+1)
				assertFits(t, m.View(), h*3, w%7+1)
			})
		}
	}
}

func TestViewMarkedFirst(t *testing.T) {
	tests := []struct {
		name   string
		marked []any
		filter string
		// wide sets the size to 80x24, and is 40x12 otherwise.
		wide bool
	}{
		{name: "both groups", marked: []any{"z", "b"}},
		{name: "both groups, 80 columns", marked: []any{"z", "b"}, wide: true},
		{name: "none marked"},
		{name: "all marked", marked: []any{"z", "a", "b", "c", "d", "e"}},
		{name: "filtered both groups", marked: []any{"z"}, filter: "e"},
		{name: "filtered one group", marked: []any{"b"}, filter: "e"},
		{name: "ascii", marked: []any{"z", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := 40, 12
			if tt.wide {
				w, h = 80, 24
			}
			opts := []Option{WithItems(fruit), WithMarks("[x]", "[ ]"), WithMarkedFirst(true), WithModes(true), WithSize(w, h)}
			if tt.name == "ascii" {
				st := asciiStyles()
				st.RuleGlyph = "-"
				opts = append(opts, WithStyles(st))
			}
			m := open(t, nil, opts...)
			m.SetMarked(tt.marked)
			if tt.filter != "" {
				m = typeText(t, m, "/"+tt.filter)
			}
			v := m.View()
			assertFits(t, v, w, h)
			golden.RequireEqual(t, v)
		})
	}
}
