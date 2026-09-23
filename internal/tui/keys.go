package tui

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/tabs"
)

// KeyMap holds the keys the app handles itself. Sections have their own.
type KeyMap struct {
	Quit key.Binding
	Help key.Binding
	Tabs tabs.KeyMap
}

func newKeyMap(keys map[string][]string) KeyMap {
	tk := tabs.DefaultKeyMap()
	tk.Next = ui.Binding(keys, config.ActionNextTab, "next tab")
	tk.Prev = ui.Binding(keys, config.ActionPrevTab, "previous tab")
	return KeyMap{
		Quit: ui.Binding(keys, config.ActionQuit, "quit"),
		Help: ui.Binding(keys, config.ActionHelp, "help"),
		Tabs: tk,
	}
}
