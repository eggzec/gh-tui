package pulls

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// reactOverlay is the checklist of the reactions on a comment, keyed by the
// kind of the reaction.
type reactOverlay = prpanes.TickOverlay[string]

// openReact shows the kinds of reaction over the panes for the comment under
// the cursor, each ticked while the viewer has given it. Applying adds the
// ticked kinds that weren't and removes those that no longer are.
func (m *panesModal) openReact() tea.Cmd {
	s, ok := m.conv.model.Current()
	if !ok || s.Item == nil || (s.Kind != conv.KindComment && s.Kind != conv.KindRunComment) {
		return ui.Notify(toast.Info, "select a comment to react to")
	}
	id := s.Item.ID
	glyphs := conv.NewStyles(m.theme, m.icons, m.icons.Set()).Glyphs.Reactions
	o := &reactOverlay{Heading: "React", Rows: len(conv.ReactionOrder), Flat: true, On: map[string]bool{}, Was: map[string]bool{}}
	for _, r := range s.Item.Reactions {
		if r.Mine {
			o.On[r.Content], o.Was[r.Content] = true, true
		}
	}
	o.Change = func(on, was map[string]bool) tea.Cmd {
		var cmds []tea.Cmd
		for _, content := range conv.ReactionOrder {
			if on[content] == was[content] {
				continue
			}
			done := "reaction removed"
			if on[content] {
				done = "reaction added"
			}
			op := m.svc.React(m.repo, m.number, id, content, on[content])
			send := m.sendConvOp(op, "react to comment", done)
			cmds = append(cmds, m.recacheConv(), send)
		}
		return tea.Batch(cmds...)
	}
	items := make([]picker.Item, len(conv.ReactionOrder))
	var marked []any
	for i, content := range conv.ReactionOrder {
		name := strings.ToLower(strings.ReplaceAll(content, "_", " "))
		items[i] = picker.Item{Title: glyphs[content] + " " + name, Value: content}
		if o.On[content] {
			marked = append(marked, content)
		}
	}
	st := m.theme.Picker(m.icons)
	st.Frame = lipgloss.NewStyle()
	o.P = picker.New(nil,
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
	o.P.SetMarked(marked)
	m.open(o)
	return o.P.Focus()
}
