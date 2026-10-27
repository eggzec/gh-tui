package tui

import (
	"reflect"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// topType is the type of the open modal, for a test that opens real modals.
func topType(m *Model) reflect.Type {
	if m.modals.Top() == nil {
		return nil
	}
	return reflect.TypeOf(m.modals.Top())
}

// TestNotificationsOverModalsReturn checks, with the real modals, that the
// notifications open over a pull request and over its merge dialog, and that
// esc returns to the one under them.
func TestNotificationsOverModalsReturn(t *testing.T) {
	t.Parallel()
	// The pull request modal takes the key too, so for both the open is made
	// as the key's command makes it.
	for _, tt := range []struct {
		name  string
		steps []string
		under reflect.Type
	}{
		{"pull request", []string{"global.pane_2", "global.select"}, reflect.TypeFor[*pulls.Panes]()},
		{"merge dialog", []string{"global.pane_2", "global.select", "pull_modal.merge"}, reflect.TypeFor[*pulls.MergeDialog]()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, tt.steps...)
				under := m.modals.Top()
				if topType(m) != tt.under {
					t.Fatalf("the steps opened %v, want %v", topType(m), tt.under)
				}
				// The merge dialog types the key as text, so the open comes
				// from the command the key runs, as the dashboard's does.
				driveKeys(t, m, m.openNotifications())
				if got := topType(m); got != reflect.TypeFor[*notifications.Modal]() {
					t.Fatalf("the open left %v over %s, want the notifications", got, tt.name)
				}
				tap(t, m, "esc")
				if m.modals.Top() != under {
					t.Errorf("esc left %v, want the %s again", topType(m), tt.name)
				}
			})
		})
	}
}

// TestOpenRowsOverTheFilter checks, with the real filter form on top, that a
// higher modal (the token prompt) opens over it and esc returns, and that
// lower ones (a pager, a goto to an issue, a file preview) are refused with
// a toast that names it, leaving it open.
func TestOpenRowsOverTheFilter(t *testing.T) {
	t.Parallel()
	steps := []string{"global.pane_2", "pulls.filter"}
	t.Run("token prompt opens over it", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, true, steps...)
			filter := m.modals.Top()
			driveKeys(t, m, m.authCommand(""))
			if m.modals.Top() == filter || m.modals.Top() == nil {
				t.Fatalf("the token prompt didn't open over the filter: %v", topType(m))
			}
			tap(t, m, "esc")
			if m.modals.Top() != filter {
				t.Errorf("esc left %v, want the filter", topType(m))
			}
		})
	})
	for _, tt := range []struct {
		name string
		open func(m *Model) tea.Cmd
	}{
		{"pager", func(m *Model) tea.Cmd { return m.openText("Notes", "notes.txt", "hello", false) }},
		{"goto issue", func(*Model) tea.Cmd { return func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 2} } }},
		{"file preview", func(*Model) tea.Cmd {
			return func() tea.Msg { return ui.OpenFileMsg{Repo: testRepo, Path: "README.md", SHA: "b1"} }
		}},
	} {
		t.Run(tt.name+" is refused", func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, steps...)
				filter := m.modals.Top()
				m.toast.Clear()
				driveKeys(t, m, tt.open(m))
				if m.modals.Top() != filter {
					t.Errorf("the open left %v, want the filter", topType(m))
				}
				if want := "Close the filter first."; !hasToast(m, want) {
					t.Errorf("toasts %s, want %q", toasted(m), want)
				}
			})
		})
	}
}

// TestGotoNumberOverAModal checks that opening an issue by its number, as
// :goto does, replaces the pull request modal on top, and that backspace
// returns to it.
func TestGotoNumberOverAModal(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "global.select")
		pull := m.modals.Top()
		driveKeys(t, m, func() tea.Msg { return ui.OpenIssueMsg{Repo: testRepo, Number: 2} })
		if m.modals.Top() == pull || m.modals.Top() == nil {
			t.Fatalf("the issue didn't open: %v", topType(m))
		}
		tap(t, m, "backspace")
		if m.modals.Top() != pull {
			t.Errorf("backspace left %v, want the pull request", topType(m))
		}
	})
}

// TestConfirmOverModalsReturn checks that a confirm dialog opens over the
// pull request modal and the merge dialog, and that esc returns to the one
// under it.
func TestConfirmOverModalsReturn(t *testing.T) {
	t.Parallel()
	for _, steps := range [][]string{
		{"global.pane_2", "global.select"},
		{"global.pane_2", "global.select", "pull_modal.merge"},
	} {
		synctest.Test(t, func(t *testing.T) {
			m := keysAfter(t, true, steps...)
			under := m.modals.Top()
			ask := ui.NewConfirmModal(ui.Confirm{Question: "Sure?"}, ui.NewConfirmKeys(m.cfg.Keys), ui.Icons{})
			driveKeys(t, m, ui.OpenModal(ask))
			if m.modals.Top() != ask {
				t.Fatalf("the confirm didn't open over %v: %v", topType(m), steps)
			}
			tap(t, m, "esc")
			if m.modals.Top() != under {
				t.Errorf("esc left %v, want the one under the confirm", topType(m))
			}
		})
	}
}
