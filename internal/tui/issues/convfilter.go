package issues

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// openConvFilter shows the kinds of items the conversation has over the
// panes, each ticked while the conversation shows it. Before the conversation
// is there, it does nothing.
func (m *detailModal) openConvFilter() tea.Cmd {
	if !m.convOn {
		return nil
	}
	st := m.theme.Picker(m.icons)
	st.Frame = lipgloss.NewStyle()
	o := m.conv.NewFilter(nil,
		picker.WithContext(m.ctx),
		picker.WithKeyMap(m.keys.pick),
		picker.WithStyles(st),
	)
	m.over = o
	return o.P.Focus()
}
