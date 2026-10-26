package tui

import (
	"slices"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/modals"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// TestQuitCommandStaysOpenWhenAGuardKeeps checks that :q asks the guards and
// leaves the modal open, and does not quit, while one of them keeps it.
func TestQuitCommandStaysOpenWhenAGuardKeeps(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	mod := &savingModal{}
	mod.title = "Comment"
	prio, ok := m.priorityOf(mod)
	if !ok {
		t.Fatal("the saving modal has no rank")
	}
	asked := 0
	m.apply(m.modals.Add(mod, prio, modals.WithGuard(func(modals.Reason) modals.Closing {
		asked++
		return modals.Refuse("Finish the comment first.")
	})))
	if m.modals.Top() != ui.Modal(mod) {
		t.Fatalf("the modal didn't open: %v", m.modals.Top())
	}
	cmd := m.runLine("q", nil)
	if asked == 0 {
		t.Error("the guard was not asked")
	}
	if quits(cmd) {
		t.Error(":q quit over a guard that keeps its modal")
	}
	if m.modals.Top() != ui.Modal(mod) {
		t.Errorf("the modal was closed: top is %v", m.modals.Top())
	}
}

// TestCtrlCSavesACoveredSaver checks that ctrl+c saves a modal that another
// covers, not only the one on top, and quits.
func TestCtrlCSavesACoveredSaver(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	held := &savingModal{}
	held.title = "Comment"
	f := &shapeModal{}
	f.title = "filter"
	run(m, ui.OpenModalOver(held, nil))
	run(m, ui.OpenModalOver(f, held))
	if m.modals.Top() != ui.Modal(f) {
		t.Fatalf("top is %v, want the filter over the comment", m.modals.Top())
	}
	cmd := m.key(ctrlC)
	if held.saved != 1 {
		t.Errorf("the covered modal was saved %d times, want 1", held.saved)
	}
	if !quits(cmd) {
		t.Error("ctrl+c didn't quit")
	}
}

// TestSearchPullIssueReturn checks, with the real modals, that esc on an issue
// opened from a pull request that the search opened returns to the search,
// and backspace to the pull request.
func TestSearchPullIssueReturn(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		key  string
		want func(search, pull ui.Modal) ui.Modal
	}{
		{"esc", func(search, _ ui.Modal) ui.Modal { return search }},
		{"backspace", func(_, pull ui.Modal) ui.Modal { return pull }},
	} {
		t.Run(tt.key, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := newKeysApp(t, true)
				driveKeys(t, m, m.key(press("S")))
				search := m.modals.Top()
				if search == nil {
					t.Fatal("S opened nothing")
				}
				driveKeys(t, m, func() tea.Msg { return ui.OpenPullMsg{Repo: testRepo, Number: 1, Over: search} })
				pull := m.modals.Top()
				if pull == nil || pull == search {
					t.Fatalf("the pull request didn't open over the search: %v", pull)
				}
				driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 2, Over: pull} })
				issue := m.modals.Top()
				if issue == nil || issue == pull || issue == search {
					t.Fatalf("the issue didn't open in place of the pull request: %v", issue)
				}
				driveKeys(t, m, m.key(press(tt.key)))
				if want := tt.want(search, pull); m.modals.Top() != want {
					t.Errorf("%s left %v, want %v", tt.key, m.modals.Top(), want)
				}
			})
		})
	}
}

// TestFilterEscTellsTheModalUnderItReopened checks that closing a modal
// that opened over another tells the one under it that it is open again, so
// it can resume.
func TestFilterEscTellsTheModalUnderItReopened(t *testing.T) {
	t.Parallel()
	m, _ := newTestApp(t)
	host := &backModal{}
	host.title = "Actions"
	f := &shapeModal{}
	f.title = "filter"
	run(m, ui.OpenModalOver(host, nil))
	run(m, ui.OpenModalOver(f, host))
	run(m, m.key(press("esc")))
	if m.modals.Top() != ui.Modal(host) {
		t.Fatalf("esc left %v, want the host", m.modals.Top())
	}
	if !slices.ContainsFunc(host.msgs, func(msg tea.Msg) bool {
		r, ok := msg.(ui.ReopenedMsg)
		return ok && r.Modal == ui.Modal(host)
	}) {
		t.Errorf("the host got %v, want a ReopenedMsg for itself", host.msgs)
	}
}
