package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/finder"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
)

// TestWidgetActionsAreInTheConfig checks that every action the key maps of
// the capturing widgets and the filter form name has keys in the config's
// defaults, so that the config validates what a user sets for them and
// checks it against the keys of the other contexts.
func TestWidgetActionsAreInTheConfig(t *testing.T) {
	keys := config.Default().Keys
	for _, tt := range []struct {
		ctx string
		km  any
	}{
		{"command_line", cmdline.KeyMap{}},
		{"help", keyhelp.KeyMap{}},
		{"prompt", prompt.KeyMap{}},
		{"finder", finder.KeyMap{}},
		{"picker", picker.KeyMap{}},
		{"filter", filterform.KeyMap{}},
		{"actions_filter", filterform.KeyMap{}},
		{"confirm", ui.ConfirmKeys{}},
	} {
		for _, name := range keymap.Names(tt.km) {
			action := name
			if !strings.Contains(name, ".") {
				action = tt.ctx + "." + name
			}
			if !slices.Contains(keys.Actions(), action) {
				t.Errorf("%s names %s, which the default keys don't have", tt.ctx, action)
			}
		}
	}
}

// TestHelpClosesWithItsOwnKey checks that the help closes with the keys of
// its context, which don't follow the key that opens it.
func TestHelpClosesWithItsOwnKey(t *testing.T) {
	cfg := config.Default()
	cfg.Keys.Set("help.close", []string{"ctrl+g"})
	m := New(t.Context(), cfg, Layout{Files: &fakeSection{title: "Files"}}, WithRepo(testRepo))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.key(press("?")))
	if !m.helpOpen() {
		t.Fatal("? didn't open the help")
	}
	run(m, m.key(press("?")))
	if !m.helpOpen() {
		t.Fatal("? closed the help after close was set to ctrl+g")
	}
	// The ? is typed into the query instead; esc clears it.
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyEscape}))
	run(m, m.key(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl}))
	if m.helpOpen() {
		t.Error("ctrl+g didn't close the help")
	}
}
