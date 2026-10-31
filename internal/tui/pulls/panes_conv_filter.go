package pulls

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
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
	o := &filterOverlay{m: m, rows: len(choices)}
	var marked []any
	items := make([]picker.Item, len(choices))
	for i, ch := range choices {
		o.on[ch.Group] = ch.On
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
// Space ticks the highlighted kind, enter applies the ticks, and esc closes
// without applying.
type filterOverlay struct {
	m    *panesModal
	p    picker.Model
	rows int
	// on is which kinds are ticked, read from the picker when enter applies.
	on   [conv.NumGroups]bool
	done bool
}

func (o *filterOverlay) title() string           { return "Show" }
func (o *filterOverlay) kind() panes.OverlayKind { return panes.Picker }

// items is what the placement takes as a picker's rows.
func (o *filterOverlay) items() int   { return o.rows }
func (o *filterOverlay) text() int    { return 0 }
func (o *filterOverlay) clear() bool  { return false }
func (o *filterOverlay) closed() bool { return o.done }

// typing reports whether the filter line takes the keys, so back is not
// offered to it.
func (o *filterOverlay) typing() bool { return o.p.Typing() }

func (o *filterOverlay) layers() []keyhelp.Layer {
	return []keyhelp.Layer{ui.ContextHelp("picker_normal", filterHelp{&o.p}, false)}
}

// filterHelp is the picker's help with a footer that says what the keys do
// here: space ticks a kind, which the picker's own footer leaves out, and
// enter applies the ticks rather than opening anything.
type filterHelp struct{ p *picker.Model }

func (h filterHelp) ShortHelp() []key.Binding {
	k := h.p.KeyMap()
	toggle := k.Normal.Toggle
	toggle.SetHelp(toggle.Help().Key, "toggle")
	apply := k.Choose
	apply.SetHelp(apply.Help().Key, "apply")
	return []key.Binding{k.Normal.Up, k.Normal.Down, k.Normal.Search, toggle, apply, k.Cancel}
}

func (h filterHelp) FullHelp() [][]key.Binding { return h.p.FullHelp() }

func (o *filterOverlay) view(w, h int) string {
	// The frame keeps a cell of room on each side of what it holds.
	o.p.SetSize(w-2, h)
	return o.p.View()
}

func (o *filterOverlay) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case picker.ChosenMsg:
		if msg.ID != o.p.ID() {
			return nil
		}
		o.done = true
		// Nothing ticked applies nothing.
		if len(msg.Ticked) == 0 {
			return nil
		}
		o.on = [conv.NumGroups]bool{}
		for _, it := range msg.Ticked {
			if g, ok := it.Value.(conv.Group); ok {
				o.on[g] = true
			}
		}
		c := &o.m.conv
		c.model.SetShown(o.on)
		o.m.refreshConv()
		return nil
	case picker.CancelMsg:
		if msg.ID == o.p.ID() {
			o.done = true
		}
		return nil
	}
	var cmd tea.Cmd
	o.p, cmd = o.p.Update(msg)
	return cmd
}
