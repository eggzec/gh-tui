package filterform

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// view returns the form's view without styles.
func view(m Model) string { return ansi.Strip(m.View()) }

func TestSpaceOpensListOnChoice(t *testing.T) {
	m := open(t, prSpec(nil), WithSize(80, 14))
	m, _ = press(t, m, down, down, space)
	if m.Capturing() {
		t.Error("a list in its normal mode captures keys")
	}
	v := view(m)
	for _, want := range []string{"╭─ Review", "○ Any", "● Requested from me", "○ Approved", "╰"} {
		if !strings.Contains(v, want) {
			t.Errorf("the view lacks %q:\n%s", want, v)
		}
	}
	if it, _ := m.pick.Selected(); it.Value != "review-requested:@me" {
		t.Errorf("highlighted %v, want the current value", it.Value)
	}
}

func TestListChooseAndClose(t *testing.T) {
	m := open(t, prSpec(nil))
	m, sent := press(t, m, down, down, space, keyJ, enter)
	if v, _ := m.Value("review"); v.Text() != "review:approved" || m.mode != rowsMode || len(sent) > 0 {
		t.Errorf("review = %q, mode %v, sent %v; want approved and the list closed", v.Text(), m.mode, sent)
	}
	m, _ = press(t, m, space, keyJ, esc)
	if v, _ := m.Value("review"); v.Text() != "review:approved" || m.mode != rowsMode {
		t.Errorf("review = %q, mode %v; want esc to leave it", v.Text(), m.mode)
	}
	// The vim keys move in the list, and g and G go to the ends.
	m, _ = press(t, m, space, keyBigG, enter)
	if v, _ := m.Value("review"); v.Text() != "review:changes_requested" {
		t.Errorf("review = %q after G, want the last option", v.Text())
	}
	m, _ = press(t, m, space, keyG, keyJ, keyJ, keyK, enter)
	if v, _ := m.Value("review"); v.Text() != "review-requested:@me" {
		t.Errorf("review = %q after g j j k, want the second option", v.Text())
	}
}

func TestListOfSortAndOrder(t *testing.T) {
	m := open(t, prSpec(nil), WithTab(SortTab))
	m, _ = press(t, m, space, keyJ, keyJ, enter)
	if got := m.Sort(); got != (Sort{By: "comments", Desc: true}) {
		t.Errorf("sort = %+v, want comments, most first", got)
	}
	m, _ = press(t, m, down, space)
	if it, _ := m.pick.Selected(); it.Value != orderDesc {
		t.Errorf("highlighted %v, want the order in force", it.Value)
	}
	m, _ = press(t, m, keyJ, enter)
	if got := m.Sort(); got != (Sort{By: "comments"}) {
		t.Errorf("sort = %+v, want comments, fewest first", got)
	}
	if got := m.Query(); !strings.HasSuffix(got, "sort:comments-asc") {
		t.Errorf("Query = %q, want it to follow", got)
	}
	// A sort that writes none has no order to list.
	s := open(t, searchSpec(nil), WithTab(SortTab), WithQuery(""))
	s, _ = press(t, s, space, keyG, enter, down, space)
	if s.mode == listMode {
		t.Error("the order of a sort that writes none opened a list")
	}
}

// A checklist's value follows its checks at once.
func TestChecklistToggleLive(t *testing.T) {
	f := &fakeLoader{}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down, space, keyJ, keyJ, space)
	if got := m.Query(); !strings.Contains(got, "label:bug,enhancement,docs") {
		t.Errorf("Query = %q, want docs checked at once", got)
	}
	// Esc undoes what was checked since the list opened, enter keeps it.
	m, _ = press(t, m, esc)
	if got := m.Query(); strings.Contains(got, "docs") || m.mode != rowsMode {
		t.Errorf("Query = %q, mode %v; want esc to undo it", got, m.mode)
	}
	m, _ = press(t, m, space, keyJ, keyJ, space, enter)
	if got := m.Query(); !strings.Contains(got, "label:bug,enhancement,docs") || m.mode != rowsMode {
		t.Errorf("Query = %q, mode %v; want enter to keep it", got, m.mode)
	}
}

// Enter closes the checklist and never leaves the highlighted item out.
func TestChecklistEnterDone(t *testing.T) {
	f := &fakeLoader{}
	m := open(t, prSpec(f.load))
	// Space on bug unchecks it, then down to docs and enter adds docs.
	m, _ = press(t, m, down, down, down, space, space, keyJ, keyJ, enter)
	if v, _ := m.Value("labels"); !slices.Equal(v.List(), []string{"enhancement", "docs"}) || m.mode != rowsMode {
		t.Errorf("labels = %q, mode %v; want docs added and the list closed", v.List(), m.mode)
	}
	// Enter on what space just chose keeps the choice.
	m, _ = press(t, m, space, keyJ, keyJ, space, enter)
	if v, _ := m.Value("labels"); !slices.Equal(v.List(), []string{"enhancement"}) {
		t.Errorf("labels = %q, want docs unchecked and kept so", v.List())
	}
}

func TestChecklistClearUnchecksAll(t *testing.T) {
	f := &fakeLoader{}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down, space, del)
	if v, _ := m.Value("labels"); len(v.List()) != 0 {
		t.Errorf("labels = %q, want none", v.List())
	}
	if m.mode != listMode || !m.picking {
		t.Error("clearing a checklist closed it")
	}
	if strings.Contains(view(m), "[x]") {
		t.Errorf("the checklist still shows checks:\n%s", view(m))
	}
	m, _ = press(t, m, keyJ, space, bksp)
	if v, _ := m.Value("labels"); len(v.List()) != 0 {
		t.Errorf("labels = %q after backspace, want none", v.List())
	}
}

func TestListClearChoosesEmpty(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, space, del)
	if v, _ := m.Value("state"); v.Text() != "" || m.mode != rowsMode {
		t.Errorf("state = %q, mode %v; want the empty option and the list closed", v.Text(), m.mode)
	}
	// A choice with no empty option has nothing to clear: the list stays.
	s := Spec{Fields: []Field{{
		Key: "lang", Label: "Lang", Kind: Choice, Qualifier: "language",
		Options: []Item{{"Go", "go", ""}, {"Rust", "rust", ""}}, Default: TextValue("go"),
	}}}
	n := open(t, s)
	n, _ = press(t, n, space, del)
	if v, _ := n.Value("lang"); v.Text() != "go" || n.mode != listMode {
		t.Errorf("lang = %q, mode %v; want it kept and the list open", v.Text(), n.mode)
	}
	// What is sorted by clears where an option writes no sort.
	b := open(t, searchSpec(nil), WithTab(SortTab), WithQuery("sort:stars-desc"))
	b, _ = press(t, b, space, del)
	if b.Sort().By != "" || b.mode != rowsMode {
		t.Errorf("sort = %+v, mode %v; want best match", b.Sort(), b.mode)
	}
}

func TestDropdownFilterInsert(t *testing.T) {
	f := &fakeLoader{}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down, space, keyI)
	if !m.Capturing() || m.CapturedBy() != CapturePicker {
		t.Fatal("i didn't start the filter")
	}
	m = typeText(t, m, "doc")
	if m.pick.Len() != 1 {
		t.Fatalf("the list has %d items, want it narrowed to 1", m.pick.Len())
	}
	// esc keeps the filter and goes back to the list, whose keys move.
	m, _ = press(t, m, esc)
	if m.Capturing() || m.mode != listMode || m.pick.Query().Text != "doc" {
		t.Errorf("Capturing %v, mode %v, filter %q; want the list in normal mode, narrowed", m.Capturing(), m.mode, m.pick.Query().Text)
	}
	// i again, and enter checks the item, clears the filter and goes back.
	m, _ = press(t, m, keyI, enter)
	if v, _ := m.Value("labels"); !slices.Equal(v.List(), []string{"bug", "enhancement", "docs"}) {
		t.Errorf("labels = %q, want docs checked", v.List())
	}
	if m.mode != listMode || m.Capturing() || m.pick.Query().Text != "" || m.pick.Len() != len(labels) {
		t.Errorf("mode %v, Capturing %v, filter %q, %d items; want the whole list in normal mode", m.mode, m.Capturing(), m.pick.Query().Text, m.pick.Len())
	}
	if it, _ := m.pick.Selected(); it.Value != "docs" {
		t.Errorf("highlighted %v, want docs", it.Value)
	}
	// Two labels in one go: filter, enter, filter, enter, enter.
	m, _ = press(t, m, esc)
	n := open(t, prSpec(f.load), WithQuery(""))
	n, _ = press(t, n, down, down, down, space, keyI)
	n = typeText(t, n, "bug")
	n, _ = press(t, n, enter, keyI)
	n = typeText(t, n, "docs")
	n, _ = press(t, n, enter, enter)
	if v, _ := n.Value("labels"); !slices.Equal(v.List(), []string{"bug", "docs"}) || n.mode != rowsMode {
		t.Errorf("labels = %q, mode %v; want bug and docs and the list closed", v.List(), n.mode)
	}
	// In a list, enter chooses the narrowed item.
	l := open(t, languageSpec(nil))
	l, _ = press(t, l, space, keyI)
	l = typeText(t, l, "pyth")
	l, _ = press(t, l, enter)
	if v, _ := l.Value("language"); v.Text() != "python" || l.mode != rowsMode {
		t.Errorf("language = %q, mode %v; want python", v.Text(), l.mode)
	}
	// ↑ and ↓ move while typing, and j and k are letters.
	k := open(t, languageSpec(nil))
	k, _ = press(t, k, space, keyI, down, down, keyJ)
	if it, _ := k.pick.Selected(); k.pick.Query().Text != "j" || it.Value != "java" && it.Value != "javascript" {
		t.Errorf("filter %q, highlighted %v; want j typed", k.pick.Query().Text, it.Value)
	}
}

func TestPersonTypedItem(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, down, space, keyI)
	m = typeText(t, m, "hubot")
	if v := view(m); !strings.Contains(v, `↵ use "hubot"`) {
		t.Fatalf("the typed item isn't offered:\n%s", v)
	}
	m, _ = press(t, m, enter)
	if v, _ := m.Value("author"); v.Text() != "hubot" || m.mode != rowsMode {
		t.Errorf("author = %q, mode %v; want the typed login", v.Text(), m.mode)
	}
	// Text equal to an item, in any case, is that item.
	n := open(t, prSpec(nil))
	n, _ = press(t, n, down, space, keyI)
	n = typeText(t, n, "OctoCat")
	if v := view(n); strings.Contains(v, "use") {
		t.Errorf("a typed item is offered for an item that exists:\n%s", v)
	}
	n, _ = press(t, n, enter)
	if v, _ := n.Value("author"); v.Text() != "octocat" {
		t.Errorf("author = %q, want the item", v.Text())
	}
	// @me matches the @me item.
	o := open(t, prSpec(nil), WithQuery("author:octocat"))
	o, _ = press(t, o, down, space, keyI)
	o = typeText(t, o, "@me")
	if v := view(o); strings.Contains(v, "use") {
		t.Errorf("a typed item is offered for @me:\n%s", v)
	}
	o, _ = press(t, o, enter)
	if v, _ := o.Value("author"); v.Text() != "@me" {
		t.Errorf("author = %q, want @me", v.Text())
	}
}

// With the search failed, enter still takes what was typed.
func TestPersonTypedItemAfterFailedSearch(t *testing.T) {
	spec := Spec{Fields: []Field{{
		Key: "assignee", Label: "Assignee", Kind: Person, Qualifier: "assignee",
		Load: func(_ context.Context, q string) ([]Item, error) {
			if q == "" {
				return []Item{{Label: "@me", Value: "@me"}}, nil
			}
			return nil, errBoom
		},
	}}}
	m := open(t, spec)
	m, _ = press(t, m, space, keyI)
	m = typeText(t, m, "hubot")
	if m.pick.Err() == nil {
		t.Fatal("the search didn't fail")
	}
	m, _ = press(t, m, enter)
	if v, _ := m.Value("assignee"); v.Text() != "hubot" || m.mode != rowsMode {
		t.Errorf("assignee = %q, mode %v; want the typed login", v.Text(), m.mode)
	}
}

func TestPersonClear(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, down, space, del)
	if v, _ := m.Value("author"); v.Text() != "" || m.mode != rowsMode {
		t.Errorf("author = %q, mode %v; want it cleared and the list closed", v.Text(), m.mode)
	}
}

func TestRetryKeyReloads(t *testing.T) {
	f := &fakeLoader{fail: errBoom}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down, space)
	if m.picking || f.calls() != 1 {
		t.Fatalf("picking %v, %d calls; want a failed load", m.picking, f.calls())
	}
	f.setFail(nil)
	m, _ = press(t, m, keyR)
	if f.calls() != 2 || !m.picking || m.pick.Len() != len(labels) {
		t.Errorf("%d calls, picking %v, %d items; want the labels loaded again", f.calls(), m.picking, m.pick.Len())
	}
	// esc closes a failed list.
	f.setFail(errBoom)
	n := open(t, prSpec(f.load))
	n, _ = press(t, n, down, down, down, space, esc)
	if n.mode != rowsMode {
		t.Error("esc didn't close the failed list")
	}
}

// Space on a Multi that isn't loaded yet starts the load, and opens the
// list when it lands; a second space meanwhile doesn't load again.
func TestListOpensWhenTheLoadLands(t *testing.T) {
	f := &fakeLoader{}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down)
	m, cmd := m.Update(space)
	if m.picking || !m.Loading() || !strings.Contains(view(m), "Loading labels") {
		t.Fatalf("picking %v, loading %v; want the spinner in the dropdown:\n%s", m.picking, m.Loading(), view(m))
	}
	m, _ = run(t, m, cmd)
	if !m.picking || f.calls() != 1 {
		t.Errorf("picking %v after %d calls; want the list open", m.picking, f.calls())
	}
}

func TestDropdownPlacement(t *testing.T) {
	line := func(m Model, s string) int {
		for i, l := range strings.Split(view(m), "\n") {
			if strings.Contains(l, s) {
				return i
			}
		}
		return -1
	}
	// Under a top row.
	m := open(t, prSpec(nil), WithSize(80, 14))
	m, _ = press(t, m, space)
	if top, row := line(m, "╭─ State"), line(m, "State  "); top != row+1 {
		t.Errorf("the box starts at line %d, want the line under its row %d", top, row)
	}
	// Above the last row, which has no room below.
	s := open(t, lastChoiceSpec(nil), WithSize(80, 14))
	s, _ = press(t, s, keyBigG, keyK, space)
	if bottom, row := line(s, "╰"), line(s, "‹"); bottom != row-1 {
		t.Errorf("the box ends at line %d, want the line over its row %d", bottom, row)
	}
	// Never over the help line, whatever the height.
	for h := 4; h <= 20; h++ {
		for _, tt := range [][]tea.Msg{{space}, {down, down, space}, {keyBigG, keyK, space}} {
			f := open(t, lastChoiceSpec(nil), WithSize(80, h))
			f, _ = press(t, f, tt...)
			lines := strings.Split(view(f), "\n")
			if len(lines) != h || !strings.HasPrefix(strings.TrimSpace(lines[h-1]), "j/k") && f.mode == listMode && f.picking {
				t.Errorf("height %d: the last line is %q, want the help", h, lines[h-1])
			}
		}
	}
}

func TestTabSwitchClosesList(t *testing.T) {
	f := &fakeLoader{}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down, space, keyJ, space)
	m, _ = press(t, m, nextTab)
	if m.mode != rowsMode || m.Tab() != SortTab || m.picking {
		t.Errorf("mode %v, tab %v; want the list closed on the Sort tab", m.mode, m.Tab())
	}
	if v, _ := m.Value("labels"); !slices.Equal(v.List(), []string{"bug"}) {
		t.Errorf("labels = %q, want the uncheck kept", v.List())
	}
	m, _ = press(t, m, space, prevTab)
	if m.mode != rowsMode || m.Tab() != FiltersTab {
		t.Errorf("mode %v, tab %v; want the list closed on the Filters tab", m.mode, m.Tab())
	}
}

// A blur closes the list and keeps what was checked.
func TestBlurKeepsToggles(t *testing.T) {
	f := &fakeLoader{}
	m := open(t, prSpec(f.load))
	m, _ = press(t, m, down, down, down, space, space)
	m.Blur()
	if m.mode != rowsMode {
		t.Error("blur left the list open")
	}
	if v, _ := m.Value("labels"); !slices.Equal(v.List(), []string{"enhancement"}) {
		t.Errorf("labels = %q, want the toggle kept", v.List())
	}
}

// A resize with a list open places it again, and the rows keep their top.
func TestResizeWithListOpen(t *testing.T) {
	f := &fakeLoader{}
	m := open(t, prSpec(f.load), WithSize(80, 14))
	m, _ = press(t, m, down, down, down, space)
	for _, sz := range [][2]int{{60, 9}, {120, 30}, {80, 6}, {30, 5}, {80, 14}} {
		m.SetSize(sz[0], sz[1])
		assertFits(t, m.View(), sz[0], sz[1])
	}
	if v := view(m); !strings.Contains(v, "╭─ Labels") {
		t.Errorf("the list isn't back after the resizes:\n%s", v)
	}
}

// A Choice with no options, and with one: the list says so, and enter
// does nothing or chooses it.
func TestListWithFewOptions(t *testing.T) {
	none := open(t, Spec{Fields: []Field{{Key: "wf", Label: "Workflow", Kind: Choice, Qualifier: "workflow", Empty: "No workflows."}}})
	none, sent := press(t, none, space)
	if !strings.Contains(view(none), "No workflows.") {
		t.Errorf("the empty list says nothing:\n%s", view(none))
	}
	none, sent2 := press(t, none, enter)
	if none.mode != listMode || len(sent)+len(sent2) > 0 {
		t.Errorf("mode %v, sent %v; want enter to do nothing", none.mode, sent2)
	}
	one := open(t, Spec{Fields: []Field{{Key: "wf", Label: "Workflow", Kind: Choice, Qualifier: "workflow", Options: []Item{{"CI", "ci", ""}}}}})
	one, _ = press(t, one, space, enter)
	if v, _ := one.Value("wf"); v.Text() != "ci" || one.mode != rowsMode {
		t.Errorf("wf = %q, mode %v; want the only option", v.Text(), one.mode)
	}
}

// Free text in the query survives choosing from a list.
func TestFreeTextSurvivesList(t *testing.T) {
	m := open(t, prSpec(nil), WithQuery("is:open fix the crash"))
	m, _ = press(t, m, space, keyJ, enter)
	if got := m.Query(); !strings.HasSuffix(got, "fix the crash") || !strings.Contains(got, "is:closed") {
		t.Errorf("Query = %q, want closed and the free text", got)
	}
}

// Hostile names are cleaned inside the box, which keeps its frame.
func TestListHostileLabels(t *testing.T) {
	load := func(context.Context, string) ([]Item, error) {
		return []Item{{Label: "bug\x1b[31m\x1b]8;;http://x\x07evil\nline", Value: "bug", Detail: "d\x1b[2J"}}, nil
	}
	m := open(t, prSpec(load), WithSize(60, 14))
	m, _ = press(t, m, down, down, down, space)
	assertFits(t, m.View(), 60, 14)
	if v := m.View(); strings.Contains(v, "\x1b]8") || strings.Contains(v, "\x1b[2J") {
		t.Errorf("the view holds a hostile sequence: %q", v)
	}
}

// items counts the options a view shows, by their radio or check marks.
func items(v string) int {
	n := 0
	for l := range strings.SplitSeq(v, "\n") {
		if strings.Contains(l, "● ") || strings.Contains(l, "○ ") || strings.Contains(l, "[x] ") || strings.Contains(l, "[ ] ") {
			n++
		}
	}
	return n
}

// DropdownExtra gives a parent that sizes the form as the modal does, with
// the height of its rows, the rule, the query and the help line, room for
// all the items the dropdown shows.
func TestDropdownExtra(t *testing.T) {
	f := &fakeLoader{}
	for _, tt := range []struct {
		name  string
		spec  func() Spec
		keys  []tea.Msg
		items int
	}{
		{"labels", func() Spec { return prSpec(f.load) }, keys2(down, rowLabels, space), len(labels)},
		{"last row", func() Spec { return lastChoiceSpec(nil) }, []tea.Msg{keyBigG, keyK, space}, 5},
		{"language", func() Spec { return languageSpec(nil) }, []tea.Msg{space}, dropRows},
		{"dashboard language", func() Spec {
			s := languageSpec(nil)
			fields := make([]Field, 0, 6)
			for _, n := range []string{"Name", "Visibility", "Forks", "Archived", "Templates"} {
				fields = append(fields, Field{Key: n, Label: n, Kind: Text, Qualifier: n})
			}
			fields = append(fields, s.Fields[0])
			s.Fields = fields
			return s
		}, keys2(down, 5, space), dropRows},
		{"language last", func() Spec {
			s := languageSpec(nil)
			s.Fields[0], s.Fields[1] = s.Fields[1], s.Fields[0]
			return s
		}, []tea.Msg{down, space}, dropRows},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.spec()
			rows := len(spec.Fields)
			m := open(t, spec, WithSize(80, 10), WithTabBar(false))
			if got := m.DropdownExtra(rows, 80); got != 0 {
				t.Errorf("extra = %d with no list open, want 0", got)
			}
			m, _ = press(t, m, tt.keys...)
			extra := m.DropdownExtra(rows, 80)
			h := rows + 2 + m.QueryLines(80) + extra
			m.SetSize(80, h)
			if got := items(view(m)); got != tt.items {
				t.Errorf("%d items show in a form of %d lines, want %d:\n%s", got, h, tt.items, view(m))
			}
			if more := m.DropdownExtra(rows+extra, 80); more != 0 {
				t.Errorf("extra = %d with the room given, want 0", more)
			}
		})
	}
}

// A second enter reaches the form before the commands of the first have
// run, and applies; the first chose the person at once.
func TestPersonEnterIsSynchronous(t *testing.T) {
	m := open(t, prSpec(nil))
	m, _ = press(t, m, down, space, keyJ)
	m, c1 := m.Update(enter)
	if v, _ := m.Value("author"); v.Text() != "octocat" || m.mode != rowsMode {
		t.Fatalf("author = %q, mode %v after one enter; want the person chosen and the list closed", v.Text(), m.mode)
	}
	m, c2 := m.Update(enter)
	_, sent := run(t, m, tea.Batch(c1, c2))
	if len(sent) != 1 {
		t.Fatalf("sent %v, want one AppliedMsg", sent)
	}
	if a, ok := sent[0].(AppliedMsg); !ok || a.Values["author"].Text() != "octocat" {
		t.Errorf("sent %+v, want the form applied with octocat", sent[0])
	}
	// Enter and then esc: esc closes the form, it isn't taken by the list.
	n := open(t, prSpec(nil))
	n, _ = press(t, n, down, space, keyI)
	n = typeText(t, n, "hubot")
	n, c1 = n.Update(enter)
	n, c2 = n.Update(esc)
	_, sent = run(t, n, tea.Batch(c1, c2))
	if v, _ := n.Value("author"); v.Text() != "hubot" || len(sent) != 1 {
		t.Errorf("author = %q, sent %v; want hubot and the form closed", v.Text(), sent)
	}
	if _, ok := sent[0].(CancelMsg); !ok {
		t.Errorf("sent %+v, want a CancelMsg", sent[0])
	}
}

// With the group headers off, the typed item has none in the people list.
func TestPersonListHasNoTypedHeader(t *testing.T) {
	m := open(t, prSpec(nil), WithSize(80, 14))
	m, _ = press(t, m, down, space, keyI)
	m = typeText(t, m, "hubot")
	if v := view(m); strings.Contains(v, "Typed") || !strings.Contains(v, `use "hubot"`) {
		t.Errorf("the typed item has a header, or is missing:\n%s", v)
	}
}
