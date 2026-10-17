package conv

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

// sha returns a full SHA that starts with the seven hex digits of n.
func sha(n int) string { return fmt.Sprintf("%07x", n) + strings.Repeat("0", 33) }

// pushed is the commit oid that who pushed, mins minutes in.
func pushed(who core.User, oid string, mins int) core.TimelineItem {
	it := ev(core.TimelinePushed, who, mins)
	it.Commit = core.TimelineCommit{OID: oid, Headline: "Commit " + shortOID(oid), Author: who}
	return it
}

// forcePushed is a force push of who, mins minutes in, from one head to
// another.
func forcePushed(who core.User, from, to string, mins int) core.TimelineItem {
	it := ev(core.TimelineForcePushed, who, mins)
	it.From, it.To = from, to
	return it
}

// pushShape writes the top level as pushes with their commits, "!" after a
// forced one, and the other nodes by their kind or ID.
func pushShape(ns []*node) string {
	var parts []string
	for _, n := range ns {
		switch n.kind {
		case KindPush:
			var ks []string
			for _, k := range n.children {
				ks = append(ks, strings.TrimPrefix(k.id, "k:"))
			}
			s := "push(" + strings.Join(ks, ",") + ")"
			if n.forced {
				s += "!"
			}
			parts = append(parts, s)
		case KindEvent:
			parts = append(parts, "event")
		default:
			parts = append(parts, shape([]*node{n}))
		}
	}
	return strings.Join(parts, " ")
}

func TestPushes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		items []core.TimelineItem
		hide  []Group
		want  string
	}{
		{"one author joins", []core.TimelineItem{pushed(alice, "c1", 0), pushed(alice, "c2", 1), pushed(alice, "c3", 2)},
			nil, "push(c1,c2,c3)"},
		{"one commit", []core.TimelineItem{pushed(alice, "c1", 0)}, nil, "push(c1)"},
		{"two authors alternating", []core.TimelineItem{
			pushed(alice, "c1", 0), pushed(bert, "c2", 1), pushed(bert, "c3", 2), pushed(alice, "c4", 3),
		}, nil, "push(c1) push(c2,c3) push(c4)"},
		{"a comment between", []core.TimelineItem{pushed(alice, "c1", 0), comment("1"), pushed(alice, "c2", 2)},
			nil, "push(c1) c:1 push(c2)"},
		{"an event between", []core.TimelineItem{pushed(alice, "c1", 0), ev(core.TimelineClosed, alice, 1), pushed(alice, "c2", 2)},
			nil, "push(c1) event push(c2)"},
		{"a hidden kind between joins", []core.TimelineItem{
			pushed(alice, "c1", 0), ev(core.TimelineClosed, alice, 1), pushed(alice, "c2", 2),
		}, []Group{GroupState}, "push(c1,c2)"},
		{"a hidden comment between joins", []core.TimelineItem{pushed(alice, "c1", 0), comment("1"), pushed(alice, "c2", 2)},
			[]Group{GroupComments}, "push(c1,c2)"},
		{"hidden pushes leave nothing", []core.TimelineItem{pushed(alice, "c1", 0), comment("1")},
			[]Group{GroupPushes}, "c:1"},
		{"forced", []core.TimelineItem{
			pushed(alice, "c1", 0), forcePushed(alice, sha(1), sha(2), 1), pushed(alice, "c2", 2), pushed(alice, "c3", 3),
		}, nil, "push(c1) push(c2,c3)!"},
		{"forced by another author", []core.TimelineItem{
			pushed(alice, "c1", 0), forcePushed(bert, sha(1), sha(2), 1), pushed(bert, "c2", 2),
		}, nil, "push(c1) push(c2)!"},
		{"a lone force push is a line", []core.TimelineItem{
			pushed(alice, "c1", 0), forcePushed(alice, sha(1), sha(2), 1), comment("1"),
		}, nil, "push(c1) event c:1"},
		{"a lone force push at the end", []core.TimelineItem{pushed(alice, "c1", 0), forcePushed(alice, sha(1), sha(2), 1)},
			nil, "push(c1) event"},
		{"two force pushes in a row", []core.TimelineItem{
			forcePushed(alice, sha(1), sha(2), 0), forcePushed(alice, sha(2), sha(3), 1), pushed(alice, "c1", 2),
		}, nil, "event push(c1)!"},
		{"a force push splits a push", []core.TimelineItem{
			pushed(alice, "c1", 0), forcePushed(alice, sha(1), sha(2), 1), pushed(alice, "c2", 2),
		}, nil, "push(c1) push(c2)!"},
		{"a push closes a run of comments", []core.TimelineItem{
			comment("1"), comment("2"), pushed(alice, "c1", 0), comment("3"), comment("4"),
		}, nil, "run:1[rc:1 rc:2] push(c1) run:3[rc:3 rc:4]"},
		{"a login tells authors apart", []core.TimelineItem{
			pushed(alice, "c1", 0), pushed(core.User{Login: "alice2", Name: "Alice Chen"}, "c2", 1),
		}, nil, "push(c1) push(c2)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var o Options
			for _, g := range tc.hide {
				o.hide[g] = true
			}
			if got := pushShape(tree(Data{Items: tc.items}, o)); got != tc.want {
				t.Errorf("tree = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPushAuthorFallsBackToTheCommit(t *testing.T) {
	t.Parallel()
	a, b := pushed(core.User{}, "c1", 0), pushed(core.User{}, "c2", 1)
	a.Commit.Author, b.Commit.Author = core.User{Name: "Dee"}, core.User{Name: "Dee"}
	c := pushed(core.User{}, "c3", 2)
	c.Commit.Author = core.User{Name: "Eve"}
	if got := pushShape(tree(Data{Items: []core.TimelineItem{a, b, c}}, Options{})); got != "push(c1,c2) push(c3)" {
		t.Errorf("tree = %q, want the commits' authors to tell the pushes apart", got)
	}
}

// pushLine returns the first line of the row id, without colors or the
// trailing space.
func pushLine(rows []foldtree.Row, id string) string {
	for _, r := range rows {
		if r.ID == id {
			return strings.TrimRight(ansi.Strip(r.Lines[0]), " ")
		}
	}
	return ""
}

func TestPushLines(t *testing.T) {
	t.Parallel()
	o := opts(config.IconsASCII, 120)
	cut, dot := o.Styles.Glyphs.Cut, o.Styles.Glyphs.Dot
	items := []core.TimelineItem{
		pushed(alice, sha(0xa1b2c3d), 0), pushed(alice, sha(0xb2c3d4e), 1), pushed(alice, sha(0xc3d4e5f), 2),
		comment("1"),
		forcePushed(bert, sha(1), sha(2), 3), pushed(bert, sha(0xd4e5f60), 4),
		comment("2"),
		forcePushed(carol, sha(0xaaaaaaa), sha(0xbbbbbbb), 5),
	}
	nodes := tree(Data{Items: items}, o)
	rows, byNode := build(Data{Items: items}, o, true)
	three := "Alice Chen pushed 3 commits (a1b2c3d" + cut + "c3d4e5f)"
	forced := "Bert Moss pushed 1 commit (d4e5f60) " + dot + " forced"
	for _, tc := range []struct {
		name, id, want string
		form           int
	}{
		{"closed", nodes[0].id, three, 0},
		{"open", nodes[0].id, three, 1},
		{"forced", "push:" + sha(0xd4e5f60), forced, 0},
	} {
		n := byNode[tc.id]
		if n == nil {
			t.Fatalf("%s: no node %q", tc.name, tc.id)
		}
		_, got, _ := strings.Cut(strings.TrimSpace(ansi.Strip(n.forms[tc.form])), " ")
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
	closed, open := ansi.Strip(byNode[nodes[0].id].forms[0]), ansi.Strip(byNode[nodes[0].id].forms[1])
	if !strings.HasPrefix(closed, " ") {
		t.Errorf("closed line %q is not centred", closed)
	}
	if strings.HasPrefix(open, " ") {
		t.Errorf("open line %q is not at the left", open)
	}
	if got := pushLine(rows, nodes[0].id); got != strings.TrimRight(closed, " ") {
		t.Errorf("the row of a push = %q, want its closed line %q", got, closed)
	}
	var lone string
	for _, r := range rows {
		if strings.HasPrefix(r.ID, "e:") {
			_, lone, _ = strings.Cut(strings.TrimSpace(ansi.Strip(r.Lines[0])), " ")
		}
	}
	if want := "Carol Diaz force-pushed the branch from aaaaaaa to bbbbbbb"; lone != want {
		t.Errorf("lone force push = %q, want %q", lone, want)
	}
	r := byID(rows)["push:"+sha(0xa1b2c3d)]
	if !r.Fold || !r.Closed || r.Marker != "" || r.Right == "" {
		t.Errorf("push row = %+v, want a closed fold with no marker and an age", r)
	}
	k := byID(rows)["k:"+sha(0xb2c3d4e)]
	if k.Depth != 1 || !k.Action || k.Fold || k.Kind != KindCommit {
		t.Errorf("commit row = %+v, want a depth 1 action", k)
	}
	if got := ansi.Strip(k.Lines[0]); got != "b2c3d4e  Commit b2c3d4e" {
		t.Errorf("commit line = %q", got)
	}
}

func TestPushIDsStable(t *testing.T) {
	t.Parallel()
	items := []core.TimelineItem{pushed(alice, "c1", 0), pushed(alice, "c2", 1), comment("1")}
	ids := func(items []core.TimelineItem) []string {
		var out []string
		for _, r := range Build(Data{Items: items}, opts(config.IconsASCII, 100)) {
			if r.Kind == KindPush || r.Kind == KindCommit {
				out = append(out, r.ID)
			}
		}
		return out
	}
	before := ids(items)
	more := slices.Concat(items, []core.TimelineItem{pushed(alice, "c3", 3), pushed(bert, "c4", 4)})
	after := ids(more)
	if len(before) != 3 || !slices.Equal(before, after[:3]) {
		t.Errorf("IDs = %q, then %q after a page, want the first three kept", before, after)
	}
	// A page that ends a push in two keeps the ID of the push.
	split := ids([]core.TimelineItem{pushed(alice, "c1", 0)})
	joined := ids([]core.TimelineItem{pushed(alice, "c1", 0), pushed(alice, "c2", 1)})
	if split[0] != joined[0] {
		t.Errorf("push ID = %q, then %q once a commit joined it", split[0], joined[0])
	}
}

func TestPreviewHasNoPushes(t *testing.T) {
	t.Parallel()
	d := Data{Items: []core.TimelineItem{comment("1"), pushed(alice, "c1", 0), forcePushed(alice, sha(1), sha(2), 1), comment("2")}}
	if got := shape(tree(d, Options{preview: true})); got != "c:1 c:2" {
		t.Errorf("preview = %q, want the comments alone", got)
	}
}

func TestCountsPushes(t *testing.T) {
	t.Parallel()
	f := newFake([][]core.TimelineItem{{
		pushed(alice, "c1", 0), pushed(alice, "c2", 1), pushed(bert, "c3", 2), comment("1"),
		forcePushed(bert, sha(1), sha(2), 3), pushed(bert, "c4", 4), forcePushed(bert, sha(2), sha(3), 5),
	}}, [][]core.Thread{nil})
	d := newDriver(t, f)
	d.all()
	if c := d.m.Choices()[GroupPushes]; c.Count != 4 {
		t.Errorf("pushes = %d, want 4: two pushes, a forced one and a lone force push", c.Count)
	}
}

// pushKeys are the keys of a tree that open and close folds.
func pushKeys() foldtree.KeyMap {
	return foldtree.NewKeyMap(keytest.Table(map[string][]string{
		"up":            {"up"},
		"down":          {"down"},
		"expand":        {"right"},
		"collapse":      {"left"},
		"global.select": {"enter"},
	}))
}

func TestPushFormFollowsFold(t *testing.T) {
	t.Parallel()
	f := newFake([][]core.TimelineItem{{
		pushed(alice, sha(1), 0), pushed(alice, sha(2), 1), comment("1"),
	}}, [][]core.Thread{nil})
	d := newDriver(t, f, WithSize(100, 40), WithOptions(opts0()), WithTreeKeys(pushKeys()))
	d.all()
	d.m.Focus()
	id := "push:" + sha(1)
	line := func() string { return ansi.Strip(d.m.views[""].rows[0].Lines[0]) }
	selected := func() string {
		t.Helper()
		got, _, ok := d.m.Selected()
		if !ok {
			t.Fatal("nothing is selected")
		}
		return got
	}
	if d.m.views[""].rows[0].ID != id || !strings.HasPrefix(line(), " ") {
		t.Fatalf("closed push = %q, want it centred", line())
	}
	d.send(tea.KeyPressMsg{Code: tea.KeyRight})
	if strings.HasPrefix(line(), " ") || !strings.Contains(ansi.Strip(d.m.View()), "Commit 0000002") {
		t.Errorf("open push = %q, want it at the left with its commits under it:\n%s", line(), ansi.Strip(d.m.View()))
	}
	if got := selected(); got != id {
		t.Errorf("cursor = %q after opening, want it to stay on %q", got, id)
	}
	d.send(tea.KeyPressMsg{Code: tea.KeyLeft})
	if !strings.HasPrefix(line(), " ") {
		t.Errorf("closed again push = %q, want it centred", line())
	}
	if got := selected(); got != id {
		t.Errorf("cursor = %q after closing, want it to stay on %q", got, id)
	}
	// A reload keeps the form of the fold the user left.
	d.send(tea.KeyPressMsg{Code: tea.KeyRight})
	d.m.version++
	d.m.sync()
	if strings.HasPrefix(line(), " ") {
		t.Errorf("push after a rebuild = %q, want it open and at the left", line())
	}
}

// TestPushFormAfterFilter checks a push that a filter change joins to another
// and the cursor then opens: it shows the line of its open form.
func TestPushFormAfterFilter(t *testing.T) {
	t.Parallel()
	d := newDriver(t, newFake([][]core.TimelineItem{{
		pushed(alice, sha(1), 0), comment("1"), pushed(alice, sha(2), 2),
	}}, [][]core.Thread{nil}), WithSize(100, 40), WithOptions(opts0()), WithTreeKeys(pushKeys()))
	d.all()
	v := d.m.views[""]
	v.tree.Reveal("k:" + sha(2))
	d.m.SetShown(allBut(GroupComments))

	v = d.m.views[""]
	id := "push:" + sha(1)
	if v.tree.Folded(id) {
		t.Fatalf("push %q is closed, want the cursor to have opened it (rows %v)", id, d.rowIDs())
	}
	for _, r := range v.rows {
		if r.ID == id && r.Lines[0] != v.nodes[id].forms[1] {
			t.Errorf("open push = %q, want its open form %q", r.Lines[0], v.nodes[id].forms[1])
		}
	}
}

// openPush opens the push id of rows built with the nodes: the fold and its
// line.
func openPush(rows []foldtree.Row, nodes map[string]*node, id string) {
	for i, r := range rows {
		if r.ID == id {
			rows[i].Closed = false
			rows[i].Lines = []string{nodes[id].forms[1]}
		}
	}
}

func TestPushesGolden(t *testing.T) {
	t.Parallel()
	d := pushesFixture()
	for _, w := range []int{80, 120, 160} {
		for _, open := range []bool{false, true} {
			name := strconv.Itoa(w)
			if open {
				name += "-open"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				rows, nodes := build(d, opts(config.IconsNerd, w), true)
				if open {
					openPush(rows, nodes, "push:"+sha(0xa1b2c3d))
				}
				golden.RequireEqual(t, text(rows, w, false))
			})
		}
	}
}

// pushesFixture is a conversation of pushes among other items.
func pushesFixture() Data {
	ago := func(it core.TimelineItem, h int) core.TimelineItem {
		it.At = now.Add(-time.Duration(h) * time.Hour)
		return it
	}
	first := comment("c1")
	first.Actor, first.Body = alice, "Opening this for review."
	last := comment("c2")
	last.Actor, last.Body = bert, "Looks fine to me."
	return Data{Items: []core.TimelineItem{
		ago(first, 80),
		ago(pushed(alice, sha(0xa1b2c3d), 0), 79),
		ago(pushed(alice, sha(0xb2c3d4e), 0), 78),
		ago(pushed(alice, sha(0xc3d4e5f), 0), 77),
		ago(last, 70),
		ago(forcePushed(alice, sha(0xc3d4e5f), sha(0xd4e5f60), 0), 60),
		ago(pushed(alice, sha(0xd4e5f60), 0), 59),
		ago(pushed(bert, sha(0xe5f6071), 0), 40),
		ago(forcePushed(bert, sha(0xe5f6071), sha(0xf607182), 0), 20),
	}}
}
