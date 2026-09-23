package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
)

// Binding makes the binding of action from the configured keys, labelled with
// desc in help. An action without keys gives a disabled binding.
func Binding(keys map[string][]string, action, desc string) key.Binding {
	ks := keys[action]
	if len(ks) == 0 {
		return key.NewBinding(key.WithDisabled())
	}
	return key.NewBinding(
		key.WithKeys(ks...),
		key.WithHelp(label(ks[0]), desc),
	)
}

// label shortens key names for the help line.
func label(k string) string {
	switch k {
	case "enter":
		return "↵"
	case "esc":
		return "esc"
	case "space", " ":
		return "space"
	case "up":
		return "↑"
	case "down":
		return "↓"
	}
	return strings.ReplaceAll(k, "ctrl+", "^")
}
