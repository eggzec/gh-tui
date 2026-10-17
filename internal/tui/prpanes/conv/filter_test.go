package conv

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// allBut returns the groups with the given ones off.
func allBut(off ...Group) (on [NumGroups]bool) {
	for g := range on {
		on[g] = !slices.Contains(off, Group(g))
	}
	return on
}

func TestEveryKindHasGroup(t *testing.T) {
	t.Parallel()
	f, err := parser.ParseFile(token.NewFileSet(), "../../../core/timeline.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	ast.Inspect(f, func(x ast.Node) bool {
		spec, ok := x.(*ast.ValueSpec)
		if !ok || spec.Type == nil || len(spec.Values) != len(spec.Names) {
			return true
		}
		if id, ok := spec.Type.(*ast.Ident); !ok || id.Name != "TimelineKind" {
			return true
		}
		for i, name := range spec.Names {
			n++
			kind, err := strconv.Unquote(spec.Values[i].(*ast.BasicLit).Value)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := groupOf(core.TimelineKind(kind)); !ok {
				t.Errorf("%s (%q) has no group: add it to kindGroups", name.Name, kind)
			}
		}
		return true
	})
	if n < 20 {
		t.Fatalf("found only %d kinds in core/timeline.go", n)
	}
	if len(kindGroups) != n {
		t.Errorf("kindGroups has %d kinds, core has %d", len(kindGroups), n)
	}
}

func TestGroupNamesAreTheConfigs(t *testing.T) {
	t.Parallel()
	if !slices.Equal(groupNames[:], config.ConversationGroups) {
		t.Errorf("group names = %v, config has %v", groupNames, config.ConversationGroups)
	}
	cfg := config.Config{}
	cfg.UI.Conversation.Filter = []string{"reviews", "state"}
	m := New(Source{}, Configure(cfg))
	for _, c := range m.Choices() {
		if want := c.Group == GroupReviews || c.Group == GroupState; c.On != want {
			t.Errorf("%s is on = %v, want %v", c.Name, c.On, want)
		}
	}
}

func TestFilterHidesKinds(t *testing.T) {
	t.Parallel()
	items := []core.TimelineItem{comment("1"), review("R1", core.ReviewStateCommented), comment("2"), comment("3")}
	ths := []core.Thread{thread("T1", "R1", 2)}
	draft := pend(thread("T9", "", 1))
	for _, tc := range []struct {
		name string
		off  []Group
		want string
	}{
		{"everything", nil, "c:1 r:R1[t:T1[p:T1-b]] run:2[rc:2 rc:3] r:pending[t:T9]"},
		{"reviews off hides threads and the drafts", []Group{GroupReviews}, "run:1[rc:1 rc:2 rc:3]"},
		{"comments off", []Group{GroupComments}, "r:R1[t:T1[p:T1-b]] r:pending[t:T9]"},
		{"both off", []Group{GroupComments, GroupReviews}, ""},
		{"another group changes nothing", []Group{GroupPushes, GroupLabels}, "c:1 r:R1[t:T1[p:T1-b]] run:2[rc:2 rc:3] r:pending[t:T9]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := Options{}
			for _, g := range tc.off {
				o.hide[g] = true
			}
			got := shape(tree(Data{Items: items, Threads: append(slices.Clone(ths), draft)}, o))
			if got != tc.want {
				t.Errorf("tree = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestChoicesCounts(t *testing.T) {
	t.Parallel()
	draft := pend(thread("T9", "RP", 1))
	f := newFake(
		[][]core.TimelineItem{{
			comment("1"), comment("2"), review("R1", core.ReviewStateCommented),
			event("unknown"), review("RP", core.ReviewStatePending),
		}},
		[][]core.Thread{{thread("T1", "R1", 1), draft}},
	)
	d := newDriver(t, f)
	d.all()
	got := d.m.Choices()
	if len(got) != int(NumGroups) {
		t.Fatalf("%d choices, want %d", len(got), NumGroups)
	}
	for _, c := range got {
		want := map[Group]int{GroupComments: 2, GroupReviews: 2}[c.Group]
		if c.Count != want || !c.On || c.Name != c.Group.String() {
			t.Errorf("%+v, want count %d, on", c, want)
		}
	}
	if n, total := d.m.Shown(); n != 4 || total != 4 {
		t.Errorf("shown %d of %d, want 4 of 4", n, total)
	}
	d.m.SetShown(allBut(GroupReviews))
	if n, total := d.m.Shown(); n != 2 || total != 4 {
		t.Errorf("with reviews off, shown %d of %d, want 2 of 4", n, total)
	}
	if c := d.m.Choices()[GroupReviews]; c.On || c.Count != 2 {
		t.Errorf("reviews = %+v, want off and still counted", c)
	}
}

func TestFilterName(t *testing.T) {
	t.Parallel()
	only := func(gs ...Group) (on [NumGroups]bool) {
		for _, g := range gs {
			on[g] = true
		}
		return on
	}
	for _, tc := range []struct {
		on   [NumGroups]bool
		want string
	}{
		{allBut(), ""},
		{only(GroupReviews), "reviews"},
		{only(GroupComments, GroupState), "comments, state"},
		{only(GroupComments, GroupState, GroupPushes), "3 kinds"},
		{allBut(GroupLabels), "8 kinds"},
		{only(), "nothing"},
	} {
		m := New(Source{})
		m.SetShown(tc.on)
		if got := m.FilterName(); got != tc.want {
			t.Errorf("FilterName(%v) = %q, want %q", tc.on, got, tc.want)
		}
	}
}

func TestFilterKeepsFolds(t *testing.T) {
	t.Parallel()
	d := newDriver(t, conversation())
	d.all()
	tr := &d.m.views[""].tree
	was := tr.Folded("t:T1")
	tr.SetFolded("t:T1", !was)

	d.m.SetShown(allBut(GroupReviews))
	if slices.Contains(d.rowIDs(), "t:T1") {
		t.Fatalf("the thread stays with its review hidden: %v", d.rowIDs())
	}
	d.m.SetShown(allBut())
	if got := d.m.views[""].tree.Folded("t:T1"); got == was {
		t.Errorf("the thread is folded = %v after the review came back, want %v", got, !was)
	}
}

func TestFilterCursorToAncestor(t *testing.T) {
	t.Parallel()
	onRow := func(d *driver, want string) {
		t.Helper()
		if id, _, _ := d.m.Selected(); id != want {
			t.Errorf("the cursor is on %q, want %q (rows %v)", id, want, d.rowIDs())
		}
	}
	t.Run("a reply of a hidden review goes to the row before the review", func(t *testing.T) {
		t.Parallel()
		d := newDriver(t, conversation())
		d.all()
		d.m.views[""].tree.Reveal("p:T1-b")
		d.m.SetShown(allBut(GroupReviews))
		onRow(d, "rc:1")
	})
	t.Run("a comment that joins a run keeps the cursor", func(t *testing.T) {
		t.Parallel()
		d := newDriver(t, conversation())
		d.all()
		d.m.views[""].tree.Reveal("c:2")
		d.m.SetShown(allBut(GroupReviews))
		onRow(d, "rc:2")
	})
	t.Run("a run that joins another goes to its first comment", func(t *testing.T) {
		t.Parallel()
		d := newDriver(t, newFake([][]core.TimelineItem{{comment("1"), comment("2"), review("R1", core.ReviewStateCommented), comment("3"), comment("4")}}, [][]core.Thread{{}}))
		d.all()
		d.m.views[""].tree.Reveal("run:3")
		d.m.SetShown(allBut(GroupReviews))
		onRow(d, "rc:3")
	})
	t.Run("a hidden first row goes to the first row", func(t *testing.T) {
		t.Parallel()
		d := newDriver(t, newFake([][]core.TimelineItem{{review("R1", core.ReviewStateCommented), comment("1")}}, [][]core.Thread{{}}))
		d.all()
		d.m.views[""].tree.Reveal("r:R1")
		d.m.SetShown(allBut(GroupReviews))
		onRow(d, "c:1")
	})
	// eventRows returns the IDs of the system lines, in order.
	eventRows := func(d *driver) []string {
		var ids []string
		for _, id := range d.rowIDs() {
			if strings.HasPrefix(id, "e:") {
				ids = append(ids, id)
			}
		}
		return ids
	}
	t.Run("a hidden system line goes to the row before it", func(t *testing.T) {
		t.Parallel()
		closed := ev(core.TimelineClosed, alice, 30)
		items := []core.TimelineItem{comment("1"), labeled(bert, "bug", 5), comment("2"), review("R3", core.ReviewStateCommented), closed, comment("4")}
		d := newDriver(t, newFake([][]core.TimelineItem{items}, [][]core.Thread{{}}))
		d.all()
		lines := eventRows(d)
		if len(lines) != 2 {
			t.Fatalf("system lines = %v, want two", lines)
		}
		d.m.views[""].tree.Reveal(lines[1])
		d.m.SetShown(allBut(GroupState))
		onRow(d, "r:R3")
	})
	t.Run("a system line that merges goes to the line it merged into", func(t *testing.T) {
		t.Parallel()
		items := []core.TimelineItem{comment("0"), labeled(alice, "bug", 0), comment("1"), labeled(alice, "ui", 2)}
		d := newDriver(t, newFake([][]core.TimelineItem{items}, [][]core.Thread{{}}))
		d.all()
		lines := eventRows(d)
		if len(lines) != 2 {
			t.Fatalf("system lines = %v, want two", lines)
		}
		d.m.views[""].tree.Reveal(lines[1])
		d.m.SetShown(allBut(GroupComments))
		merged := eventRows(d)
		if len(merged) != 1 {
			t.Fatalf("system lines = %v after comments are hidden, want one", merged)
		}
		onRow(d, merged[0])
	})
	t.Run("a row that stays keeps the cursor", func(t *testing.T) {
		t.Parallel()
		d := newDriver(t, conversation())
		d.all()
		d.m.views[""].tree.Reveal("p:T1-b")
		d.m.SetShown(allBut(GroupComments))
		onRow(d, "p:T1-b")
	})
}

func TestEmptyHint(t *testing.T) {
	t.Parallel()
	text := func(d *driver) string { return ansi.Strip(d.m.View()) }
	d := newDriver(t, newFake([][]core.TimelineItem{{comment("1")}}, [][]core.Thread{{}}))
	d.all()
	if got := text(d); strings.Contains(got, noMatch) {
		t.Fatalf("a conversation with rows says nothing matches:\n%s", got)
	}

	d.m.SetFilterHint("f")
	d.m.SetShown(allBut(GroupComments))
	if got := text(d); !strings.Contains(got, noMatch+" (f changes it)") {
		t.Errorf("the empty text with a key = \n%s", got)
	}
	d.m.SetFilterHint("")
	if got := text(d); !strings.Contains(got, noMatch) || strings.Contains(got, "changes it") {
		t.Errorf("the empty text without a key = \n%s", got)
	}
	d.m.SetShown(allBut())
	if got := text(d); strings.Contains(got, noMatch) || !strings.Contains(got, "body 1") {
		t.Errorf("with the filter off again:\n%s", got)
	}
}

func TestEmptyGolden(t *testing.T) {
	t.Parallel()
	for _, hint := range []string{"", "f"} {
		for _, w := range []int{80, 120, 160} {
			t.Run(hint+"-"+strconv.Itoa(w), func(t *testing.T) {
				t.Parallel()
				d := newDriver(t, newFake([][]core.TimelineItem{{comment("1")}}, [][]core.Thread{{}}),
					WithSize(w, 6), WithOptions(opts(config.IconsNerd, w)), WithTreeKeys(treeKeys()))
				d.all()
				d.m.SetFilterHint(hint)
				d.m.SetShown(allBut(GroupComments))
				lines := strings.Split(ansi.Strip(d.m.View()), "\n")
				for i, l := range lines {
					lines[i] = strings.TrimRight(l, " ")
				}
				golden.RequireEqual(t, strings.TrimRight(strings.Join(lines, "\n"), "\n")+"\n")
			})
		}
	}
}

func TestPreviewIgnoresFilter(t *testing.T) {
	t.Parallel()
	d := newDriver(t, conversation())
	d.all()
	d.m.SetShown(allBut(GroupReviews))
	d.m.SetView("p", Narrow{
		Item:   func(it core.TimelineItem) bool { return it.ID == "R1" },
		Thread: func(th core.Thread) bool { return th.ReviewID == "R1" },
	})
	if got, want := d.rowIDs(), []string{"r:R1", "t:T1", "p:T1-b"}; !slices.Equal(got, want) {
		t.Errorf("preview rows = %v, want %v", got, want)
	}
}
