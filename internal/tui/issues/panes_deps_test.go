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
	root := depNode{kind: depBlockedBy, trail: []int{999}}
	rows := depRows("blocked_by", root, []core.Dependency{
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

func TestDepRowsDepthCap(t *testing.T) {
	t.Parallel()
	trail := make([]int, maxDepDepth)
	for i := range trail {
		trail[i] = i + 1
	}
	rows := depRows("x", depNode{kind: depBlocking, trail: trail}, []core.Dependency{{Number: 100, Title: "Deep"}})
	if rows[0].Right != "..." || rows[0].Branch {
		t.Errorf("row past the depth cap = %+v, want a closed row", rows[0])
	}
	rows = depRows("x", depNode{kind: depBlocking, trail: trail[:maxDepDepth-1]}, []core.Dependency{{Number: 100, Title: "Last"}})
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
	press(t, h, "*", "5", "l", "down")
	cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("enter on a blocker did nothing")
	}
	open, ok := cmd().(ui.OpenIssueMsg)
	if !ok || open.Number != 12 || open.Over == nil {
		t.Fatalf("enter gave %#v, want OpenIssueMsg for #12 over the modal", open)
	}
}
