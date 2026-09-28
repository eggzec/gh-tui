package uitest

import (
	"slices"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// Find returns the first binding of layers described as desc in help,
// and whether there is one.
func Find(layers []keyhelp.Layer, desc string) (key.Binding, bool) {
	for _, l := range layers {
		for _, b := range l.Bindings {
			if b.Help().Desc == desc {
				return b, true
			}
		}
	}
	return key.Binding{}, false
}

// Enabled returns the descriptions of the enabled bindings of layers, in
// order.
func Enabled(layers []keyhelp.Layer) []string {
	var out []string
	for _, l := range layers {
		for _, b := range l.Bindings {
			if b.Enabled() {
				out = append(out, b.Help().Desc)
			}
		}
	}
	return out
}

// Winner returns the binding that k, a key as tea.KeyPressMsg.String
// names it, reaches in layers, and the source of its layer, as
// keyhelp.Analyze finds them, or false when no binding gets it.
func Winner(layers []keyhelp.Layer, k string) (b key.Binding, source string, ok bool) {
	for _, r := range keyhelp.Analyze(layers) {
		if r.Status == keyhelp.Disabled || !slices.Contains(r.Binding.Keys(), k) {
			continue
		}
		if !slices.ContainsFunc(r.Lost, func(l keyhelp.Loss) bool { return l.Key == k }) {
			return r.Binding, r.Source, true
		}
	}
	return key.Binding{}, "", false
}
