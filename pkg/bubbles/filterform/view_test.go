package filterform

import (
	"strconv"
	"strings"
	"testing"
	"unicode"

	"charm.land/bubbles/v2/spinner"
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
		// block leaves the labels loading, with the list closed, and
		// blockOpen with it open.
		block     bool
		blockOpen bool
		blurred   bool
		opts      []Option
		keys      []tea.Msg
		typed     string
		// spec, when set, is the form's spec in place of prSpec.
		spec func(Loader) Spec
	}{
		{name: "defaults 60", width: 60, height: 12},
		{name: "defaults 100", width: 100, height: 12},
		{name: "blurred", width: 60, height: 12, blurred: true},
		{name: "rows 80", width: 80, height: 12, keys: []tea.Msg{down, down, down}},
		{name: "rows 120", width: 120, height: 12, keys: []tea.Msg{down, down}},
		{name: "multi overflow 60", width: 60, height: 12, query: `is:open label:bug,enhancement,docs,"good first issue",wontfix,needs-triage`, keys: []tea.Msg{down, down, down}},
		{name: "checklist labels 60", width: 60, height: 18, keys: []tea.Msg{down, down, down, space, down, down, space}},
		{name: "checklist filtered 100", width: 100, height: 18, keys: []tea.Msg{down, down, down, space, keyI}, typed: "doc"},
		{name: "checklist labels 80", width: 80, height: 14, keys: []tea.Msg{down, down, down, space}},
		{name: "checklist labels 120", width: 120, height: 16, keys: []tea.Msg{down, down, down, space, down, space}},
		{name: "checklist ascii 80", width: 80, height: 14, opts: []Option{WithStyles(asciiStyles()), WithKeyNames(asciiKeys)}, keys: []tea.Msg{down, down, down, space}},
		{name: "list review 80", width: 80, height: 14, keys: []tea.Msg{down, down, space}},
		{name: "list review 120", width: 120, height: 16, keys: []tea.Msg{down, down, space, down}},
		{name: "list sort by 80", width: 80, height: 12, opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{space}},
		{name: "list order 80", width: 80, height: 12, opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{down, space}},
		{name: "list filter many 80", width: 80, height: 16, spec: languageSpec, keys: []tea.Msg{space}},
		{name: "list filter typed 80", width: 80, height: 16, spec: languageSpec, keys: []tea.Msg{space, keyI}, typed: "py"},
		{name: "list light 80", width: 80, height: 14, opts: []Option{WithStyles(DefaultStyles(false))}, keys: []tea.Msg{down, down, space}},
		{name: "list above 80", width: 80, height: 14, spec: lastChoiceSpec, keys: []tea.Msg{keyBigG, keyK, space, down}},
		{name: "list failed 80", width: 80, height: 14, fail: true, keys: []tea.Msg{down, down, down, space}},
		{name: "list loading 80", width: 80, height: 14, blockOpen: true, keys: []tea.Msg{down, down, down}},
		{name: "people typing 120", width: 120, height: 16, keys: []tea.Msg{down, space, keyI}, typed: "octo"},
		{name: "people 80", width: 80, height: 14, keys: []tea.Msg{down, space}},
		{name: "editing text", width: 60, height: 12, keys: []tea.Msg{down, down, down, down, down, keyA}, typed: "-next"},
		{name: "insert text 80", width: 80, height: 12, keys: []tea.Msg{down, down, down, down, down, keyA}, typed: "-next"},
		{name: "insert query 80", width: 80, height: 12, keys: []tea.Msg{keyBigG, keyA}, typed: " fix"},
		{name: "query focused", width: 60, height: 12, keys: []tea.Msg{keyBigG}},
		{name: "ascii 80", width: 80, height: 14, opts: []Option{WithStyles(asciiStyles()), WithKeyNames(strings.NewReplacer("↵", "enter").Replace)}, keys: []tea.Msg{down, down}},
		{name: "free text", width: 100, height: 12, query: "is:merged fix crash repo:cli/cli"},
		{name: "empty query", width: 60, height: 12, query: "sort:updated-desc", keys: []tea.Msg{down, down, down, del, del}},
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
			if tt.block || tt.blockOpen {
				f.block = make(chan struct{})
				t.Cleanup(func() { close(f.block) })
			}
			opts := append([]Option{WithSize(tt.width, tt.height)}, tt.opts...)
			if tt.query != "" {
				opts = append(opts, WithQuery(tt.query))
			}
			spec := prSpec
			if tt.spec != nil {
				spec = tt.spec
			}
			m := newKeyed(t, spec(f.load), opts...)
			m.Focus()
			m, _ = press(t, m, tt.keys...)
			if tt.block {
				m, _ = m.Update(space)
				m, _ = press(t, m, esc)
			}
			if tt.blockOpen {
				m, _ = m.Update(space)
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
				m := newKeyed(t, prSpec(f.load), WithSize(w, h))
				m.Focus()
				assertFits(t, m.View(), w, h)
				m, _ = press(t, m, down, down, down, space)
				assertFits(t, m.View(), w, h)
				m, _ = press(t, m, esc, keyBigG, keyA)
				assertFits(t, m.View(), w, h)
				m.SetSize(h*3, w%7+1)
				assertFits(t, m.View(), h*3, w%7+1)
			})
		}
	}
}

func TestViewFollowsFocus(t *testing.T) {
	m := newKeyed(t, prSpec(nil), WithSize(60, 12))
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
	m := newKeyed(t, prSpec(nil), WithSize(100, 12))
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

// asciiKeys writes the keys of the help line as an ASCII terminal does.
func asciiKeys(s string) string {
	return strings.NewReplacer("↵", "enter", "↑", "up", "↓", "down").Replace(s)
}

// languageSpec has a choice with more options than a dropdown shows.
func languageSpec(Loader) Spec {
	names := []string{
		"Any", "C", "C++", "C#", "Clojure", "CSS", "Dart", "Elixir", "Erlang", "Go", "Haskell", "HTML",
		"Java", "JavaScript", "Kotlin", "Lua", "Nix", "OCaml", "Perl", "PHP", "Python", "R", "Ruby",
		"Rust", "Scala", "Shell", "Swift", "TypeScript", "Zig",
	}
	opts := make([]Item, len(names))
	for i, n := range names {
		opts[i] = Item{Label: n, Value: strings.ToLower(n)}
	}
	opts[0].Value = ""
	return Spec{Fields: []Field{
		{Key: "language", Label: "Language", Kind: Choice, Qualifier: "language", Options: opts},
		{Key: "owner", Label: "Owner", Kind: Text, Qualifier: "user", Hint: "anyone"},
	}}
}

// lastChoiceSpec has a choice on its last row, which has no room under it
// for a dropdown.
func lastChoiceSpec(Loader) Spec {
	fields := make([]Field, 0, 8)
	for _, n := range []string{"Title", "Body", "Author", "Owner", "Org", "Repo", "Path"} {
		fields = append(fields, Field{Key: strings.ToLower(n), Label: n, Kind: Text, Qualifier: strings.ToLower(n)})
	}
	fields = append(fields, Field{
		Key: "state", Label: "State", Kind: Choice, Qualifier: "is",
		Options: []Item{{"Open", "open", ""}, {"Closed", "closed", ""}, {"Merged", "merged", ""}, {"Draft", "draft", ""}, {"All", "", ""}},
	})
	return Spec{Fields: fields}
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

// asciiStyles are styles whose glyphs are all ASCII.
func asciiStyles() Styles {
	st := DefaultStyles(true)
	st.Glyphs = Glyphs{
		Cursor: ">", Edge: "|", Prev: "<", Next: ">", Drop: "v", Rule: "-",
		On: "(*)", Off: "( )", Down: "v", Up: "^", Separator: " - ", Ellipsis: "...",
	}
	st.ErrorGlyph, st.ErrorSeparator, st.ErrorEllipsis = "x", " - ", "..."
	st.SpinnerFrames = spinner.Spinner{Frames: []string{"|", "/", "-", `\`}, FPS: spinner.Dot.FPS}
	st.Picker.SpinnerFrames = st.SpinnerFrames
	st.DropFrame = st.DropFrame.Border(lipgloss.ASCIIBorder())
	st.Picker.PromptGlyph, st.Picker.CursorGlyph, st.Picker.Ellipsis = ">", ">", "..."
	return st
}

// A form drawn with ASCII glyphs is ASCII alone: its fields, tabs, rule,
// sort orders, the picker of a list and the mark of insert mode. The help
// line, which names keys as the key map labels them, is left out.
func TestViewASCII(t *testing.T) {
	st := asciiStyles()
	for _, tt := range []struct {
		name string
		opts []Option
		keys []tea.Msg
		fail bool
	}{
		{name: "filters"},
		{name: "focused choice", keys: []tea.Msg{down}},
		{name: "multi", keys: []tea.Msg{down, down, down}},
		{name: "checklist", keys: []tea.Msg{down, down, down, space, down, space}},
		{name: "checklist filtered", keys: []tea.Msg{down, down, down, space, keyI, keyX}},
		{name: "list", keys: []tea.Msg{down, down, space}},
		{name: "people typing", keys: []tea.Msg{down, space, keyI, keyX}},
		{name: "failed list", keys: []tea.Msg{down, down, down, space}, fail: true},
		{name: "sort list", opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{space}},
		{name: "insert", keys: append(keys(down, rowBase), keyA)},
		{name: "insert query", keys: []tea.Msg{keyBigG, keyA}},
		{name: "sort", opts: []Option{WithTab(SortTab)}},
		{name: "sort order", opts: []Option{WithTab(SortTab)}, keys: []tea.Msg{down}},
	} {
		f := &fakeLoader{}
		if tt.fail {
			f.fail = errBoom
		}
		opts := append([]Option{WithStyles(st), WithKeyNames(asciiKeys), WithHelpLine(false), WithSize(40, 18)}, tt.opts...)
		m := open(t, prSpec(f.load), opts...)
		m, _ = press(t, m, tt.keys...)
		if v := ansi.Strip(m.View()); strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("%s: view isn't ASCII:\n%s", tt.name, v)
		}
	}
	// A list that waits for its options shows the spinner of the styles.
	f := &fakeLoader{block: make(chan struct{})}
	defer close(f.block)
	m := open(t, prSpec(f.load), WithStyles(st), WithKeyNames(asciiKeys), WithHelpLine(false), WithSize(40, 18))
	m, _ = press(t, m, down, down, down)
	m, _ = m.Update(space)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Loading labels") || strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
		t.Errorf("loading list isn't ASCII or doesn't say it loads:\n%s", v)
	}
}

// The mark of the keys that change a choice, and the count of the labels
// that don't fit, show only where they apply.
func TestViewCollapsedRows(t *testing.T) {
	const q = `is:open label:bug,enhancement,docs,"good first issue",wontfix,needs-triage`
	m := open(t, prSpec(nil), WithQuery(q), WithSize(60, 12), WithHelpLine(false))
	lines := func(m Model) []string {
		out := make([]string, 0, 16)
		for l := range strings.SplitSeq(ansi.Strip(m.View()), "\n") {
			out = append(out, strings.TrimRight(l, " "))
		}
		return out
	}
	view := strings.Join(lines(m), "\n")
	if strings.Count(view, "‹") != 1 || !strings.Contains(view, "‹ Open ›") {
		t.Errorf("want ‹ Open › on the focused row alone:\n%s", view)
	}
	m, _ = press(t, m, down, down, down)
	view = strings.Join(lines(m), "\n")
	if strings.Contains(view, "‹") || strings.Contains(view, "Open ›") {
		t.Errorf("want no ‹ › off the choice rows:\n%s", view)
	}
	var labelsRow string
	for _, l := range lines(m) {
		if strings.Contains(l, "Labels") {
			labelsRow = l
		}
	}
	if !strings.Contains(labelsRow, "+") || !strings.HasSuffix(labelsRow, "▾") || !strings.Contains(labelsRow, "bug, enhancement") {
		t.Errorf("labels row = %q, want the names that fit, +N and ▾", labelsRow)
	}
	for i, l := range lines(m) {
		if strings.Contains(l, "Review") && strings.Contains(l, "‹") {
			t.Errorf("line %d: %q shows ‹ off its row", i, l)
		}
	}
	// A single very long label is cut to fit, and keeps its mark.
	long := open(t, prSpec(nil), WithQuery("label:"+strings.Repeat("x", 80)), WithSize(40, 12), WithHelpLine(false))
	for _, l := range lines(long) {
		if strings.Contains(l, "Labels") && (!strings.Contains(l, "…") || !strings.HasSuffix(l, "▾")) {
			t.Errorf("labels row = %q, want the label cut with … and ▾", l)
		}
	}
}

// A help line that is too wide drops the hints that matter least, the other
// tab and clear first, and keeps the keys that apply and close.
func TestHelpLineDropsTheLeastImportantFirst(t *testing.T) {
	for _, tt := range []struct {
		width   int
		want    []string
		missing []string
	}{
		{width: 100, want: []string{"j/k field", "i insert", "a append", "delete clear", "↵ apply", "esc close", "] sort"}},
		{width: 64, want: []string{"j/k field", "↵ apply", "esc close"}, missing: []string{"] sort", "delete clear"}},
		{width: 30, want: []string{"↵ apply", "esc close"}, missing: []string{"j/k field", "] sort"}},
	} {
		m := open(t, prSpec(nil), WithSize(tt.width, 14))
		m, _ = press(t, m, keys(down, rowBase)...)
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		help := lines[len(lines)-1]
		for _, w := range tt.want {
			if !strings.Contains(help, w) {
				t.Errorf("%d wide: the help line %q lacks %q", tt.width, help, w)
			}
		}
		for _, w := range tt.missing {
			if strings.Contains(help, w) {
				t.Errorf("%d wide: the help line %q keeps %q", tt.width, help, w)
			}
		}
	}
}

// The keys of the help line are written as the parent says.
func TestHelpLineKeyNames(t *testing.T) {
	m := open(t, prSpec(nil), WithSize(80, 14), WithKeyNames(strings.NewReplacer("↵", "enter").Replace))
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if help := lines[len(lines)-1]; !strings.Contains(help, "enter apply") || strings.Contains(help, "↵") {
		t.Errorf("the help line is %q, want enter in words", help)
	}
}
