package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
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

func TestMenuXKeyOpensOnAListOnly(t *testing.T) {
	t.Parallel()
	m := keysAfter(t, true, "global.pane_2")
	driveKeys(t, m, m.key(press("x")))
	menu, ok := m.modals.Top().(*menuModal)
	if !ok {
		t.Fatalf("top modal = %T after x on a list, want the menu", m.modals.Top())
	}

	// In a modal, x is the modal's key: it opens nothing over it.
	driveKeys(t, m, m.key(press("x")))
	if m.modals.Top() != menu {
		t.Errorf("top modal = %T after x in a modal, want the same menu", m.modals.Top())
	}

	// In the command line, x is typed.
	n := keysAfter(t, true, "global.pane_2")
	driveKeys(t, n, n.openLine())
	driveKeys(t, n, n.key(press("x")))
	if n.modals.Top() != nil || !n.line.Focused() {
		t.Errorf("after x in the command line: modal %T, line focused %v; want no modal and the line open", n.modals.Top(), n.line.Focused())
	}
}

func TestMenuChoosingClosesThenPresses(t *testing.T) {
	t.Parallel()
	m := keysAfter(t, true, "global.pane_2")
	driveKeys(t, m, m.openMenu())
	if _, ok := m.modals.Top().(*menuModal); !ok {
		t.Fatalf("top modal = %T, want the menu", m.modals.Top())
	}
	// The first entry is "merge".
	driveKeys(t, m, m.key(press("enter")))
	top := m.modals.Top()
	if _, ok := top.(*menuModal); ok {
		t.Fatal("the menu is still open after choosing an entry")
	}
	if top == nil {
		t.Fatal("choosing merge pressed nothing: no modal opened")
	}
	if got := top.Title(); !strings.HasPrefix(got, "Merge #") {
		t.Errorf("modal after choosing merge = %T %q, want the merge dialog", top, got)
	}
}

func TestMenuChoosingSequencesTheClose(t *testing.T) {
	t.Parallel()
	m := keysAfter(t, true, "global.pane_2")
	driveKeys(t, m, m.openMenu())
	menu, ok := m.modals.Top().(*menuModal)
	if !ok {
		t.Fatalf("top modal = %T, want the menu", m.modals.Top())
	}
	item, _ := menu.pick.Selected()
	cmd := menu.Update(picker.ChosenMsg{ID: menu.pick.ID(), Item: item})
	if cmd == nil {
		t.Fatal("choosing returned no command")
	}
	msg := cmd()
	if _, batch := msg.(tea.BatchMsg); batch {
		t.Fatal("choosing returned a batch, which runs the close and the press in no order")
	}
	cmds, ok := cmdsOf(msg)
	if !ok || len(cmds) != 2 {
		t.Fatalf("choosing returned %T, want a sequence of the close and the press", msg)
	}
	if _, ok := cmds[1]().(menuRunMsg); !ok {
		t.Errorf("the second step is not the press of the action")
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
