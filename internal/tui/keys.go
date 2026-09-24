package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// KeyMap holds the keys the app handles itself. Sections have their own.
type KeyMap struct {
	Quit   key.Binding
	Help   key.Binding
	Search key.Binding
	// History opens the history of the repository on the repository
	// screen.
	History key.Binding
	// Notifications switches between the screen on view and the
	// notifications, and Dashboard between it and the dashboard.
	Notifications key.Binding
	Dashboard     key.Binding
	// Next and Prev cycle the focus through the panes.
	Next key.Binding
	Prev key.Binding
	// Panes focus pane 1, 2 and 3.
	Panes []key.Binding
	// jump stands for Panes in the help.
	jump key.Binding
}

func newKeyMap(keys map[string][]string) KeyMap {
	k := KeyMap{
		Quit:          ui.Binding(keys, config.ActionQuit, "quit"),
		Help:          ui.Binding(keys, config.ActionHelp, "help"),
		Search:        ui.Binding(keys, config.ActionSearch, "search"),
		History:       ui.Binding(keys, config.ActionHistory, "history"),
		Notifications: ui.Binding(keys, config.ActionNotifications, "notifications"),
		Dashboard:     ui.Binding(keys, config.ActionDashboard, "dashboard"),
		Next:          ui.Binding(keys, config.ActionNextTab, "next pane"),
		Prev:          ui.Binding(keys, config.ActionPrevTab, "previous pane"),
		Panes: []key.Binding{
			ui.Binding(keys, config.ActionPane1, "files"),
			ui.Binding(keys, config.ActionPane2, "pull requests"),
			ui.Binding(keys, config.ActionPane3, "issues"),
		},
	}
	labels := make([]string, 0, len(k.Panes))
	for _, b := range k.Panes {
		if b.Enabled() {
			labels = append(labels, b.Help().Key)
		}
	}
	if len(labels) > 0 {
		k.jump = key.NewBinding(key.WithKeys(labels...), key.WithHelp(strings.Join(labels, "/"), "focus pane"))
	} else {
		k.jump = key.NewBinding(key.WithDisabled())
	}
	return k
}

// pane returns the index of the pane that msg focuses, or -1.
func (k KeyMap) pane(msg tea.KeyPressMsg) int {
	for i, b := range k.Panes {
		if key.Matches(msg, b) {
			return i
		}
	}
	return -1
}
