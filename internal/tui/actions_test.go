package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// tabbedModal is a fake modal with tabs.
type tabbedModal struct {
	fakeModal
	names  []string
	active int
}

func (t *tabbedModal) Tabs() (names []string, active int) { return t.names, t.active }

// actionsOpener opens a fake modal and records the repositories it was
// opened on.
type actionsOpener struct {
	modal   *tabbedModal
	opened  []core.RepoRef
	filters []core.RunFilter
	targets []ui.ActionsAt
	loads   int
}

func (a *actionsOpener) open(_ context.Context, repo core.RepoRef, f core.RunFilter, at ui.ActionsAt) (ui.Modal, tea.Cmd) {
	a.opened = append(a.opened, repo)
	a.filters = append(a.filters, f)
	a.targets = append(a.targets, at)
	a.modal = &tabbedModal{title: "Actions · " + repo.String(), names: []string{"All", "Failing", "Running", "Mine"}, active: 1}
	return a.modal, func() tea.Msg { a.loads++; return nil }
}

func TestActionsKeyOpensTheActions(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	run(m, m.Init())
	if s := onScreen(m); !strings.Contains(s, "A actions") {
		t.Errorf("help lacks the actions key:\n%s", s)
	}
	run(m, m.key(press("A")))
	if m.modals.Top() != a.modal || a.loads != 1 || len(a.opened) != 1 || a.opened[0] != testRepo {
		t.Fatalf("A opened %v on %v and loaded %d times, want the actions of %s loaded once", m.modals.Top(), a.opened, a.loads, testRepo)
	}
	if a.modal.width == 0 || !a.modal.themed {
		t.Error("the modal wasn't sized and themed before it was drawn")
	}
}

func TestActionsKeyNeedsARepoAndItsScreen(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newApp(t, core.RepoRef{}, WithActions(a.open))
	run(m, m.key(press("A")))
	if s := onScreen(m); strings.Contains(s, "A actions") {
		t.Errorf("help offers the actions without a repository:\n%s", s)
	}
	m, _ = newDashApp(t, testRepo, WithActions(a.open))
	run(m, m.showScreen(dashScreen, 0))
	if m.screen != dashScreen {
		t.Fatalf("on screen %d, want the dashboard", m.screen)
	}
	run(m, m.key(press("A")))
	if len(a.opened) != 0 || m.modals.Top() != nil {
		t.Errorf("A opened the actions %d times off the repository screen", len(a.opened))
	}
	m, _ = newTestApp(t)
	run(m, m.key(press("A")))
	if m.modals.Top() != nil {
		t.Error("A opened a modal without actions")
	}
}

// overlayingModal opens a fake overlay over itself for each key it gets, as
// the actions modal does for the filter, sort and group.
type overlayingModal struct {
	fakeModal
}

func (o *overlayingModal) Update(msg tea.Msg) tea.Cmd {
	o.fakeModal.Update(msg)
	if _, ok := msg.(tea.KeyPressMsg); ok {
		ov := &fakeModal{title: "Overlay"}
		return func() tea.Msg { return ui.OpenModalMsg{Modal: ov, Over: o} }
	}
	return nil
}

func TestOverlaysOverTheActionsReturnToIt(t *testing.T) {
	t.Parallel()
	host := &overlayingModal{}
	host.title = "Actions"
	m, _ := newTestApp(t)
	run(m, ui.OpenModal(host))
	for _, key := range []string{"f", "s", "g"} {
		run(m, m.key(press(key)))
		if m.modals.Top() == ui.Modal(host) {
			t.Fatalf("%s opened nothing over the actions", key)
		}
		run(m, ui.LeaveModal(m.modals.Top()))
		if m.modals.Top() != ui.Modal(host) {
			t.Fatalf("closing the overlay of %s left %v, want the actions", key, m.modals.Top())
		}
	}
}

func TestFrameShowsTheTabs(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	run(m, m.key(press("A")))
	top := topEdge(m)
	if !strings.Contains(top, "Actions · eggzec/gh-tui") || !strings.HasSuffix(top, "All · Failing · Running · Mine ─╮") {
		t.Errorf("the top edge is %q, want the title and the tabs", top)
	}
	if w, fw := ansi.StringWidth(top), 120-2*12; w != fw {
		t.Errorf("the top edge is %d wide, want the frame's %d", w, fw)
	}
	// The tabs give way to the title when both don't fit.
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 30})
	if top := topEdge(m); strings.Contains(top, "Mine") || !strings.Contains(top, "Actions") {
		t.Errorf("at 50 columns the top edge is %q, want the title alone", top)
	}
}

// A long title gives way to the tabs at 80 columns, down to a stretch of
// it, so the tabs still show.
func TestFrameShortensALongTitle(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(ui.OpenActionsMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea-app-template"}})
	top := topEdge(m)
	if !strings.Contains(top, "Actions · charmbrace") || !strings.Contains(top, "…") || !strings.HasSuffix(top, "All · Failing · Running · Mine ─╮") {
		t.Errorf("the top edge is %q, want the title shortened and the tabs", top)
	}
	golden.RequireEqual(t, m.View().Content)
}

// topEdge is the top edge of the modal's frame, without styles.
func topEdge(m *Model) string {
	for l := range strings.SplitSeq(ansi.Strip(m.View().Content), "\n") {
		if i := strings.Index(l, "╭─ Actions"); i >= 0 {
			rest := l[i:]
			if j := strings.Index(rest, "╮"); j >= 0 {
				return rest[:j+len("╮")]
			}
		}
	}
	return ""
}

func TestOpenActionsMsgOpensTheRunsOfAnyRepo(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	run(m, m.key(press("I")))
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	f := core.RunFilter{Branch: "feat/x", Status: "failure"}
	m.Update(ui.OpenActionsMsg{Repo: other, Filter: f})
	if m.modals.Top() != a.modal || len(a.opened) != 1 || a.opened[0] != other || a.filters[0] != f {
		t.Errorf("opened %v with %v, want the failed runs of the branch of %s", a.opened, a.filters, other)
	}
	if a.targets[0] != (ui.ActionsAt{}) {
		t.Errorf("opened at %+v, want the first run", a.targets[0])
	}
}

func TestOpenActionsMsgOpensOnARunAndAJob(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	run(m, m.key(press("I")))
	f := core.RunFilter{HeadSHA: "f00dcafe"}
	at := ui.ActionsAt{Run: 4812, Job: 101}
	m.Update(ui.OpenActionsMsg{Repo: testRepo, Filter: f, At: at})
	if m.modals.Top() != a.modal || len(a.opened) != 1 || a.filters[0] != f || a.targets[0] != at {
		t.Errorf("opened %v with %v at %v, want the runs of the commit opened at %+v", a.opened, a.filters, a.targets, at)
	}
}

// leavingModal is a fake of the Actions modal's keys: esc leaves it, the back
// key is left to the app, as on the runs, and the quit key closes it.
type leavingModal struct {
	backModal
}

func (l *leavingModal) Act(action string) (tea.Cmd, bool) {
	switch action {
	case ui.ActQuit:
		return ui.CloseModal(l), true
	case ui.ActDismiss:
		return ui.LeaveModal(l), true
	}
	return nil, false
}

// openedOver opens an Actions modal that is a leavingModal over a modal of
// the app, with back as the modal to return to, which may be nil, and returns
// both.
func openedOver(t *testing.T, withBack bool) (m *Model, under, over *leavingModal) {
	t.Helper()
	over = &leavingModal{}
	over.title = "Actions"
	open := func(context.Context, core.RepoRef, core.RunFilter, ui.ActionsAt) (ui.Modal, tea.Cmd) {
		return over, nil
	}
	m, _ = newTestApp(t, WithActions(open))
	under = &leavingModal{}
	under.title = "Pull request"
	msg := ui.OpenActionsMsg{Repo: testRepo}
	if withBack {
		run(m, ui.OpenModalOver(under, nil))
		msg.Over = under
	}
	run(m, func() tea.Msg { return msg })
	if m.modals.Top() != over {
		t.Fatalf("the Actions modal didn't open: %v", m.modals.Top())
	}
	return m, under, over
}

func reopened(mod *leavingModal) bool {
	return slices.ContainsFunc(mod.msgs, func(msg tea.Msg) bool { _, ok := msg.(ui.ReopenedMsg); return ok })
}

func TestActionsOverAModalReturnsToIt(t *testing.T) {
	t.Parallel()
	for name, key := range map[string]tea.KeyPressMsg{"esc": {Code: tea.KeyEscape}, "backspace": backspace} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, under, over := openedOver(t, true)
			run(m, m.key(key))
			if m.modals.Top() != under || over.discarded != 1 || !reopened(under) {
				t.Errorf("%s left the Actions modal for %v, discarded %d, reopened %v; want the modal under, told it is open again", name, m.modals.Top(), over.discarded, reopened(under))
			}
		})
	}
}

func TestQuitClosesTheActionsAndWhatIsUnderIt(t *testing.T) {
	t.Parallel()
	m, _, _ := openedOver(t, true)
	run(m, m.key(press("q")))
	if m.modals.Top() != nil {
		t.Errorf("q left %v open", m.modals.Top())
	}
}

func TestActionsOverNothingKeepsItsKeys(t *testing.T) {
	t.Parallel()
	m, _, over := openedOver(t, false)
	run(m, m.key(backspace))
	if m.modals.Top() != over {
		t.Fatalf("backspace left the Actions modal for %v with nothing under it", m.modals.Top())
	}
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyEscape}))
	if m.modals.Top() != nil {
		t.Errorf("esc left %v open, want the modal closed", m.modals.Top())
	}
}

func TestOpenReleaseMsgOpensTheRelease(t *testing.T) {
	t.Parallel()
	type opened struct {
		repo core.RepoRef
		id   int64
		url  string
	}
	var got []opened
	mod := &fakeModal{title: "v3.0.0 · charmbracelet/glow"}
	loads := 0
	open := func(_ context.Context, repo core.RepoRef, id int64, url string) (ui.Modal, tea.Cmd) {
		got = append(got, opened{repo, id, url})
		return mod, func() tea.Msg { loads++; return nil }
	}
	msg := ui.OpenReleaseMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "glow"}, ID: 368759772, URL: "https://github.com/charmbracelet/glow/releases"}

	m, _ := newTestApp(t)
	m.Update(msg)
	if m.modals.Top() != nil {
		t.Error("a release opened without a release modal")
	}

	m, _ = newTestApp(t, WithRelease(open))
	run(m, m.key(press("I")))
	_, cmd := m.Update(msg)
	run(m, cmd)
	if m.modals.Top() != mod || loads != 1 || len(got) != 1 || got[0] != (opened{msg.Repo, msg.ID, msg.URL}) {
		t.Errorf("opened %v and loaded %d times, want the release of the message loaded once", got, loads)
	}
	if mod.width == 0 || !mod.themed {
		t.Error("the modal wasn't sized and themed before it was drawn")
	}
}
