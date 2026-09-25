package ui

import (
	"slices"
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

// FreeKeys returns b without the keys that the bindings of taken use, so
// that a bubble inside a view leaves the view's own keys to it. The help of
// what is left names its keys; b without any keys is disabled.
func FreeKeys(b key.Binding, taken ...key.Binding) key.Binding {
	keys := slices.DeleteFunc(slices.Clone(b.Keys()), func(k string) bool {
		return slices.ContainsFunc(taken, func(t key.Binding) bool {
			return t.Enabled() && slices.Contains(t.Keys(), k)
		})
	})
	switch len(keys) {
	case len(b.Keys()):
		return b
	case 0:
		return key.NewBinding(key.WithDisabled())
	}
	labels := make([]string, 0, 2)
	for _, k := range keys[:min(len(keys), 2)] {
		labels = append(labels, label(k))
	}
	nb := key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(labels, "/"), b.Help().Desc))
	nb.SetEnabled(b.Enabled())
	return nb
}
