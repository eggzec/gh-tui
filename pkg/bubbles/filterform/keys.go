package filterform

import (
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// KeyMap holds the key bindings of a form. It implements help.KeyMap.
//
// While a text input, a picker or the query line has focus the form takes
// every key, so only Up, Down, Toggle, Edit, Apply and Cancel act there;
// the rest are typed.
type KeyMap struct {
	// Up and Down move between the rows and the query line.
	Up   key.Binding
	Down key.Binding
	// Left and Right choose in a Choice or the sort, and move between the
	// chips of a Multi.
	Left  key.Binding
	Right key.Binding
	// Toggle flips a Toggle, picks the next choice, flips the sort's
	// direction, and in a Multi's picker chooses the highlighted item.
	Toggle key.Binding
	// Edit opens the editor of a Multi, Person or Text field, and closes
	// it again keeping what was chosen.
	Edit key.Binding
	// Apply sends an AppliedMsg from any other row or the query line.
	Apply key.Binding
	// Remove removes the chip under the cursor, or clears a field.
	Remove key.Binding
	// Reset puts every field back to its default.
	Reset key.Binding
	// Cancel closes an open editor and undoes what it changed, or sends a
	// CancelMsg.
	Cancel key.Binding
	// Picker holds the keys of the picker in a Multi or Person editor. Its
	// Choose and Cancel are taken by Edit and Cancel.
	Picker picker.KeyMap
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:     key.NewBinding(key.WithKeys("up", "shift+tab"), key.WithHelp("↑", "previous field")),
		Down:   key.NewBinding(key.WithKeys("down", "tab"), key.WithHelp("↑↓", "field")),
		Left:   key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "previous")),
		Right:  key.NewBinding(key.WithKeys("right"), key.WithHelp("←→", "choose")),
		Toggle: key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "choose")),
		Edit:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "edit")),
		Apply:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("↵", "apply")),
		Remove: key.NewBinding(key.WithKeys("x", "backspace"), key.WithHelp("x", "remove")),
		Reset:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reset")),
		Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		Picker: picker.DefaultKeyMap(),
	}
}

// ShortHelp returns the bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Down, k.Right, k.Apply, k.Reset, k.Cancel}
}

// FullHelp returns the bindings for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.Toggle},
		{k.Edit, k.Remove, k.Apply, k.Reset, k.Cancel},
	}
}

// relabel returns b with its help text set to desc.
func relabel(b key.Binding, desc string) key.Binding {
	b.SetHelp(b.Help().Key, desc)
	return b
}
