package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/keyname"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// keyPress returns a press of the key named name, as the config and
// bubbletea name keys, and false for a name it can't press.
func keyPress(name string) (tea.KeyPressMsg, bool) { return keyname.Press(name) }

// press presses the first key bound to action, as the user would, so that
// a command does just what its key does, where the key goes: the same
// gates, questions and help apply.
func (m *Model) press(action string) tea.Cmd {
	for _, name := range m.cfg.Keys.Of(action) {
		if msg, ok := keyPress(name); ok {
			return m.key(msg)
		}
	}
	return m.toast.Push(toast.Error, "No key is bound to keys."+action+" in the config.")
}
