package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(config.Default().Keys))
}

// winner names the binding that k reaches in the app's layers, by its
// layer.
func winner(m *Model, k string) string {
	b, src, ok := uitest.Winner(m.keyLayers(), k)
	if !ok {
		return "nothing"
	}
	return src + ": " + b.Help().Desc
}

// The layers take a key in the order the app routes it: a claimed key to
// the section, the app's own before the section's, a modal's and a
// capturing section's before everything but the quit key.
func TestKeyLayersOrder(t *testing.T) {
	m, pulls, _ := newFilterApp(t)
	if got := winner(m, "]"); got != "Pull requests: claimed" {
		t.Errorf("] reaches %q, want the section that claims it", got)
	}
	run(m, m.key(press("]")))
	if m.focus != 1 || !pulls.got(isKey("]")) {
		t.Error("] didn't reach the section that claims it")
	}
	if got := winner(m, "["); got != "app: previous pane" {
		t.Errorf("[ reaches %q, want the app", got)
	}

	m, fakes := newTestApp(t)
	fakes[0].keyMap = backKeys{}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	if got := winner(m, "esc"); got != "Files: back" {
		t.Errorf("esc reaches %q, want the section's back", got)
	}
	run(m, m.key(press("z")))
	if got := winner(m, "esc"); got != "app: unzoom" {
		t.Errorf("esc reaches %q while zoomed, want the unzoom", got)
	}
	run(m, m.key(press("esc")))
	if m.zoom || fakes[0].got(isKey("esc")) {
		t.Error("esc while zoomed didn't unzoom, or reached the section")
	}

	fakes[0].capturing = true
	if got := winner(m, "q"); got != "nothing" {
		t.Errorf("q reaches %q in a capturing section, want it typed", got)
	}
	if got := winner(m, "ctrl+c"); got != "app: quit" {
		t.Errorf("ctrl+c reaches %q in a capturing section, want the quit", got)
	}
	fakes[0].capturing = false

	// Away from the repository screen there are no panes to cycle.
	run(m, m.key(press("n")))
	if m.screen != notifScreen {
		t.Fatalf("n showed screen %d, want the notifications", m.screen)
	}
	if got := winner(m, "tab"); got == "app: next pane" {
		t.Error("the notifications offer the next pane")
	}
	run(m, m.key(press("n")))

	mod := &fakeModal{title: "Preview"}
	run(m, ui.OpenModal(mod))
	if got := winner(m, "x"); got != "Preview: close" {
		t.Errorf("x reaches %q with a modal open, want the modal", got)
	}
	if got := winner(m, "?"); got != "app: help" {
		t.Errorf("? reaches %q with a modal open, want the help", got)
	}
	mod.typing = true
	if got := winner(m, "?"); got != "nothing" {
		t.Errorf("? reaches %q with a modal that types, want it typed", got)
	}
}
