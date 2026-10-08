package keymap

import (
	"slices"

	"charm.land/bubbles/v2/key"
)

// Join returns one binding for several that do one thing, so that help can
// list them as one row, such as the quit and dismiss keys of a reader that
// both close it. It holds the keys of those that are enabled, in order, or
// of all of them while none is. It is labelled with all of its keys, joined
// by "/", and worded as the first of those it takes keys from that has a
// description, or else the first. It is enabled while any of them is. Join
// of none is a disabled binding.
func Join(bs ...key.Binding) key.Binding { return Derive(join(bs...), bs...) }

func join(bs ...key.Binding) key.Binding {
	use := slices.DeleteFunc(slices.Clone(bs), func(b key.Binding) bool { return !b.Enabled() })
	enabled := len(use) > 0
	if !enabled {
		use = bs
	}
	if len(use) == 0 {
		return key.NewBinding(key.WithDisabled())
	}
	var keys []string
	for _, b := range use {
		for _, k := range b.Keys() {
			if !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
	}
	// A binding that says nothing, such as one a parent switched off, lends
	// its words to none.
	lead := use[0]
	if i := slices.IndexFunc(use, func(b key.Binding) bool { return b.Help().Desc != "" }); i >= 0 {
		lead = use[i]
	}
	h := lead.Help()
	if len(keys) == 0 {
		return key.NewBinding(key.WithHelp(h.Key, h.Desc), key.WithDisabled())
	}
	out := key.NewBinding(key.WithKeys(keys...), key.WithHelp(Labels(keys), h.Desc))
	out.SetEnabled(enabled)
	return out
}
