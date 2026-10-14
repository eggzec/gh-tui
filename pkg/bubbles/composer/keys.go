package composer

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// KeyMap holds the keys of a composer that aren't editing: those that
// submit, close it, and open the text in the editor. They are not letters,
// since the composer types them. Editing keys belong to the text area
// underneath and keep their defaults.
type KeyMap struct {
	Submit key.Binding `keymap:"submit" help:"submit"`
	Cancel key.Binding `keymap:"cancel" help:"cancel"`
	Edit   key.Binding `keymap:"edit" help:"editor"`
}

// NewKeyMap returns the key bindings that look gives for the actions of the
// composer, which the tags of its fields name. An action without keys gives
// a disabled binding that keeps its help text.
func NewKeyMap(look keymap.Lookup) KeyMap {
	var k KeyMap
	keymap.Fill(&k, look)
	return k
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Edit, k.Cancel}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Submit, k.Edit, k.Cancel}}
}
