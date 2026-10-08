// Package keyhelp lists the keys a program takes, for a help overlay.
//
// Keys come in [Layer]s, in the order a key press is offered to them, such
// as a modal's keys before the app's. [Analyze] finds which binding each
// key reaches, and the [Model] shows every binding with its state: one a
// key reaches, one that is disabled, one that loses its key to another
// binding, or one whose key is typed into an input first. The user can
// filter the list by a fuzzy query, or by pressing the key itself.
package keyhelp

import (
	"slices"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// Layer is a set of bindings that take keys together, such as those of one
// component.
type Layer struct {
	// Source names where the bindings belong, such as "Pull requests".
	Source string
	// Context is the name of the context of keys that the layer is, such
	// as "pulls", or empty for a layer that is none, such as the keys the
	// app always takes.
	Context string
	// Bindings holds every binding of the layer, disabled ones too, in
	// the order the layer matches them.
	Bindings []key.Binding
	// Actions names the actions of each binding, in the order of Bindings,
	// as their config paths without the leading "keys.", such as
	// "pulls.merge": a binding made of several, such as one that quits and
	// closes, has each. It may be shorter than Bindings, or empty, and a
	// binding that is no action of the config has none. The help filter
	// matches the path "keys.<action>".
	Actions [][]string
	// Typing is whether the layer types the printable keys none of its
	// bindings take, as a text input does, so that later layers never
	// see them.
	Typing bool
	// Short holds the bindings worth a hint, as help.KeyMap's ShortHelp
	// does.
	Short []key.Binding
}

// FromHelp returns the layer of km: its full help, flattened, and its
// short help.
func FromHelp(source string, km help.KeyMap, typing bool) Layer {
	return Layer{
		Source:   source,
		Bindings: slices.Concat(km.FullHelp()...),
		Typing:   typing,
		Short:    slices.Clone(km.ShortHelp()),
	}
}
