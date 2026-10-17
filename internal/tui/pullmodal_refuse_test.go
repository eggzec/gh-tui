package tui

import (
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// openPanes opens the modal of the first pull request.
func openPanes(t *testing.T) *Model {
	t.Helper()
	m := newKeysApp(t, true)
	tapAction(t, m, "global.pane_2")
	tapAction(t, m, "global.select")
	if got := layerNames(m.keyLayers()); got != "global, pull_modal, pull_conversation" {
		t.Fatalf("the pull request opened with the keys %q", got)
	}
	return m
}

// A check row of the pull request modal opens the Actions modal over it, and
// leaving that returns to the pull request modal, on the pane and the zoom it
// had; q closes both.
func TestPullModalActionsReturnsToIt(t *testing.T) {
	t.Parallel()
	open := func(t *testing.T) (m *Model, modal ui.Modal, layers, view string) {
		t.Helper()
		m = openPanes(t)
		tapAction(t, m, "global.pane_3")
		tapAction(t, m, "global.zoom")
		modal, layers, view = m.modal, layerNames(m.keyLayers()), m.modal.View()
		driveKeys(t, m, func() tea.Msg { return checktree.OpenJobMsg{SHA: strings.Repeat("a", 40), RunID: 1, JobID: 2} })
		if m.modal == modal {
			t.Fatal("the job message left the pull request modal open, want the Actions modal")
		}
		if got := layerNames(m.keyLayers()); !strings.Contains(got, "actions") {
			t.Fatalf("the Actions modal opened with the keys %q", got)
		}
		return m, modal, layers, view
	}
	for name, key := range map[string]tea.KeyPressMsg{"esc": {Code: tea.KeyEscape}, "backspace": backspace} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				m, modal, layers, view := open(t)
				// The Actions modal opens on a job, so backspace first steps back
				// through its panes, and leaves it from the runs.
				for range 3 {
					driveKeys(t, m, m.key(key))
					if m.modal == modal {
						break
					}
				}
				if m.modal != modal {
					t.Fatalf("%s left the Actions modal for %v, want the pull request modal", name, m.topModal())
				}
				if got := layerNames(m.keyLayers()); got != layers {
					t.Errorf("the keys after %s are %q, want %q", name, got, layers)
				}
				if got := m.modal.View(); got != view {
					t.Errorf("the pull request modal shows something else after %s", name)
				}
			})
		})
	}
	t.Run("q", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			m, _, _, _ := open(t)
			driveKeys(t, m, m.key(press("q")))
			if m.topModal() != nil {
				t.Errorf("q left %v open", m.topModal())
			}
		})
	})
}

// The modal of a pull request shows no links and no timeline, so the keys of
// the issue modal for them do nothing there.
func TestPullModalIgnoresIssueKeys(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := openPanes(t)
		modal, view := m.modal, m.modal.View()
		for _, k := range []string{"#", "t"} {
			driveKeys(t, m, m.key(press(k)))
			if m.modal != modal || layerNames(m.keyLayers()) != "global, pull_modal, pull_conversation" {
				t.Errorf("%s changed the modal or its keys to %q", k, layerNames(m.keyLayers()))
			}
			if got := m.modal.View(); got != view {
				t.Errorf("%s changed what the modal shows", k)
			}
		}
	})
}

// :references is refused over a pull request, with the key unbound or not.
func TestPullModalRefusesReferences(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := openPanes(t)
		modal := m.modal
		runReferences(t, m)
		if !hasToast(m, "Close the pull request first to use references.") {
			t.Errorf("toast = %q, want the refusal", toasted(m))
		}
		if m.modal != modal || layerNames(m.keyLayers()) != "global, pull_modal, pull_conversation" {
			t.Errorf("the command left the keys at %q", layerNames(m.keyLayers()))
		}
	})
}
