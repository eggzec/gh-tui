package filterform

import (
	"strconv"
	"strings"
	"testing"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		query         string
		fail          bool
		// block leaves the labels loading, with the editor closed.
		block   bool
		blurred bool
		opts    []Option
		keys    []tea.Msg
		typed   string
		// spec, when set, is the form's spec in place of prSpec.
		spec func(Loader) Spec
	}{
		{name: "defaults 60", width: 60, height: 12},
		{name: "defaults 100", width: 100, height: 12},
		{name: "blurred", width: 60, height: 12, blurred: true},
		{name: "chips wrap", width: 60, height: 13, query: `is:open label:bug,enhancement,docs,"good first issue",wontfix,needs-triage`, keys: []tea.Msg{down, down, down, left}},
		{name: "labels open 60", width: 60, height: 18, keys: []tea.Msg{down, down, down, enter, down, down, space}},
		{name: "labels open 100", width: 100, height: 18, keys: []tea.Msg{down, down, down, enter}, typed: "doc"},
		{name: "labels failed", width: 60, height: 14, fail: true, keys: []tea.Msg{down, down, down, enter}},
		{name: "editing text", width: 60, height: 12, keys: []tea.Msg{down, down, down, down, down, enter}, typed: "-next"},
		{name: "query focused", width: 60, height: 12, keys: []tea.Msg{up}, typed: " fix"},
		{name: "free text", width: 100, height: 12, query: "is:merged fix crash repo:cli/cli"},
		{name: "empty query", width: 60, height: 12, query: "sort:updated-desc", keys: []tea.Msg{down, down, down, keyX, keyX}},
		{name: "narrow 40", width: 40, height: 14},
		{name: "scrolled", width: 60, height: 6, keys: []tea.Msg{down, down, down, down, down, down}},
		{name: "no help", width: 60, height: 10, opts: []Option{WithHelpLine(false)}},
		{name: "loading", width: 60, height: 12, block: true, keys: []tea.Msg{down, down, down}},
		{name: "light", width: 60, height: 12, opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "filters 80", width: 80, height: 12},
		{name: "filters 120", width: 120, height: 12},
		{name: "filters 80 light", width: 80, height: 12, opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "filters 120 light", width: 120, height: 12, opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "sort 80", width: 80, height: 12, opts: []Option{WithTab(SortTab)}},
		{name: "sort 120", width: 120, height: 12, opts: []Option{WithTab(SortTab)}},
		{name: "sort 80 light", width: 80, height: 12, opts: []Option{WithTab(SortTab), WithStyles(DefaultStyles(false))}},
		{name: "sort 120 light", width: 120, height: 12, opts: []Option{WithTab(SortTab), WithStyles(DefaultStyles(false))}},
		{name: "sort order", width: 80, height: 12, query: "sort:comments-asc", opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{down}},
		{name: "sort best match", width: 80, height: 12, spec: searchSpec, opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{down}},
		{name: "sort after next tab", width: 80, height: 12, keys: []tea.Msg{nextTab}},
		{name: "single tab 80", width: 80, height: 10, spec: noSortSpec, opts: []Option{WithTab(SortTab)}},
		{name: "no tab bar", width: 80, height: 10, opts: []Option{WithTabBar(false), WithTab(SortTab)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLoader{}
			if tt.fail {
				f.fail = errBoom
			}
			opts := append([]Option{WithSize(tt.width, tt.height)}, tt.opts...)
			if tt.query != "" {
				opts = append(opts, WithQuery(tt.query))
			}
			spec := prSpec
			if tt.spec != nil {
				spec = tt.spec
			}
			m := New(spec(f.load), opts...)
			m.Focus()
			m, _ = press(t, m, tt.keys...)
			if tt.block {
				m, _ = m.Update(enter)
				m, _ = press(t, m, esc)
			}
			m = typeText(t, m, tt.typed)
			if tt.blurred {
				m.Blur()
			}
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

// Every size renders exactly its width and height.
func TestViewFits(t *testing.T) {
	f := &fakeLoader{}
	for _, w := range []int{1, 2, 3, 5, 10, 20, 40, 60, 100, 200} {
		for _, h := range []int{1, 2, 3, 4, 6, 12, 40} {
			t.Run(strconv.Itoa(w)+"x"+strconv.Itoa(h), func(t *testing.T) {
				m := New(prSpec(f.load), WithSize(w, h))
				m.Focus()
				assertFits(t, m.View(), w, h)
				m, _ = press(t, m, down, down, down, enter)
				assertFits(t, m.View(), w, h)
				m.SetSize(h*3, w%7+1)
				assertFits(t, m.View(), h*3, w%7+1)
			})
		}
	}
}

func TestViewFollowsFocus(t *testing.T) {
	m := New(prSpec(nil), WithSize(60, 12))
	blurred := m.View()
	m.Focus()
	if m.View() == blurred {
		t.Error("focus didn't change the view")
	}
	m.Blur()
	if m.View() != blurred {
		t.Error("blur didn't restore the view")
	}
}

func TestViewShowsQuery(t *testing.T) {
	m := New(prSpec(nil), WithSize(100, 12))
	if v := ansi.Strip(m.View()); !strings.Contains(v, prDefaults) {
		t.Errorf("the view doesn't show the query:\n%s", v)
	}
}

func assertFits(t *testing.T, v string, width, height int) {
	t.Helper()
	if width == 0 || height == 0 {
		if v != "" {
			t.Fatalf("view = %q, want empty", v)
		}
		return
	}
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		t.Fatalf("view has %d lines, want %d:\n%s", len(lines), height, v)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			t.Errorf("line %d is %d wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

// searchSpec is a spec whose sort can be left out, as GitHub's search
// ranks by best match without one.
func searchSpec(Loader) Spec {
	return Spec{
		Fields: []Field{{Key: "lang", Label: "Language", Kind: Choice, Qualifier: "language", Options: []Item{{Label: "Any"}, {Label: "Go", Value: "go"}}}},
		Sort: &SortField{
			Options: []SortOption{
				{Label: "Best match"},
				{Label: "Stars", Value: "stars", Desc: "Most first", Asc: "Fewest first"},
				{Label: "Updated", Value: "updated", Desc: "Newest first", Asc: "Oldest first"},
			},
			Default: Sort{Desc: true},
		},
	}
}

// noSortSpec is prSpec without a sort, so the form has no tabs.
func noSortSpec(load Loader) Spec {
	s := prSpec(load)
	s.Sort = nil
	return s
}

// A form drawn with ASCII glyphs is ASCII alone: its fields, tabs, rule,
// sort orders and the picker of a list. The help line, which names keys
// as the key map labels them, is left out.
func TestViewASCII(t *testing.T) {
	st := DefaultStyles(true)
	st.Glyphs = Glyphs{
		Cursor: ">", On: "*", Off: "o", Remove: "x", Drop: "-", Rule: "-",
		Chosen: "+", NotChosen: "-", Down: "v", Up: "^", Separator: " - ", Ellipsis: "...",
	}
	st.Picker.Frame = st.Picker.Frame.Border(lipgloss.ASCIIBorder(), false, false, false, true)
	st.Picker.PromptGlyph, st.Picker.CursorGlyph, st.Picker.Ellipsis = ">", ">", "..."
	for _, tt := range []struct {
		name string
		opts []Option
		keys []tea.Msg
	}{
		{name: "filters"},
		{name: "chips", keys: []tea.Msg{down, down, down, left}},
		{name: "labels open", keys: []tea.Msg{down, down, down, enter, down, space}},
		{name: "sort", opts: []Option{WithTab(SortTab)}},
	} {
		f := &fakeLoader{}
		opts := append([]Option{WithStyles(st), WithHelpLine(false), WithSize(40, 18)}, tt.opts...)
		m := open(t, prSpec(f.load), opts...)
		m, _ = press(t, m, tt.keys...)
		if v := ansi.Strip(m.View()); strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("%s: view isn't ASCII:\n%s", tt.name, v)
		}
	}
}
