package tui

import (
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// backLabel returns what the open modal's help names for the back key, or
// "" when no enabled binding holds it.
func backLabel(m *Model) string {
	if m.modal == nil {
		return ""
	}
	b, _, ok := uitest.Winner(m.modal.KeyLayers(), "backspace")
	if !ok {
		return ""
	}
	return b.Help().Desc
}

// closing are the modals that esc closes, each with the steps that leave
// it where it is the deepest.
var closing = []struct {
	name  string
	steps []string
}{
	{"history graph", []string{"repo.history"}},
	{"actions runs", []string{"repo.actions"}},
	{"actions jobs", []string{"repo.actions", "global.next_pane"}},
	{"actions log", []string{"repo.actions", "global.next_pane", "global.next_pane"}},
	{"preview", []string{"files.down", "global.select"}},
	{"config pager", []string{"global.command", typed("config"), "command_line.run"}},
	{"filter", []string{"global.pane_2", "pulls.filter"}},
	{"token prompt", []string{"global.command", typed("auth"), "command_line.run"}},
	{"finder", []string{"global.find_file"}},
}

// TestEscClosesTheModalFromAnyStep checks that esc closes the whole modal
// at once, from the deepest step of it, not a step at a time.
func TestEscClosesTheModalFromAnyStep(t *testing.T) {
	for _, tt := range closing {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, tt.steps...)
				if m.modal == nil {
					t.Fatal("the modal isn't open")
				}
				tap(t, m, "esc")
				if m.modal != nil {
					t.Error("esc left the modal open")
				}
			})
		})
	}
	t.Run("release", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := openRelease(t)
			tap(t, m, "esc")
			if m.modal != nil {
				t.Error("esc left the release open")
			}
		})
	})
}

func openRelease(t *testing.T) *Model {
	t.Helper()
	m := newKeysApp(t, true)
	driveKeys(t, m, func() tea.Msg {
		return ui.OpenReleaseMsg{Repo: testRepo, ID: keyRelease.ID, URL: keyRelease.URL}
	})
	if m.modal == nil {
		t.Fatal("the release isn't open")
	}
	return m
}

// TestBackspaceDoesNothingAtTheTopOfAModal checks that the back key leaves
// a modal that has no step before it as it is, and doesn't close it.
func TestBackspaceDoesNothingAtTheTopOfAModal(t *testing.T) {
	tops := []struct {
		name  string
		steps []string
	}{
		{"actions runs", []string{"repo.actions"}},
		{"preview", []string{"files.down", "global.select"}},
		{"config pager", []string{"global.command", typed("config"), "command_line.run"}},
		{"filter", []string{"global.pane_2", "pulls.filter"}},
		{"token prompt", []string{"global.command", typed("auth"), "command_line.run"}},
	}
	for _, tt := range tops {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, tt.steps...)
				open, before := m.modal, onScreen(m)
				tap(t, m, "backspace")
				if m.modal != open {
					t.Fatal("backspace closed the modal")
				}
				if after := onScreen(m); after != before {
					t.Errorf("backspace changed the modal:\n%s\nwas:\n%s", after, before)
				}
			})
		})
	}
	t.Run("release", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			m := openRelease(t)
			open, before := m.modal, onScreen(m)
			tap(t, m, "backspace")
			if m.modal != open || onScreen(m) != before {
				t.Error("backspace changed the release")
			}
		})
	})
}

// TestBackspaceStepsBackThroughActions checks the steps of the Actions
// modal: the log goes back to the jobs, the jobs to the runs, and the runs
// have nothing before them, and the help names the step the key goes to.
func TestBackspaceStepsBackThroughActions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "repo.actions", "global.next_pane", "global.next_pane")
		for _, want := range []string{"jobs", "runs", ""} {
			if got := backLabel(m); got != want {
				t.Fatalf("the help names the back key %q, want %q", got, want)
			}
			tap(t, m, "backspace")
			if m.modal == nil {
				t.Fatal("backspace closed the modal")
			}
		}
	})
}

// TestBackspaceStepsBackThroughHistory checks the steps of History: the
// graph goes back to the branches, which have nothing before them.
func TestBackspaceStepsBackThroughHistory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "repo.history")
		for _, want := range []string{"branches", ""} {
			if got := backLabel(m); got != want {
				t.Fatalf("the help names the back key %q, want %q", got, want)
			}
			tap(t, m, "backspace")
			if m.modal == nil {
				t.Fatal("backspace closed the modal")
			}
		}
	})
}

// TestEscClearsASearchBeforeClosing checks that esc takes away the search
// that a pager shows before it closes the modal.
func TestEscClearsASearchBeforeClosing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.command", typed("config"), "command_line.run")
		tap(t, m, "/")
		for _, k := range []string{"c", "o", "enter"} {
			tap(t, m, k)
		}
		tap(t, m, "esc")
		if m.modal == nil {
			t.Fatal("esc closed the pager with a search on view")
		}
		tap(t, m, "esc")
		if m.modal != nil {
			t.Error("esc left the pager open once the search was gone")
		}
	})
}

// TestBackspaceDeletesInATypingWidget checks that the keys that type keep
// backspace: in the search of a log, the query of the finder, and a
// pager's search, it deletes a character and neither steps back nor
// closes.
func TestBackspaceDeletesInATypingWidget(t *testing.T) {
	tests := []struct {
		name  string
		steps []string
		typed []string
	}{
		{"actions log search", []string{"repo.actions", "global.next_pane", "global.next_pane"}, []string{"/", "a", "b"}},
		{"finder", []string{"global.find_file"}, []string{"a", "b"}},
		{"pager search", []string{"global.command", typed("config"), "command_line.run"}, []string{"/", "a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, tt.steps...)
				for _, k := range tt.typed {
					tap(t, m, k)
				}
				open, before := m.modal, onScreen(m)
				tap(t, m, "backspace")
				if m.modal != open {
					t.Fatal("backspace closed or replaced the modal")
				}
				if onScreen(m) == before {
					t.Errorf("backspace deleted nothing:\n%s", before)
				}
				if !m.modalTakesKeys() {
					t.Error("backspace left the typing widget")
				}
			})
		})
	}
}
