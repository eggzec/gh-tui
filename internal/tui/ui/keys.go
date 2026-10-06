package ui

import (
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
)

// Binding makes the binding of action from the configured keys, labelled with
// desc in help. An action without keys, which the config unbinds, gives a
// disabled binding that still says what it does, so the help lists it
// without a key rather than as a blank line.
func Binding(keys map[string][]string, action, desc string) key.Binding {
	ks := keys[action]
	if len(ks) == 0 {
		return key.NewBinding(key.WithHelp("", desc), key.WithDisabled())
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
		return key.NewBinding(key.WithHelp("", b.Help().Desc), key.WithDisabled())
	}
	y := key.NewBinding(key.WithKeys(keys...), key.WithHelp(label(keys[0]), b.Help().Desc))
	y.SetEnabled(b.Enabled())
	return y
}

// OpenHint returns the hint that open, the key that opens something on
// GitHub, does so, such as "o to open on GitHub", with the key named as ic
// names it, or "" when it has no key.
func OpenHint(ic Icons, open key.Binding) string {
	if !open.Enabled() || open.Help().Key == "" {
		return ""
	}
	return ic.Key(open.Help().Key) + " to open on GitHub"
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

// Jump returns the binding that stands for the enabled keys of panes in
// help, such as "1-5 focus pane", or a disabled one while none has a key.
// Keys that don't run on, one after the other, as a config that unbinds
// or rebinds some may leave them, are listed each, such as "1/3/4".
func Jump(panes ...key.Binding) key.Binding {
	var keys, labels []string
	for _, b := range panes {
		if b.Enabled() {
			keys = append(keys, b.Keys()...)
			labels = append(labels, b.Help().Key)
		}
	}
	if len(labels) == 0 {
		return key.NewBinding(key.WithHelp("", "focus pane"), key.WithDisabled())
	}
	name := strings.Join(labels, "/")
	if len(labels) > 2 && runOn(labels) {
		name = labels[0] + "-" + labels[len(labels)-1]
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(name, "focus pane"))
}

// runOn reports whether labels are single characters that follow each
// other, such as 1, 2 and 3.
func runOn(labels []string) bool {
	prev := rune(-1)
	for i, l := range labels {
		r, n := utf8.DecodeRuneInString(l)
		if n != len(l) || i > 0 && r != prev+1 {
			return false
		}
		prev = r
	}
	return true
}
