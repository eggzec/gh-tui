package tui

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

func TestMenuEntriesSkipUnmarkedAndUnknown(t *testing.T) {
	t.Parallel()
	// Bindings in Bindings but not in Menu never list; a menu binding
	// that names no config action has nothing to press and is left out.
	l := keyhelp.Layer{
		Bindings: []key.Binding{key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "unmarked"))},
	}
	l.SetMenu([]key.Binding{key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "no action"))})
	if got := menuEntries([]keyhelp.Layer{l}); len(got) != 0 {
		t.Errorf("menuEntries = %v, want none", got)
	}
}

func TestMenuEntriesOfThePullsList(t *testing.T) {
	t.Parallel()
	m := keysAfter(t, true, "global.pane_2")
	layers, _ := m.innerLayers()
	entries := menuEntries(layers)
	actions := make([]string, 0, len(entries))
	for _, e := range entries {
		actions = append(actions, e.action)
	}
	if !slices.Contains(actions, "pulls.merge") {
		t.Errorf("menu actions = %v, want pulls.merge among them", actions)
	}
	if slices.Contains(actions, "global.menu") {
		t.Errorf("menu actions = %v, must not list unmarked actions", actions)
	}
}

func TestMenuChoosingClosesThenPresses(t *testing.T) {
	t.Parallel()
	m := keysAfter(t, true, "global.pane_2")
	driveKeys(t, m, m.openMenu())
	if _, ok := m.modals.Top().(*menuModal); !ok {
		t.Fatalf("top modal = %T, want the menu", m.modals.Top())
	}
	driveKeys(t, m, m.key(press("enter")))
	if _, ok := m.modals.Top().(*menuModal); ok {
		t.Fatal("the menu is still open after choosing an entry")
	}
	if m.modals.Top() == nil {
		t.Error("choosing the first entry pressed nothing: no modal opened")
	}
}

func TestMenuRefusesWhenTheSelectionMoved(t *testing.T) {
	t.Parallel()
	m := keysAfter(t, true, "global.pane_2")
	msg := menuRunMsg{action: "pulls.merge", sel: ui.Selection{What: "pull request", Number: 999}, had: true}
	driveKeys(t, m, m.runMenu(msg))
	if !hasToast(m, "The selection changed") {
		t.Errorf("toast = %q, want the selection-changed warning", toasted(m))
	}
	if m.modals.Top() != nil {
		t.Errorf("top modal = %T, want none: nothing may run", m.modals.Top())
	}
}

func TestMenuKeyOfUnboundAction(t *testing.T) {
	t.Parallel()
	b := key.NewBinding(key.WithHelp("", "merge"))
	if got := menuKey(b); got != "" {
		t.Errorf("menuKey = %q, want empty", got)
	}
	b = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "merge"))
	if got := menuKey(b); got != "m" {
		t.Errorf("menuKey = %q, want m", got)
	}
}
