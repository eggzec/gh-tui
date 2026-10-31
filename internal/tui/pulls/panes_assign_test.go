package pulls

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/facets"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// EditAssignees shows the people in add as assigned and takes those whose
// logins are in remove off the pull request at once.
func (f *fakeService) EditAssignees(_ core.RepoRef, number int, add []core.User, remove []string) *optimistic.Op {
	var what strings.Builder
	what.WriteString("assignees")
	for _, u := range add {
		fmt.Fprintf(&what, " +%s", u.Login)
	}
	for _, l := range remove {
		fmt.Fprintf(&what, " -%s", l)
	}
	return f.change(what.String(), number, func(pr *core.PullRequest) {
		kept := make([]core.User, 0, len(pr.Assignees)+len(add))
		for _, u := range pr.Assignees {
			if !slices.Contains(remove, u.Login) {
				kept = append(kept, u)
			}
		}
		pr.Assignees = slices.Concat(kept, add)
	})
}

// assigning opens the picker of the assignees from the row of bob, who is
// assigned, and returns the modal.
func assigning(t *testing.T, opts ...Option) (*involvedService, *panesModal) {
	t.Helper()
	svc := &involvedService{fakeService: newFakeService()}
	m := openedInvolved(t, svc, opts...)
	m.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps})
	if !moveTo(m, "bob") {
		t.Fatal("no row for bob")
	}
	settle(m, m.Update(keyMsg("e")))
	return svc, m
}

func assigneesOverlayOf(t *testing.T, m *panesModal) *assigneesOverlay {
	t.Helper()
	o, ok := m.top().(*assigneesOverlay)
	if !ok {
		t.Fatalf("the top overlay is %T, want the assignees picker", m.top())
	}
	return o
}

func TestAssigneesTickDiff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		on       []string
		wantSent string
	}{
		{"remove", nil, "assignees -bob"},
		{"no change", []string{"bob"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, m := assigning(t)
			o := assigneesOverlayOf(t, m)
			o.on = map[string]bool{}
			for _, l := range tt.on {
				o.on[l] = true
			}
			cmd := o.change(o.on, o.was)
			run(m, cmd)
			got := svc.changes()
			if tt.wantSent == "" {
				if len(got) != 0 {
					t.Fatalf("sent %q, want nothing", got)
				}
				return
			}
			if len(got) != 1 || !strings.HasPrefix(got[0], tt.wantSent) {
				t.Errorf("sent %q, want %q", got, tt.wantSent)
			}
		})
	}
}

func TestAssigneesEnterApplies(t *testing.T) {
	t.Parallel()
	svc, m := assigning(t)
	assigneesOverlayOf(t, m)
	press2(m, "space")
	run(m, m.Update(keyMsg("enter")))
	if m.top() != nil {
		t.Error("enter left the picker open")
	}
	if got := svc.changes(); len(got) != 1 || !strings.HasPrefix(got[0], "assignees -bob") {
		t.Errorf("sent %q, want one change that removes bob", got)
	}
}

// stuckFacets is a fake whose People never answers until release is closed.
type stuckFacets struct {
	fakeFacets
	release chan struct{}
}

func (f stuckFacets) People(ctx context.Context, _ facets.PeopleQuery) ([]core.User, error) {
	select {
	case <-f.release:
	case <-ctx.Done():
	}
	return nil, errors.New("no answer")
}

// failingFacets is a fake whose People fails.
type failingFacets struct{ fakeFacets }

func (failingFacets) People(context.Context, facets.PeopleQuery) ([]core.User, error) {
	return nil, errors.New("offline")
}

// Enter before the first search answers, or after it failed, keeps the
// assignees, as the picker lists none of them yet.
func TestAssigneesEnterWithoutAnswerSendsNothing(t *testing.T) {
	t.Parallel()
	stuck := stuckFacets{release: make(chan struct{})}
	t.Cleanup(func() { close(stuck.release) })
	for name, f := range map[string]Facets{"pending": stuck, "failed": failingFacets{}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			svc := &involvedService{fakeService: newFakeService()}
			m := openedInvolved(t, svc, WithFacets(f))
			m.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps})
			if !moveTo(m, "bob") {
				t.Fatal("no row for bob")
			}
			cmd := m.Update(keyMsg("e"))
			if name == "failed" {
				settle(m, cmd)
			}
			assigneesOverlayOf(t, m)
			run(m, m.Update(keyMsg("enter")))
			if got := svc.changes(); len(got) != 0 {
				t.Errorf("sent %q, want nothing", got)
			}
		})
	}
}

// settle runs cmd, and the commands of the batches in it, feeding their
// messages back to the modal until none is left.
func settle(m *panesModal, cmd tea.Cmd) {
	var queue []tea.Cmd
	queue = append(queue, cmd)
	for n := 0; len(queue) > 0 && n < 50; n++ {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		msg := c()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		queue = append(queue, m.Update(msg))
	}
}

// tick moves to the row of login and ticks or unticks it.
func tick(t *testing.T, m *panesModal, login string) {
	t.Helper()
	o := assigneesOverlayOf(t, m)
	if !o.p.Select(login) {
		t.Fatalf("no row for %s", login)
	}
	run(m, m.Update(keyMsg("space")))
}

func TestAssigneesWithFacets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		do   func(t *testing.T, m *panesModal)
		want string
	}{
		{"add", func(t *testing.T, m *panesModal) {
			t.Helper()
			tick(t, m, "hubot")
		}, "assignees +hubot"},
		{"swap", func(t *testing.T, m *panesModal) {
			t.Helper()
			tick(t, m, "hubot")
			tick(t, m, "bob")
		}, "assignees +hubot -bob"},
		{"re-query", func(t *testing.T, m *panesModal) {
			t.Helper()
			settle(m, m.Update(keyMsg("/")))
			for _, r := range "oct" {
				settle(m, m.Update(keyMsg(string(r))))
			}
			// Enter keeps the query and returns to the list; esc then clears
			// the query, which searches again, and the ticks stay.
			settle(m, m.Update(keyMsg("enter")))
			tick(t, m, "octocat")
			settle(m, m.Update(keyMsg("esc")))
			if m.top() == nil {
				t.Fatal("esc closed the picker instead of clearing the query")
			}
		}, "assignees +octocat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, m := assigning(t, WithFacets(fakeFacets{}))
			tt.do(t, m)
			settle(m, m.Update(keyMsg("enter")))
			if m.top() != nil {
				t.Error("enter left the picker open")
			}
			got := svc.changes()
			if len(got) != 1 || !strings.HasPrefix(got[0], tt.want) {
				t.Errorf("sent %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAssigneesEscAndBackDoNotApply(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"esc", "backspace"} {
		t.Run(k, func(t *testing.T) {
			t.Parallel()
			svc, m := assigning(t)
			assigneesOverlayOf(t, m)
			press2(m, "space")
			run(m, m.Update(keyMsg(k)))
			if m.top() != nil {
				t.Errorf("%s left the picker open", k)
			}
			if got := svc.changes(); len(got) != 0 {
				t.Errorf("%s sent %q", k, got)
			}
		})
	}
}

func TestAssigneesKeyOnlyOnAssignees(t *testing.T) {
	t.Parallel()
	svc := &involvedService{fakeService: newFakeService()}
	m := openedInvolved(t, svc)
	m.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps})
	if !moveTo(m, "rita") {
		t.Fatal("no row for rita")
	}
	run(m, m.Update(keyMsg("e")))
	if m.top() != nil {
		t.Errorf("e on a reviewer opened %T", m.top())
	}
	if !moveTo(m, "bob") {
		t.Fatal("no row for bob")
	}
	run(m, m.Update(keyMsg("e")))
	assigneesOverlayOf(t, m)
}

func TestAssigneesRefusedWithoutTriage(t *testing.T) {
	t.Parallel()
	svc := &involvedService{fakeService: newFakeService()}
	m := openedInvolved(t, svc)
	m.Update(ui.CapsMsg{Repo: repo, Caps: readCaps})
	if !moveTo(m, "bob") {
		t.Fatal("no row for bob")
	}
	toasts := run(m, m.Update(keyMsg("e")))
	if m.top() != nil {
		t.Errorf("e opened %T for a viewer who may not edit", m.top())
	}
	if len(toasts) == 0 {
		t.Error("the refusal said nothing")
	}
}

func TestAssigneesView(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()
			_, m := assigning(t)
			m.SetSize(size[0], size[1])
			golden.RequireEqual(t, m.View())
		})
	}
}

// press2 sends a key to the modal.
func press2(m *panesModal, k string) { run(m, m.Update(keyMsg(k))) }
