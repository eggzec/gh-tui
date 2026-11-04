package issues

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestDepRows(t *testing.T) {
	t.Parallel()
	other := core.RepoRef{Owner: "acme", Name: "lib"}
	home := core.RepoRef{Owner: "acme", Name: "app"}
	root := depNode{kind: depBlockedBy, trail: []depRef{{home, 999}}}
	rows := depRows(home, "blocked_by", root, []core.Dependency{
		{Number: 5, Title: "Plain"},
		{Number: 999, Title: "Itself"},
		{Number: 7, Title: "Elsewhere", Repo: other, IsPullRequest: true},
	})
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if r := rows[0]; r.Right != "" || !r.Branch || r.ID != "blocked_by>#5" || r.Name != "#5 Plain" {
		t.Errorf("plain row = %+v", r)
	}
	if r := rows[1]; r.Right != "cycle" || r.Branch {
		t.Errorf("cycle row = %+v", r)
	}
	v, _ := rows[2].Value.(depNode)
	if v.dep.Repo != other || len(v.trail) != 2 {
		t.Errorf("cross-repo row keeps %+v", v)
	}
}

// A blocker of another repository with the number of the issue, or of its
// sibling, is another issue: no cycle, and no clash of the row IDs.
func TestDepRowsAcrossRepos(t *testing.T) {
	t.Parallel()
	home := core.RepoRef{Owner: "o", Name: "a"}
	other := core.RepoRef{Owner: "o", Name: "b"}
	root := depNode{kind: depBlockedBy, trail: []depRef{{home, 5}}}
	rows := depRows(home, "blocked_by", root, []core.Dependency{
		{Number: 5, Title: "Same number elsewhere", Repo: other},
		{Number: 7, Title: "Here", Repo: home},
		{Number: 7, Title: "There", Repo: other},
		{Number: 5, Title: "Itself", Repo: home},
	})
	if r := rows[0]; r.Right != "" || !r.Branch {
		t.Errorf("o/b#5 under o/a#5 = %+v, want a plain row", r)
	}
	if rows[1].ID == rows[2].ID {
		t.Errorf("o/a#7 and o/b#7 share the ID %q", rows[1].ID)
	}
	if rows[2].Name != "o/b#7 There" || rows[1].Name != "#7 Here" {
		t.Errorf("names = %q, %q", rows[1].Name, rows[2].Name)
	}
	if rows[3].Right != "cycle" {
		t.Errorf("the issue itself = %+v, want a cycle", rows[3])
	}
}

func TestDepRowsDepthCap(t *testing.T) {
	t.Parallel()
	home := core.RepoRef{Owner: "acme", Name: "app"}
	trail := make([]depRef, maxDepDepth)
	for i := range trail {
		trail[i] = depRef{home, i + 1}
	}
	rows := depRows(home, "x", depNode{kind: depBlocking, trail: trail}, []core.Dependency{{Number: 100, Title: "Deep"}})
	if rows[0].Right != "deeper" || rows[0].Branch {
		t.Errorf("row past the depth cap = %+v, want a closed row", rows[0])
	}
	rows = depRows(home, "x", depNode{kind: depBlocking, trail: trail[:maxDepDepth-1]}, []core.Dependency{{Number: 100, Title: "Last"}})
	if rows[0].Right != "" || !rows[0].Branch {
		t.Errorf("row at the depth cap = %+v, want an open row", rows[0])
	}
}

func blockedSvc() *fakeService {
	svc := newFakeService(sampleIssues(12))
	svc.deps = func(_ context.Context, _ core.RepoRef, n int, kind string) ([]core.Dependency, error) {
		if n == 999 && kind == depBlockedBy {
			return []core.Dependency{{Number: 12, Title: "Fix the thing", State: core.StateOpen}}, nil
		}
		if n == 999 {
			return nil, core.ErrNotFound
		}
		return nil, nil
	}
	return svc
}

func TestBlockersPane(t *testing.T) {
	t.Parallel()
	for _, w := range []int{80, 120} {
		h := started(t, blockedSvc(), w, 30)
		press(t, h, "down", "enter")
		m := h.modal()
		if m == nil {
			t.Fatal("enter didn't open the issue")
		}
		press(t, h, "*", "5")
		v := ansi.Strip(m.View())
		assertFits(t, m.View(), w, 30)
		for _, want := range []string{"Blocked by", "Blocking"} {
			if !strings.Contains(v, want) {
				t.Errorf("at %d the pane lacks %q:\n%s", w, want, v)
			}
		}
		press(t, h, "l")
		if v := ansi.Strip(m.View()); !strings.Contains(v, "#12 Fix the thing") {
			t.Errorf("at %d the blocker isn't listed:\n%s", w, v)
		}
	}
}

func TestBlockerEnterOpensIssue(t *testing.T) {
	t.Parallel()
	h := started(t, blockedSvc(), 120, 30)
	press(t, h, "down", "enter")
	m := h.modal()
	press(t, h, "*", "5")
	m.View() // sizes the tree, which takes no row moves before it has a size
	press(t, h, "l", "down")
	cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter on a blocker did nothing")
	}
	// The tree answers with its own open message, which the modal turns
	// into the request to open the issue.
	cmd = m.Update(cmd())
	if cmd == nil {
		t.Fatal("the tree's open message did nothing")
	}
	open, ok := cmd().(ui.OpenIssueMsg)
	if !ok || open.Number != 12 || open.Over == nil {
		t.Fatalf("enter gave %#v, want OpenIssueMsg for #12 over the modal", open)
	}
}
