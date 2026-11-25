package conv

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// Marks of the kinds in the filter: boxes, as the filter forms draw a checklist.
const (
	kindOn  = "[x]"
	kindOff = "[ ]"
)

// FilterOverlay is the checklist of the kinds of items in a conversation.
type FilterOverlay = prpanes.TickOverlay[Group]

// NewFilter returns the checklist of the kinds m holds, each ticked while m
// shows it. Applying sets what m shows, then calls changed, if set, so the host
// redraws. The host gives the picker options that are its own (context, keys
// and styles) and opens the overlay, focusing its picker.
func (m *Model) NewFilter(changed func(), opts ...picker.Option) *FilterOverlay {
	choices := m.Choices()
	o := &FilterOverlay{Heading: "Show", Rows: len(choices), Flat: true}
	o.Change = func(on, _ map[Group]bool) tea.Cmd {
		// Nothing ticked applies nothing.
		if len(on) == 0 {
			return nil
		}
		var shown [NumGroups]bool
		for g := range on {
			shown[g] = true
		}
		m.SetShown(shown)
		if changed != nil {
			changed()
		}
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
	base := make([]picker.Option, 0, 6+len(opts))
	base = append(base,
		picker.WithItems(items),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(true),
		picker.WithMarks(kindOn, kindOff),
		picker.WithTicks(),
	)
	o.P = picker.New(nil, append(base, opts...)...)
	o.P.SetMarked(marked)
	return o
}
