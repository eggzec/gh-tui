package pulls

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// Marks of the kinds in a checklist: boxes, as the filter forms draw one.
const (
	kindOn  = "[x]"
	kindOff = "[ ]"
)

// openConvFilter shows the kinds of items the conversation has over the panes,
// each ticked while the conversation shows it. Before the conversation is
// there, it does nothing.
func (m *panesModal) openConvFilter() tea.Cmd {
	c := &m.conv
	if !c.started {
		return nil
	}
	st := m.theme.Picker(m.icons)
	st.Frame = lipgloss.NewStyle()
	o := c.model.NewFilter(m.refreshConv,
		picker.WithContext(m.ctx),
		picker.WithKeyMap(m.keys.pick),
		picker.WithStyles(st),
	)
	m.open(o)
	return o.P.Focus()
}
