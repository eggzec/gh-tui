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
	// Menu holds the bindings the context menu offers on the item the layer
	// acts on, in the order it lists them. Each is also one of Bindings, so
	// a disabled one is hidden from the menu.
	//
	// It is a pointer so that a Layer stays small enough to range over by
	// value; use [Layer.MenuItems] and [Layer.SetMenu].
	Menu *[]key.Binding
}

// MenuItems returns the bindings of the context menu, or nil for a layer
// that offers none.
func (l *Layer) MenuItems() []key.Binding {
	if l.Menu == nil {
		return nil
	}
	return *l.Menu
}

// SetMenu sets the bindings of the context menu.
func (l *Layer) SetMenu(items []key.Binding) {
	if len(items) == 0 {
		l.Menu = nil
		return
	}
	l.Menu = &items
}

// SameBinding reports whether a and b are the same binding, in the same
// state: enabled alike, with the same help and keys.
func SameBinding(a, b key.Binding) bool {
	return a.Enabled() == b.Enabled() && a.Help() == b.Help() && slices.Equal(a.Keys(), b.Keys())
}

// Equal reports whether l and o hold the same bindings, in the same state,
// so that the hints they give are the same. It compares Source, Typing,
// Bindings and Short, and not Context or Actions.
func (l Layer) Equal(o Layer) bool {
	return l.Source == o.Source && l.Typing == o.Typing &&
		slices.EqualFunc(l.Bindings, o.Bindings, SameBinding) && slices.EqualFunc(l.Short, o.Short, SameBinding)
}

// Clone returns a copy of l whose Bindings and Short its maker can't change,
// as a section may keep its bindings and change them later.
func (l Layer) Clone() Layer {
	l.Bindings = slices.Clone(l.Bindings)
	l.Short = slices.Clone(l.Short)
	l.SetMenu(slices.Clone(l.MenuItems()))
	return l
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
