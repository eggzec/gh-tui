package tui

import (
	"testing"
	"testing/synctest"
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
