package pulls

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
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
		picker.WithFilterLine(false),
		picker.WithMarks(kindOn, kindOff),
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
	// on is which kinds are ticked, and touched says that one was ticked or
	// unticked, which makes enter apply what is there rather than tick the
	// highlighted kind first.
	on      [conv.NumGroups]bool
	touched bool
	done    bool
}

func (o *filterOverlay) title() string           { return "Show" }
func (o *filterOverlay) kind() panes.OverlayKind { return panes.Picker }

// items is what the placement takes as a picker's rows, which it gives the
// two lines of a filter line besides: this one has none, so those are two
// rows less than it needs.
func (o *filterOverlay) items() int   { return max(o.rows-2, 0) }
func (o *filterOverlay) text() int    { return 0 }
func (o *filterOverlay) clear() bool  { return false }
func (o *filterOverlay) closed() bool { return o.done }

func (o *filterOverlay) layers() []keyhelp.Layer {
	return []keyhelp.Layer{ui.ContextHelp("picker_normal", filterHelp{&o.p}, false)}
}

// cancel closes the checklist without applying it, as esc does: the back key
// never reaches the picker, so the modal tells the overlay.
func (o *filterOverlay) cancel() { o.done = true }

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
	return []key.Binding{k.Normal.Up, k.Normal.Down, toggle, apply, k.Cancel}
}

func (h filterHelp) FullHelp() [][]key.Binding { return h.p.FullHelp() }

func (o *filterOverlay) view(w, h int) string {
	// The frame keeps a cell of room on each side of what it holds.
	o.p.SetSize(w-2, h)
	return o.p.View()
}

// toggle ticks or unticks the highlighted kind.
func (o *filterOverlay) toggle() {
	it, ok := o.p.Selected()
	if !ok {
		return
	}
	g, ok := it.Value.(conv.Group)
	if !ok {
		return
	}
	o.touched = true
	o.on[g] = !o.on[g]
	var marked []any
	for i, on := range o.on {
		if on {
			marked = append(marked, conv.Group(i))
		}
	}
	o.p.SetMarked(marked)
}

func (o *filterOverlay) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if keymap.Matches(msg, o.p.KeyMap().Normal.Toggle) {
			o.toggle()
			return nil
		}
	case picker.ChosenMsg:
		if msg.ID != o.p.ID() {
			return nil
		}
		if !o.touched {
			o.toggle()
		}
		o.done = true
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
