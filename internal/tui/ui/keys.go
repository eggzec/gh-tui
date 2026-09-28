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

// Yield returns b as the help lists it beside held, a binding matched
// before it that shares some of its keys: while held is disabled, which
// the help shows as taking no key, it still takes those keys, to say why
// it can't act, so b is listed without them. The keys are matched as
// they are; only the help uses it.
func Yield(b, held key.Binding) key.Binding {
	if held.Enabled() {
		return b
	}
	keys := slices.DeleteFunc(slices.Clone(b.Keys()), func(k string) bool { return slices.Contains(held.Keys(), k) })
	switch len(keys) {
	case len(b.Keys()):
		return b
	case 0:
		return key.NewBinding(key.WithDisabled())
	}
	y := key.NewBinding(key.WithKeys(keys...), key.WithHelp(label(keys[0]), b.Help().Desc))
	y.SetEnabled(b.Enabled())
	return y
}

// OpenHint returns the hint that open, the key that opens something on
// GitHub, does so, such as "o to open on GitHub", or "" when it has no key.
func OpenHint(open key.Binding) string {
	if !open.Enabled() || open.Help().Key == "" {
		return ""
	}
	return open.Help().Key + " to open on GitHub"
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
