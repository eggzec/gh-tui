package prompt

import "charm.land/bubbles/v2/key"

// KeyMap holds the keys that finish a prompt. Editing keys belong to the
// text area or input underneath and keep their defaults.
type KeyMap struct {
	// Submit submits a prompt in either mode.
	Submit key.Binding `keymap:"submit" help:"submit"`
	// SubmitLine submits a single-line prompt too. A multi-line prompt
	// leaves it to the text area, where enter starts a new line.
	SubmitLine key.Binding `keymap:"submit_line" help:"submit"`
	Cancel     key.Binding `keymap:"cancel" help:"cancel"`
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Submit: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "submit"),
		),
		SubmitLine: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

// ShortHelp implements help.KeyMap. It lists the keys of a multi-line
// prompt; [Model.ShortHelp] lists those of the prompt's mode.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Cancel}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Submit, k.SubmitLine, k.Cancel}}
}
