package involved

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

var t0 = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func styles(tb testing.TB, dark bool) Styles {
	tb.Helper()
	p, err := config.Default().Palette(dark)
	if err != nil {
		tb.Fatal(err)
	}
	return NewStyles(ui.NewTheme(p, dark), ui.NewIcons(config.IconsUnicode))
}

func user(login, name string) core.User { return core.User{Login: login, Name: name} }

func said(u core.User, minutes int) core.TimelineItem {
	return core.TimelineItem{Kind: core.TimelineComment, Actor: u, At: t0.Add(time.Duration(minutes) * time.Minute)}
}

func verdict(u core.User, s core.ReviewState) core.Verdict { return core.Verdict{Author: u, State: s} }

// everything has a reviewer in each state, a team, a bot, assignees and four
// commenters.
func everything() Data {
	return Data{
		Author:    user("ann", "Ann Author"),
		Assignees: []core.User{user("bob", "Bob"), user("ann", "Ann Author")},
		Reviewers: core.Reviewers{
			Requested: []core.ReviewRequest{
				{User: user("rita", "Rita")},
				{User: user("carl", "Carl")},
				{Team: "acme/platform", TeamName: "Platform infrastructure"},
				{Unknown: true},
			},
			RequestedTotal: 4,
			Latest: []core.Verdict{
				verdict(user("carl", "Carl"), core.ReviewStateApproved),
				verdict(user("dina", "Dina"), core.ReviewStateApproved),
				verdict(user("eve", "Eve"), core.ReviewStateChangesRequested),
				verdict(user("finn", "Finn"), core.ReviewStateCommented),
				verdict(user("gus", "Gus"), core.ReviewStateDismissed),
				verdict(core.User{Login: "copilot", Name: "Copilot", Bot: true}, core.ReviewStateCommented),
			},
			LatestTotal: 6,
		},
		Items: []core.TimelineItem{
			said(user("zed", "Zed"), 1), said(user("dina", "Dina"), 2), said(user("yan", "Yan"), 3),
			said(core.User{Login: "dependabot", Bot: true}, 4), said(user("zed", "Zed"), 5), said(user("ann", "Ann Author"), 6),
		},
		Threads: []core.Thread{{Comments: []core.ThreadComment{{Author: user("xia", "Xia"), At: t0.Add(7 * time.Minute)}}}},
	}
}

func frame(d Data, o Options, width int) string {
	rows := Rows(d, o)
	tree := foldtree.New(foldtree.WithSize(width, len(rows)), foldtree.WithRows(rows))
	lines := strings.Split(tree.View(), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestGoldens(t *testing.T) {
	t.Parallel()
	few := Data{Author: user("ann", "Ann"), Items: []core.TimelineItem{said(user("zed", "Zed"), 1)}}
	for _, theme := range []struct {
		name string
		dark bool
	}{{"dark", true}, {"light", false}} {
		for _, sc := range []struct {
			name string
			d    Data
		}{{"all", everything()}, {"empty", Data{Author: user("ann", "Ann")}}, {"few", few}} {
			t.Run(theme.name+"/"+sc.name, func(t *testing.T) {
				t.Parallel()
				var b strings.Builder
				for _, w := range []int{40, 80, 120} {
					fmt.Fprintf(&b, "== width %d ==\n%s", w, frame(sc.d, Options{Styles: styles(t, theme.dark)}, w))
				}
				golden.RequireEqual(t, b.String())
			})
		}
	}
}

func TestReviewerStates(t *testing.T) {
	t.Parallel()
	rows, who := build(everything(), Options{Styles: styles(t, true)})
	want := map[string]State{
		"reviewer:user:rita":          Requested,
		"reviewer:user:carl":          Rerequested,
		"reviewer:team:acme/platform": Requested,
		"reviewer:user:dina":          Approved,
		"reviewer:user:eve":           ChangesRequested,
		"reviewer:user:finn":          Commented,
		"reviewer:user:gus":           Dismissed,
		"reviewer:user:copilot":       Commented,
	}
	for id, st := range want {
		if got := who[id].State; got != st {
			t.Errorf("%s: state %v, want %v", id, got, st)
		}
	}
	if p := who["reviewer:team:acme/platform"]; p.Name != "Platform infrastructure" || p.Login != "" {
		t.Errorf("team person = %+v", p)
	}
	// The request that can't be named has no row, and counts.
	if !strings.Contains(rows[1].Lines[0], "9") {
		t.Errorf("reviewers header %q does not count 9", rows[1].Lines[0])
	}
}

// The author's replies in review threads are reviews to GitHub, but the author
// is not a reviewer: no row, nothing to re-request, and not in the count.
func TestAuthorIsNotAReviewer(t *testing.T) {
	t.Parallel()
	d := everything()
	ann := user("ann", "Ann Author")
	d.Reviewers.Latest = append(slices.Clone(d.Reviewers.Latest), verdict(ann, core.ReviewStateCommented))
	d.Reviewers.LatestTotal++
	d.Reviewers.Verdicts = []core.Verdict{verdict(ann, core.ReviewStateCommented)}
	rows, who := build(d, Options{Styles: styles(t, true)})
	if _, ok := who["reviewer:user:ann"]; ok {
		t.Error("the author has a reviewer row")
	}
	if !strings.Contains(rows[1].Lines[0], "9") {
		t.Errorf("reviewers header %q does not count 9", rows[1].Lines[0])
	}
}

func TestCommenters(t *testing.T) {
	t.Parallel()
	_, who := build(everything(), Options{Styles: styles(t, true)})
	var got []string
	for id := range who {
		if strings.HasPrefix(id, "commenter:") {
			got = append(got, id)
		}
	}
	slices.Sort(got)
	// Not the author, the reviewers or the assignees, nobody twice.
	want := []string{"commenter:dependabot", "commenter:xia", "commenter:yan", "commenter:zed"}
	if !slices.Equal(got, want) {
		t.Errorf("commenters = %v, want %v", got, want)
	}
	people := compose(everything()).commenters
	order := make([]string, 0, len(people))
	for _, u := range people {
		order = append(order, u.Login)
	}
	if want := []string{"zed", "yan", "dependabot", "xia"}; !slices.Equal(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestDefaultFolds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		n      int
		closed bool
	}{{0, false}, {3, false}, {4, true}} {
		var d Data
		for i := range tc.n {
			d.Items = append(d.Items, said(user(fmt.Sprint("u", i), ""), i))
		}
		got := DefaultFolds(d)
		if got[idCommenters] != tc.closed || got[idReviewers] || got[idAssignees] {
			t.Errorf("%d commenters: folds %v", tc.n, got)
		}
		for _, r := range Rows(d, Options{Styles: styles(t, true)}) {
			if r.ID == idCommenters && r.Closed != tc.closed {
				t.Errorf("%d commenters: row closed = %v", tc.n, r.Closed)
			}
		}
	}
}

func TestStableIDs(t *testing.T) {
	t.Parallel()
	rows := Rows(everything(), Options{Styles: styles(t, true)})
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	want := []string{
		"author", "group:reviewers", "reviewer:user:rita", "reviewer:user:carl", "reviewer:team:acme/platform",
		"reviewer:user:dina", "reviewer:user:eve", "reviewer:user:finn", "reviewer:user:gus", "reviewer:user:copilot",
		"group:assignees", "assignee:bob", "assignee:ann",
		"group:commenters", "commenter:zed", "commenter:yan", "commenter:dependabot", "commenter:xia",
	}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v\nwant %v", ids, want)
	}
}

func TestEmptyRowsHaveNoAction(t *testing.T) {
	t.Parallel()
	for _, r := range Rows(Data{Author: user("ann", "Ann")}, Options{Styles: styles(t, true)}) {
		if strings.HasPrefix(r.ID, "empty:") && r.Action {
			t.Errorf("%s has an action", r.ID)
		}
	}
}

func TestEnter(t *testing.T) {
	t.Parallel()
	m := New(foldtree.WithSize(80, 30), foldtree.WithKeyMap(foldtree.NewKeyMap(keytest.Table(map[string][]string{
		"up": {"up"}, "down": {"down"}, "global.select": {"enter"},
	}))), foldtree.WithFocused(true))
	m.SetData(everything(), Options{Styles: styles(t, true)})
	enter := func(id string) tea.Msg {
		var cmd tea.Cmd
		m, cmd = m.Update(foldtree.EnterMsg{Tree: m.ID(), ID: id})
		if cmd == nil {
			return nil
		}
		return cmd()
	}
	if msg := enter("reviewer:user:dina"); msg != (FocusPersonMsg{Login: "dina", Name: "Dina"}) {
		t.Errorf("enter on a person = %#v", msg)
	}
	if msg := enter("reviewer:team:acme/platform"); msg != (FocusPersonMsg{Team: "acme/platform", Name: "Platform infrastructure"}) {
		t.Errorf("enter on a team = %#v", msg)
	}
	if m.Folded("group:reviewers") {
		t.Fatal("reviewers start closed")
	}
	if msg := enter("group:reviewers"); msg != nil || !m.Folded("group:reviewers") {
		t.Errorf("enter on a header: msg %v, closed %v", msg, m.Folded("group:reviewers"))
	}
	enter("group:reviewers")
	if m.Folded("group:reviewers") {
		t.Error("enter again did not open the group")
	}
	if p, ok := m.Selected(); !ok || p.Login != "ann" {
		t.Errorf("selected = %+v, %v; want the author", p, ok)
	}
}

func TestCommentersFollowTheDefaultUntilFolded(t *testing.T) {
	t.Parallel()
	m := New(foldtree.WithSize(80, 30))
	o := Options{Styles: styles(t, true)}
	m.SetData(Data{Author: user("ann", "Ann")}, o)
	m.SetData(everything(), o)
	if !m.Folded(idCommenters) {
		t.Error("four commenters did not close the group that nobody touched")
	}
	m.tree.SetFolded(idCommenters, false)
	m.SetData(everything(), o)
	if m.Folded(idCommenters) {
		t.Error("a group the user opened was closed again")
	}
}

func TestHasComments(t *testing.T) {
	t.Parallel()
	d := everything()
	for login, want := range map[string]bool{"zed": true, "xia": true, "dina": true, "rita": false, "": false} {
		if got := HasComments(d, login); got != want {
			t.Errorf("HasComments(%q) = %v, want %v", login, got, want)
		}
	}
}

func BenchmarkRows(b *testing.B) {
	d, o := everything(), Options{Styles: styles(b, true)}
	b.ReportAllocs()
	for b.Loop() {
		Rows(d, o)
	}
}
