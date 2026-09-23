package tabs

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Init returns no command.
func (m Model) Init() tea.Cmd { return nil }

// Update handles key presses while the bar is focused. Changing tab returns
// a command that emits a [ChangeMsg].
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.focused || len(m.tabs) == 0 {
		return m, nil
	}
	n := len(m.tabs)
	var cmd tea.Cmd
	switch {
	case key.Matches(k, m.keys.Next):
		cmd = m.SetActive((m.active + 1) % n)
	case key.Matches(k, m.keys.Prev):
		cmd = m.SetActive((m.active - 1 + n) % n)
	case key.Matches(k, m.keys.Jump):
		if s := k.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			cmd = m.SetActive(int(s[0] - '1'))
		}
	}
	return m, cmd
}
