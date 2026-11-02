package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
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

// menuTitles returns the titles and keys of the menu of the screen that
// the steps reach.
func menuTitles(t *testing.T, repo bool, steps ...string) []string {
	t.Helper()
	m := keysAfter(t, repo, steps...)
	layers, _ := m.innerLayers()
	entries := menuEntries(layers)
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.title+" "+e.key)
	}
	return got
}

func TestMenuEntriesOfOtherSurfaces(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		repo  bool
		steps []string
		want  []string
	}{
		{"issues list", true, []string{"global.pane_3"}, []string{"close issue X"}},
		{"notifications", false, []string{"global.notifications"}, []string{"read U", "done D"}},
		{"dashboard work", false, []string{"global.pane_3"}, []string{"checks C"}},
		{"dashboard inbox", false, []string{"global.pane_5"}, []string{"read U", "done D"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := menuTitles(t, tt.repo, tt.steps...); !slices.Equal(got, tt.want) {
				t.Errorf("menu = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMenuWithoutItemsToasts(t *testing.T) {
	t.Parallel()
	m := keysAfter(t, true, "global.pane_1")
	driveKeys(t, m, m.key(press("x")))
	if !hasToast(m, "Nothing to do with the selection.") {
		t.Errorf("no toast: toast %q", toasted(m))
	}
	if m.modals.Top() != nil {
		t.Errorf("top modal = %T, want none", m.modals.Top())
	}
}

// chooseMenuEntry opens the menu with x and chooses the nth entry.
func chooseMenuEntry(t *testing.T, m *Model, n int) {
	t.Helper()
	driveKeys(t, m, m.key(press("x")))
	if _, ok := m.modals.Top().(*menuModal); !ok {
		t.Fatalf("top modal = %T after x, want the menu", m.modals.Top())
	}
	for range n {
		driveKeys(t, m, m.key(press("down")))
	}
	driveKeys(t, m, m.key(press("enter")))
}

func TestMenuChoosingEachEntryOfOtherSurfaces(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		repo     bool
		steps    []string
		n        int
		wantText string
	}{
		{"close issue", true, []string{"global.pane_3"}, 0, "Close issue"},
		{"inbox read", false, []string{"global.pane_5"}, 0, "as read"},
		{"inbox done", false, []string{"global.pane_5"}, 1, "as done"},
		{"work checks", false, []string{"global.pane_3"}, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := keysAfter(t, tt.repo, tt.steps...)
			chooseMenuEntry(t, m, tt.n)
			top := m.modals.Top()
			if top == nil {
				t.Fatal("choosing the entry opened no modal")
			}
			if _, ok := top.(*menuModal); ok {
				t.Fatal("the menu is still open")
			}
			if tt.wantText != "" && !strings.Contains(onScreen(m), tt.wantText) {
				t.Errorf("screen after choosing lacks %q (top modal %T)", tt.wantText, top)
			}
		})
	}
}

func TestMenuOfAnOpenIssueHasNoReopen(t *testing.T) {
	t.Parallel()
	for _, e := range menuTitles(t, true, "global.pane_3") {
		if strings.HasPrefix(e, "reopen") {
			t.Errorf("menu lists %q on an open issue", e)
		}
	}
}

func TestMenuOfAnIssueWorkItemHasNoChecks(t *testing.T) {
	t.Parallel()
	m := keysAfter(t, false, "global.pane_3")
	// The work pane has a tab of issues: on it the menu has no checks.
	seen := false
	for range 4 {
		layers, _ := m.innerLayers()
		if len(menuEntries(layers)) == 0 {
			seen = true
			driveKeys(t, m, m.key(press("x")))
			if m.modals.Top() != nil {
				t.Errorf("top modal = %T on an issue, want none", m.modals.Top())
			}
			break
		}
		driveKeys(t, m, m.key(press("]")))
	}
	if !seen {
		t.Error("no work tab without menu entries: the issues tab still offers checks")
	}
}

func TestMenuOnAnEmptyInboxOpensNothing(t *testing.T) {
	t.Parallel()
	m := newKeysAppInbox(t, false, func(*config.Config) {}, keyInbox{none: true})
	driveKeys(t, m, m.Init())
	driveKeys(t, m, m.key(press("5")))
	driveKeys(t, m, m.key(press("x")))
	if m.modals.Top() != nil {
		t.Errorf("top modal = %T on an empty inbox, want none", m.modals.Top())
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
