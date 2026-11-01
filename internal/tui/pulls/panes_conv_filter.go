package pulls

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// Marks of the kinds in the filter: boxes, as the filter forms draw a checklist.
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
	choices := c.model.Choices()
	o := &filterOverlay{heading: "Show", rows: len(choices), flat: true}
	o.change = func(on, _ map[conv.Group]bool) tea.Cmd {
		// Nothing ticked applies nothing.
		if len(on) == 0 {
			return nil
		}
		var shown [conv.NumGroups]bool
		for g := range on {
			shown[g] = true
		}
		c.model.SetShown(shown)
		m.refreshConv()
		return nil
	}
	var marked []any
	items := make([]picker.Item, len(choices))
	for i, ch := range choices {
		if ch.On {
			marked = append(marked, ch.Group)
		}
		items[i] = picker.Item{Title: ch.Name, Detail: strconv.Itoa(ch.Count), Value: ch.Group}
	}
	st := m.theme.Picker(m.icons)
	st.Frame = lipgloss.NewStyle()
	o.p = picker.New(nil,
		picker.WithContext(m.ctx),
		picker.WithItems(items),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(true),
		picker.WithMarks(kindOn, kindOff),
		picker.WithTicks(),
		picker.WithKeyMap(m.keys.pick),
		picker.WithStyles(st),
	)
	o.p.SetMarked(marked)
	m.open(o)
	return o.p.Focus()
}

// filterOverlay is the checklist of the kinds of items in the conversation.
type filterOverlay = tickOverlay[conv.Group]
