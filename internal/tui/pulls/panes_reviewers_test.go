package pulls

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// reviewing opens the reviewers picker from the row of rita, who is asked to
// review, and returns the modal. dina approved already.
func reviewing(t *testing.T, svc *involvedService, opts ...Option) *panesModal {
	t.Helper()
	m := openedInvolved(t, svc, opts...)
	m.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps})
	if !moveTo(m, "rita") {
		t.Fatal("no row for rita")
	}
	settle(m, m.Update(keyMsg("e")))
	reviewersOverlayOf(t, m)
	return m
}

func reviewersOverlayOf(t *testing.T, m *panesModal) *reviewersOverlay {
	t.Helper()
	o, ok := m.top().(*reviewersOverlay)
	if !ok {
		t.Fatalf("the top overlay is %T, want the reviewers picker", m.top())
	}
	return o
}

// tickRow moves to the row titled title and presses space, returning the
// toasts.
func tickRow(t *testing.T, m *panesModal, title string) []string {
	t.Helper()
	if !reviewersOverlayOf(t, m).p.Select(title) {
		t.Fatalf("no row for %s", title)
	}
	return run(m, m.Update(keyMsg("space")))
}

func names(rs [][]pulls.Reviewer) []string {
	var out []string
	for _, group := range rs {
		for _, r := range group {
			out = append(out, r.Login+r.Team)
		}
	}
	return out
}

func TestReviewersTickDiff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		do          func(t *testing.T, m *panesModal)
		wantAsked   []string
		wantRemoved []string
	}{
		{"no change", func(*testing.T, *panesModal) {}, nil, nil},
		{"remove pending", func(t *testing.T, m *panesModal) { t.Helper(); tickRow(t, m, userKey("rita")) }, nil, []string{"rita"}},
		{"request", func(t *testing.T, m *panesModal) { t.Helper(); tickRow(t, m, userKey("hubot")) }, []string{"hubot"}, nil},
		{"request team", func(t *testing.T, m *panesModal) { t.Helper(); tickRow(t, m, teamKey(repo.Owner+"/core")) }, []string{repo.Owner + "/core"}, nil},
		{"swap", func(t *testing.T, m *panesModal) {
			t.Helper()
			tickRow(t, m, userKey("hubot"))
			tickRow(t, m, userKey("rita"))
		}, []string{"hubot"}, []string{"rita"}},
		{"team and person swap", func(t *testing.T, m *panesModal) {
			t.Helper()
			tickRow(t, m, teamKey(repo.Owner+"/core"))
			tickRow(t, m, userKey("rita"))
		}, []string{repo.Owner + "/core"}, []string{"rita"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := &involvedService{fakeService: newFakeService(), teams: []core.Team{{Slug: "core", Name: "Core"}}}
			m := reviewing(t, svc, WithFacets(fakeFacets{}))
			settle(m, nil)
			tt.do(t, m)
			settle(m, m.Update(keyMsg("enter")))
			if m.top() != nil {
				t.Error("enter left the picker open")
			}
			if got := names(svc.asked); !slices.Equal(got, tt.wantAsked) {
				t.Errorf("asked %q, want %q", got, tt.wantAsked)
			}
			if got := names(svc.removed); !slices.Equal(got, tt.wantRemoved) {
				t.Errorf("removed %q, want %q", got, tt.wantRemoved)
			}
		})
	}
}

func TestReviewersLockedRowRefusesUntick(t *testing.T) {
	t.Parallel()
	svc := &involvedService{fakeService: newFakeService()}
	m := reviewing(t, svc, WithFacets(fakeFacets{}))
	settle(m, nil)
	toasts := tickRow(t, m, userKey("dina"))
	if !slices.Contains(toasts, notPendingText) {
		t.Errorf("toasts %q, want %q", toasts, notPendingText)
	}
	if !reviewersOverlayOf(t, m).on[userKey("dina")] {
		t.Error("the locked row was unticked")
	}
	settle(m, m.Update(keyMsg("enter")))
	if len(svc.removed) != 0 || len(svc.asked) != 0 {
		t.Errorf("sent asked %q removed %q, want nothing", names(svc.asked), names(svc.removed))
	}
}

func TestReviewersSentinels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		pick string
		want string
	}{
		{"already requested", pulls.ErrAlreadyRequested, userKey("hubot"), pulls.ErrAlreadyRequested.Error()},
		{"not pending", pulls.ErrNotPending, userKey("rita"), notPendingText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := &involvedService{fakeService: newFakeService(), refuse: tt.err}
			m := reviewing(t, svc, WithFacets(fakeFacets{}))
			settle(m, nil)
			tickRow(t, m, tt.pick)
			toasts := run(m, m.Update(keyMsg("enter")))
			if !slices.Contains(toasts, tt.want) {
				t.Errorf("toasts %q, want %q", toasts, tt.want)
			}
		})
	}
}

// Enter before the first search answers, or after it failed, keeps the
// reviewers.
func TestReviewersEnterWithoutAnswerSendsNothing(t *testing.T) {
	t.Parallel()
	stuck := stuckFacets{release: make(chan struct{})}
	t.Cleanup(func() { close(stuck.release) })
	for name, f := range map[string]Facets{"pending": stuck, "failed": failingFacets{}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			svc := &involvedService{fakeService: newFakeService()}
			m := openedInvolved(t, svc, WithFacets(f))
			m.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps})
			if !moveTo(m, "rita") {
				t.Fatal("no row for rita")
			}
			cmd := m.Update(keyMsg("e"))
			if name == "failed" {
				settle(m, cmd)
			}
			reviewersOverlayOf(t, m)
			run(m, m.Update(keyMsg("enter")))
			if len(svc.removed) != 0 || len(svc.asked) != 0 {
				t.Errorf("sent asked %q removed %q, want nothing", names(svc.asked), names(svc.removed))
			}
		})
	}
}

func TestReviewersRequeryKeepsTicks(t *testing.T) {
	t.Parallel()
	svc := &involvedService{fakeService: newFakeService()}
	m := reviewing(t, svc, WithFacets(fakeFacets{}))
	settle(m, nil)
	settle(m, m.Update(keyMsg("/")))
	for _, r := range "hub" {
		settle(m, m.Update(keyMsg(string(r))))
	}
	settle(m, m.Update(keyMsg("enter")))
	tickRow(t, m, userKey("hubot"))
	settle(m, m.Update(keyMsg("esc")))
	if m.top() == nil {
		t.Fatal("esc closed the picker instead of clearing the query")
	}
	if !reviewersOverlayOf(t, m).on[userKey("rita")] {
		t.Error("the pending reviewer rita lost her tick after the query")
	}
	settle(m, m.Update(keyMsg("enter")))
	if got := names(svc.asked); !slices.Equal(got, []string{"hubot"}) {
		t.Errorf("asked %q, want hubot", got)
	}
	if len(svc.removed) != 0 {
		t.Errorf("removed %q, want nothing", names(svc.removed))
	}
}

func TestReviewersTeamSearchFiltersByText(t *testing.T) {
	t.Parallel()
	svc := &involvedService{
		fakeService: newFakeService(),
		teams:       []core.Team{{Slug: "core", Name: "Core"}, {Slug: "docs", Name: "Docs"}},
		filterTeams: true,
	}
	m := reviewing(t, svc, WithFacets(fakeFacets{}))
	settle(m, nil)
	settle(m, m.Update(keyMsg("/")))
	for _, r := range "doc" {
		settle(m, m.Update(keyMsg(string(r))))
	}
	settle(m, m.Update(keyMsg("enter")))
	if reviewersOverlayOf(t, m).p.Select(teamKey(repo.Owner + "/core")) {
		t.Error("the team core is listed for the text doc")
	}
	tickRow(t, m, teamKey(repo.Owner+"/docs"))
	settle(m, m.Update(keyMsg("enter")))
	if got := names(svc.asked); !slices.Equal(got, []string{repo.Owner + "/docs"}) {
		t.Errorf("asked %q, want docs", got)
	}
}

func TestReviewersTeamSearchErrorKeepsPeople(t *testing.T) {
	t.Parallel()
	svc := &involvedService{
		fakeService: newFakeService(),
		teams:       []core.Team{{Slug: "core", Name: "Core"}},
		teamErr:     errors.New("hidden"),
	}
	m := reviewing(t, svc, WithFacets(fakeFacets{}))
	settle(m, nil)
	settle(m, m.Update(keyMsg("/")))
	for _, r := range "hub" {
		settle(m, m.Update(keyMsg(string(r))))
	}
	settle(m, m.Update(keyMsg("enter")))
	tickRow(t, m, userKey("hubot"))
	if !reviewersOverlayOf(t, m).p.Select(userKey("hubot")) {
		t.Error("the picker lost hubot when the team search failed")
	}
	settle(m, m.Update(keyMsg("enter")))
	if got := names(svc.asked); !slices.Equal(got, []string{"hubot"}) {
		t.Errorf("asked %q, want hubot", got)
	}
}

func TestReviewersKeyOnReviewersAndAssignees(t *testing.T) {
	t.Parallel()
	svc := &involvedService{fakeService: newFakeService()}
	m := openedInvolved(t, svc)
	m.Update(ui.CapsMsg{Repo: repo, Caps: writeCaps})
	if !moveTo(m, "rita") {
		t.Fatal("no row for rita")
	}
	settle(m, m.Update(keyMsg("e")))
	reviewersOverlayOf(t, m)
	settle(m, m.Update(keyMsg("esc")))
	if m.top() != nil {
		t.Fatal("esc left the picker open")
	}
	if !moveTo(m, "bob") {
		t.Fatal("no row for bob")
	}
	settle(m, m.Update(keyMsg("e")))
	assigneesOverlayOf(t, m)
}

func TestReviewersView(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()
			svc := &involvedService{fakeService: newFakeService(), teams: []core.Team{{Slug: "core", Name: "Core"}}}
			m := reviewing(t, svc, WithFacets(fakeFacets{}))
			settle(m, nil)
			m.SetSize(size[0], size[1])
			out := m.View()
			for _, want := range []string{"rita", "dina", "team"} {
				if !strings.Contains(out, want) {
					t.Errorf("the view lacks %q", want)
				}
			}
			golden.RequireEqual(t, out)
		})
	}
}
